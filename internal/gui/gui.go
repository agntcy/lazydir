// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agntcy/lazydir/internal/config"
	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
	"github.com/jesseduffield/gocui"
)

// connStatus describes the connectivity state of a remote service (Directory
// or OASF). A single field per connection replaces the previous boolean
// combinations (connected/connectFailed/pinging).
type connStatus int

const (
	connIdle   connStatus = iota // not yet attempted (gray ○)
	connTrying                   // check in progress (gray ○ + ↻)
	connOK                       // confirmed working (green ●)
	connFailed                   // confirmed broken (red ●)
)

// streamState describes the lifecycle phase of the records stream.
type streamState int

const (
	streamIdle      streamState = iota // no client yet, or no stream issued
	streamLoading                      // first page hasn't arrived yet
	streamStreaming                    // first page rendered, still receiving the rest
	streamDone                         // stream finished cleanly
	streamErrored                      // stream finished with an error
)

// appState holds all mutable application state. Fields are only mutated on
// the GUI goroutine (inside g.Update callbacks or key handlers).
type appState struct {
	// Connections panel cursor (0 = Directory, 1 = OASF)
	connCursor int

	// Directory connection
	activeDir        config.DirectoryEntry
	serverAddr       string
	authMode         string
	dirStatus        connStatus
	dirLastConnected time.Time
	dirError         string
	dirLastCfg       *dirclient.Config
	client           *dirclient.Client
	dirReconnStop    chan struct{}

	// OASF connection
	oasfAddr          string
	oasfClient        *oasf.Client
	oasfStatus        connStatus
	oasfLastConnected time.Time
	oasfError         string
	oasfReconnStop    chan struct{}

	// Server selection popup state
	serverMenuVisible bool
	serverMenuItems   []string
	serverMenuCursor  int

	// Auth popup content (for dynamic sizing via popupContentSize)
	authPopupText string

	// fullCache holds every record received from the unfiltered server stream.
	// Client-side filters narrow this into records; a new server stream is
	// only started on explicit refresh or server change.
	fullCache []*dirclient.RecordSummary

	// records holds the subset of fullCache that matches the current
	// server-side filter selection (or all of fullCache when no Trusted/
	// Verified filter is active). filteredRecords is the further subset
	// after the local name query.
	records         []*dirclient.RecordSummary
	filteredRecords []*dirclient.RecordSummary
	recordCursor    int

	// Record grouping: records with the same Name but different versions
	// are grouped together with a collapsible header.
	recordDisplayRows   []recordDisplayRow
	recordGroupExpanded map[string]bool

	// records-stream lifecycle
	stream     streamState
	streamErr  string
	cancelLoad context.CancelFunc

	// inline record info toggle (records panel)
	recordInfoCID     string // CID of the record whose info is expanded, "" if none
	recordInfoText    string // cached description text
	recordInfoError   bool   // true when recordInfoText is an error message
	recordInfoLoading bool   // fetch in progress

	// distinct values usable as filter options for each category, growing
	// monotonically across every stream (we never forget an option once
	// we've seen it, even if the next filtered stream wouldn't include it).
	filterValues *filterValueAggregator

	// activeFilterValues is recomputed from state.records each time
	// applyFilters runs. optionsFor reads from it so the Filters panel
	// only shows values present in the current filtered result set.
	// nil when no filters are active (falls back to filterValues).
	activeFilterValues *filterValueAggregator

	// classEntries caches enriched display info (ID, caption) for OASF
	// taxonomy classes. Populated once after the first record batch
	// arrives and provides the schema version needed for the lookup.
	classEntries    map[oasf.ClassType]map[string]oasf.ClassEntry
	classEntriesVer string // schema version used for the fetch, "" if not yet fetched

	// [2] Filters panel state
	filters filterState

	// name filter (active query; empty means no filter)
	filterQuery string

	// input prompt state
	inputVisible   bool
	inputTitle     string
	prevView       string       // view to return focus to on dismiss
	onInputConfirm func(string) // called with TextArea content on enter
	onInputCancel  func()       // called on esc
	onInputChange  func(string) // called live (debounced) as the user types; nil disables
	inputDebounce  *time.Timer  // debounce timer for live onChange

	// popup state (only one right-column popup is active at a time)
	popupPrevView  string // view to restore focus to when any popup closes
	infoPopupPanel string // which panel opened the info popup (viewDirectory/viewFilters/viewRecords)

	// confirmation popup state
	confirmPopupText string // rendered text shown inside the confirm popup
	onConfirmAction  func() // action to run when the user confirms

	// clipboard for copy-paste between nodes (issue #20)
	clipboard          map[string]*dirclient.RecordSummary // CID → full summary snapshot
	clipboardSource    string                              // display address of source server
	clipboardSourceURL string                              // full URL of source (used in CreateSync)

	// active sync operation state (for cancellation)
	syncID         string             // active sync operation ID
	syncCancelFunc context.CancelFunc // cancels pollSync/pollReconcile goroutines
	syncCIDs       []string           // CIDs involved in the active sync

	// preview dimming: stored content so we can toggle dim without refetching
	previewSubtitle string
	previewContent  string    // rendered (ANSI-colored) content
	previewDimmed   bool      // true when the preview is currently showing dimmed
	previewTree     *jsonTree // collapsible JSON tree for the preview panel
	previewCursor   int       // cursor line in the preview panel (for expand/collapse)
}

// Config bundles everything needed to start the GUI.
type Config struct {
	Directory          dirclient.Config
	OASF               oasf.Config
	DirectoryServers   []config.DirectoryEntry
	OASFServers        []string
	Theme              config.ThemeConfig
	ScrollStep         int
	SplitRatio         float64
	InputDebounceDelay int
	DimLevel           float64
	FirstPageSize      int
	BatchSize          int
}

// Gui is the top-level lazydir GUI object.
type Gui struct {
	g     *gocui.Gui
	state appState
	cfg   Config
	theme Theme
}

// New creates and starts the lazydir GUI.
func New(cfg Config) error {
	oasfClient, err := oasf.NewClient(cfg.OASF)
	if err != nil {
		return fmt.Errorf("configuring OASF client: %w", err)
	}

	initialDir := config.DirectoryEntry{Address: cfg.Directory.ServerAddress}
	if len(cfg.DirectoryServers) > 0 {
		initialDir = cfg.DirectoryServers[0]
	}

	app := &Gui{
		cfg:   cfg,
		theme: newTheme(cfg.Theme, cfg.DimLevel),
		state: appState{
			activeDir:    initialDir,
			serverAddr:   cfg.Directory.ServerAddress,
			authMode:     cfg.Directory.AuthMode,
			oasfAddr:     cfg.OASF.ServerAddress,
			oasfClient:   oasfClient,
			filters:      newFilterState(),
			filterValues: newFilterValueAggregator(),
		},
	}

	g, err := gocui.NewGui(gocui.NewGuiOpts{
		OutputMode:      gocui.OutputTrue,
		SupportOverlaps: false,
	})
	if err != nil {
		return fmt.Errorf("creating gui: %w", err)
	}
	defer g.Close()

	app.g = g
	g.Highlight = true
	g.SelFgColor = app.theme.ActiveBorderColor
	g.SelFrameColor = app.theme.ActiveBorderColor
	g.Mouse = true

	g.SetManagerFunc(app.layout)

	if err := app.bindKeys(g); err != nil {
		return fmt.Errorf("binding keys: %w", err)
	}

	// Kick off the initial connections in the background.
	app.state.dirStatus = connTrying
	if initialDir.OIDCIssuer != "" {
		go app.connectWithOIDC(initialDir)
	} else {
		dirCfg := cfg.Directory
		if dirCfg.AuthMode == "" {
			dirCfg.AuthMode = authModeInsecure
		}
		go app.connect(dirCfg)
	}
	app.state.oasfStatus = connTrying
	go app.pingOASF(oasfClient)

	if err := g.MainLoop(); err != nil && !gocui.IsQuit(err) {
		return fmt.Errorf("main loop: %w", err)
	}

	return nil
}

// connect dials the directory server and loads records.
func (app *Gui) connect(cfg dirclient.Config) {
	ctx := context.Background()
	c, err := dirclient.Connect(ctx, cfg)
	if err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.state.dirStatus = connFailed
			app.state.dirError = err.Error()
			app.state.dirLastCfg = &cfg
			app.state.stream = streamIdle
			app.renderDirectory(g)
			app.renderStatus(g)
			app.openInfoPopup(g, viewDirectory)
			app.startReconnectLoop()
			return nil
		})
		return
	}

	app.g.Update(func(g *gocui.Gui) error {
		app.stopReconnectLoop()
		if app.state.client != nil {
			app.state.client.Close()
		}
		c.FirstPageSize = app.cfg.FirstPageSize
		c.BatchSize = app.cfg.BatchSize
		app.state.client = c
		app.state.serverAddr = cfg.ServerAddress
		app.state.authMode = cfg.AuthMode
		app.state.dirLastCfg = &cfg
		app.state.dirStatus = connTrying
		app.renderDirectory(g)
		app.renderStatus(g)
		app.startRecordsStream()
		return nil
	})
}

// ── Connection health loops ──────────────────────────────────────────────────
//
// Both Directory and OASF ping periodically (5s when healthy, 1s when
// failed) and update the connection indicator. They never flush cached
// records, filters, or OASF class data — that only happens when the user
// explicitly changes an address. If the Directory has no client yet
// (initial connect failed), the loop silently retries establishing one.

const (
	retryInterval  = 1 * time.Second
	healthInterval = 5 * time.Second
	pingTimeout    = 3 * time.Second
)

// startReconnectLoop starts the Directory health/reconnect loop.
func (app *Gui) startReconnectLoop() {
	if app.state.dirReconnStop != nil {
		return
	}
	stop := make(chan struct{})
	app.state.dirReconnStop = stop
	go app.dirHealthLoop(stop)
}

func (app *Gui) stopReconnectLoop() {
	if app.state.dirReconnStop != nil {
		close(app.state.dirReconnStop)
		app.state.dirReconnStop = nil
	}
}

func (app *Gui) dirHealthLoop(stop chan struct{}) {
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	// Track consecutive ping failures for OIDC connections so we only
	// attempt a token refresh after a sustained outage, not on every blip.
	var oidcFailCount int
	const oidcRefreshThreshold = 3

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		type snapshot struct {
			client    *dirclient.Client
			cfg       *dirclient.Config
			status    connStatus
			activeDir config.DirectoryEntry
		}
		ch := make(chan snapshot, 1)
		app.g.Update(func(g *gocui.Gui) error {
			ch <- snapshot{
				client:    app.state.client,
				cfg:       app.state.dirLastCfg,
				status:    app.state.dirStatus,
				activeDir: app.state.activeDir,
			}
			return nil
		})
		snap := <-ch

		if snap.status == connTrying {
			ticker.Reset(healthInterval)
			continue
		}

		if snap.client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
			err := snap.client.Ping(ctx)
			cancel()

			if err == nil {
				oidcFailCount = 0
				app.g.Update(func(g *gocui.Gui) error {
					if app.state.client != snap.client {
						return nil
					}
					prev := app.state.dirStatus
					app.state.dirStatus = connOK
					app.state.dirLastConnected = time.Now()
					app.state.dirError = ""
					if prev != connOK {
						app.renderDirectory(g)
					}
					if prev != connOK && len(app.state.records) == 0 {
						app.startRecordsStream()
					}
					return nil
				})
				ticker.Reset(healthInterval)
				continue
			}

			// Ping failed. For OIDC connections, try a silent token refresh
			// after several consecutive failures (avoids reacting to transient
			// blips). Never wipe cached records.
			isOIDC := snap.cfg != nil && snap.cfg.OIDCIssuer != ""
			if isOIDC {
				oidcFailCount++
				if oidcFailCount >= oidcRefreshThreshold {
					if app.tryOIDCTokenRefresh(snap) {
						oidcFailCount = 0
						ticker.Reset(healthInterval)
						continue
					}
				}
			}

			app.g.Update(func(g *gocui.Gui) error {
				if app.state.client != snap.client {
					return nil
				}
				prev := app.state.dirStatus
				app.state.dirStatus = connFailed
				app.state.dirError = err.Error()
				if prev != connFailed {
					app.renderDirectory(g)
				}
				return nil
			})
			ticker.Reset(retryInterval)
			continue
		}

		if snap.cfg == nil {
			ticker.Reset(healthInterval)
			continue
		}

		// No client yet (initial connect failed) — for OIDC connections,
		// refresh the token before reconnecting so we don't reuse a stale one.
		cfg := *snap.cfg
		if cfg.OIDCIssuer != "" {
			refreshCtx, refreshCancel := context.WithTimeout(context.Background(), pingTimeout)
			token, err := dirclient.TryGetCachedToken(refreshCtx, cfg.OIDCIssuer, cfg.OIDCClientID)
			refreshCancel()
			if err != nil || token == "" {
				// No valid cached token — trigger interactive re-auth.
				app.g.Update(func(g *gocui.Gui) error {
					app.state.dirStatus = connFailed
					app.state.dirError = "OIDC token expired"
					app.renderDirectory(g)
					app.stopReconnectLoop()
					go app.connectWithOIDC(snap.activeDir)
					return nil
				})
				return
			}
			cfg.AuthToken = token
		}

		c, err := dirclient.Connect(context.Background(), cfg)
		if err != nil {
			app.g.Update(func(g *gocui.Gui) error {
				prev := app.state.dirStatus
				app.state.dirStatus = connFailed
				app.state.dirError = err.Error()
				if prev != connFailed {
					app.renderDirectory(g)
				}
				return nil
			})
			ticker.Reset(retryInterval)
			continue
		}

		app.g.Update(func(g *gocui.Gui) error {
			if app.state.client != nil {
				c.Close()
				return nil
			}
			c.FirstPageSize = app.cfg.FirstPageSize
			c.BatchSize = app.cfg.BatchSize
			app.state.client = c
			app.state.dirStatus = connOK
			app.state.dirLastConnected = time.Now()
			app.state.dirError = ""
			if app.state.infoPopupPanel == viewDirectory {
				_ = app.closeInfoPopup(g, nil)
			}
			app.renderDirectory(g)
			app.startRecordsStream()
			return nil
		})
		ticker.Reset(healthInterval)
	}
}

// tryOIDCTokenRefresh attempts to silently refresh the OIDC token from the
// local cache and reconnect with the new token. Returns true if the refresh
// succeeded and the connection was re-established. Never wipes cached records.
func (app *Gui) tryOIDCTokenRefresh(
	snap struct {
		client    *dirclient.Client
		cfg       *dirclient.Config
		status    connStatus
		activeDir config.DirectoryEntry
	},
) bool {
	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), pingTimeout)
	token, err := dirclient.TryGetCachedToken(refreshCtx, snap.cfg.OIDCIssuer, snap.cfg.OIDCClientID)
	refreshCancel()
	if err != nil || token == "" {
		return false
	}

	// If the cached token is the same one we already have, refreshing
	// won't help — the failure is not auth-related.
	if token == snap.cfg.AuthToken {
		return false
	}

	cfg := *snap.cfg
	cfg.AuthToken = token

	c, err := dirclient.Connect(context.Background(), cfg)
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	pingErr := c.Ping(ctx)
	cancel()
	if pingErr != nil {
		c.Close()
		return false
	}

	app.g.Update(func(g *gocui.Gui) error {
		if app.state.client != snap.client {
			c.Close()
			return nil
		}
		snap.client.Close()
		c.FirstPageSize = app.cfg.FirstPageSize
		c.BatchSize = app.cfg.BatchSize
		app.state.client = c
		app.state.dirLastCfg = &cfg
		app.state.dirStatus = connOK
		app.state.dirLastConnected = time.Now()
		app.state.dirError = ""
		app.renderDirectory(g)
		return nil
	})
	return true
}

// startOASFReconnectLoop starts the OASF health/reconnect loop.
func (app *Gui) startOASFReconnectLoop() {
	if app.state.oasfReconnStop != nil {
		return
	}
	stop := make(chan struct{})
	app.state.oasfReconnStop = stop
	go app.oasfHealthLoop(stop)
}

func (app *Gui) stopOASFReconnectLoop() {
	if app.state.oasfReconnStop != nil {
		close(app.state.oasfReconnStop)
		app.state.oasfReconnStop = nil
	}
}

func (app *Gui) oasfHealthLoop(stop chan struct{}) {
	interval := healthInterval
	if app.state.oasfStatus == connFailed {
		interval = retryInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		client := app.state.oasfClient
		if client == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		err := client.Ping(ctx)
		cancel()

		app.g.Update(func(g *gocui.Gui) error {
			if app.state.oasfClient != client {
				return nil
			}
			prev := app.state.oasfStatus
			if err == nil {
				app.state.oasfStatus = connOK
				app.state.oasfLastConnected = time.Now()
				app.state.oasfError = ""
			} else {
				app.state.oasfStatus = connFailed
				app.state.oasfError = err.Error()
			}
			if app.state.oasfStatus != prev {
				app.renderDirectory(g)
			}
			return nil
		})

		if err == nil {
			ticker.Reset(healthInterval)
		} else {
			ticker.Reset(retryInterval)
		}
	}
}

// pingOASF does the initial OASF connectivity check and starts the health loop.
func (app *Gui) pingOASF(client *oasf.Client) {
	if client == nil {
		return
	}
	err := client.Ping(context.Background())
	app.g.Update(func(g *gocui.Gui) error {
		if app.state.oasfClient != client {
			return nil
		}
		if err == nil {
			app.state.oasfStatus = connOK
			app.state.oasfLastConnected = time.Now()
			app.state.oasfError = ""
		} else {
			app.state.oasfStatus = connFailed
			app.state.oasfError = err.Error()
		}
		app.renderDirectory(g)
		app.startOASFReconnectLoop()
		return nil
	})
}

// startRecordsStream cancels any in-flight records stream and issues a fresh
// unfiltered SearchRecords RPC to populate the full cache. Client-side filters
// are re-applied once the cache is populated. It must run on the GUI goroutine
// (i.e. inside a g.Update callback or a key handler) because it touches state
// without taking state.mu.
func (app *Gui) startRecordsStream() {
	if app.state.client == nil {
		return
	}

	if app.state.cancelLoad != nil {
		app.state.cancelLoad()
		app.state.cancelLoad = nil
	}

	app.state.fullCache = nil
	app.state.records = nil
	app.state.recordCursor = 0
	app.state.streamErr = ""
	app.state.stream = streamLoading
	app.state.recordInfoCID = ""
	app.state.recordInfoText = ""
	app.state.recordInfoLoading = false
	app.applyNameFilter()
	app.renderRecordsView(app.g)
	app.renderDirectory(app.g)

	ctx, cancel := context.WithCancel(context.Background())
	app.state.cancelLoad = cancel

	client := app.state.client

	go client.Stream(ctx, nil, dirclient.StreamCallbacks{
		OnFirstPage: func(summaries []*dirclient.RecordSummary) {
			app.g.Update(func(g *gocui.Gui) error {
				if ctx.Err() != nil {
					return nil
				}
				if len(summaries) > 0 {
					app.state.dirStatus = connOK
					app.state.dirLastConnected = time.Now()
					app.state.dirError = ""
					if app.state.infoPopupPanel == viewDirectory {
						_ = app.closeInfoPopup(g, nil)
					}
				}
				app.state.fullCache = append(app.state.fullCache, summaries...)
				for _, r := range summaries {
					app.state.filterValues.add(r)
				}
				app.maybeStartClassEntriesFetch(summaries)
				app.state.stream = streamStreaming
				app.applyFilters()
				app.renderRecordsView(g)
				app.renderFiltersView(g)
				app.renderDirectory(g)
				app.autoPreviewRecord(g)
				return nil
			})
		},
		OnBatch: func(batch []*dirclient.RecordSummary) {
			app.g.Update(func(g *gocui.Gui) error {
				if ctx.Err() != nil {
					return nil
				}
				app.state.fullCache = append(app.state.fullCache, batch...)
				for _, r := range batch {
					app.state.filterValues.add(r)
				}
				app.applyFilters()
				app.renderRecordsView(g)
				app.renderFiltersView(g)
				return nil
			})
		},
		OnDone: func(err error) {
			app.g.Update(func(g *gocui.Gui) error {
				if ctx.Err() != nil {
					return nil
				}
				if err != nil {
					app.state.stream = streamErrored
					app.state.streamErr = err.Error()
					app.state.dirStatus = connFailed
					app.state.dirError = err.Error()
					app.state.recordInfoCID = ""
					app.state.recordInfoText = err.Error()
					app.state.recordInfoError = true
					app.state.recordInfoLoading = false
					app.openInfoPopup(g, viewRecords)
					app.renderDirectory(g)
					app.startReconnectLoop()
				} else {
					app.state.stream = streamDone
					if app.state.dirStatus == connTrying {
						app.state.dirStatus = connOK
						app.state.dirLastConnected = time.Now()
						app.state.dirError = ""
					}
					app.startReconnectLoop()
				}
				app.renderRecordsView(g)
				app.renderDirectory(g)
				return nil
			})
		},
	})
}

// applyFilters narrows state.records from fullCache according to the active
// selections (skills, domains, modules, version, schema version, author),
// then chains into applyNameFilter for the local name query. Trusted/Verified
// filters are not applicable client-side (they require server evaluation), so
// when those are active a fresh server stream is issued instead.
//
// When resetCursor is true the selection resets to the first row and the
// preview updates (used after explicit user actions). When false the cursor
// is clamped to remain valid without jumping (used during background sync).
//
// Must be called from the GUI goroutine (g.Update callback or key handler).
func (app *Gui) applyFilters() {
	app.applyFiltersOpts(true)
}

// applyFiltersSilent is like applyFilters but preserves the cursor position.
// Used during background sync reconciliation to avoid resetting the user's selection.
func (app *Gui) applyFiltersSilent() {
	app.applyFiltersOpts(false)
}

func (app *Gui) applyFiltersOpts(resetCursor bool) {
	applied := app.state.filters.applied

	if len(applied[filterTrustedVerified]) > 0 {
		app.applyFiltersServerSide()
		return
	}

	if len(applied) == 0 {
		app.state.records = app.state.fullCache
		app.state.activeFilterValues = nil
	} else {
		out := make([]*dirclient.RecordSummary, 0, len(app.state.fullCache))
		for _, r := range app.state.fullCache {
			if matchesFilters(r, applied) {
				out = append(out, r)
			}
		}
		app.state.records = out
		app.rebuildActiveFilterValues()
	}

	if resetCursor {
		app.state.recordCursor = 0
	}
	app.applyNameFilter()
	if !resetCursor {
		if max := len(app.state.recordDisplayRows) - 1; app.state.recordCursor > max && max >= 0 {
			app.state.recordCursor = max
		}
	}

	app.renderRecordsView(app.g)
	app.renderFiltersView(app.g)
	if resetCursor {
		app.autoPreviewRecord(app.g)
	}
}

// applyFiltersServerSide falls back to a server-side stream when filters
// that can't be evaluated client-side (Trusted/Verified) are active. Unlike
// startRecordsStream it does NOT clear or repopulate fullCache.
func (app *Gui) applyFiltersServerSide() {
	if app.state.client == nil {
		return
	}
	if app.state.cancelLoad != nil {
		app.state.cancelLoad()
		app.state.cancelLoad = nil
	}

	app.state.records = nil
	app.state.recordCursor = 0
	app.state.streamErr = ""
	app.state.stream = streamLoading
	app.applyNameFilter()
	app.renderRecordsView(app.g)

	ctx, cancel := context.WithCancel(context.Background())
	app.state.cancelLoad = cancel
	queries := app.activeQueries()
	client := app.state.client

	go client.Stream(ctx, queries, dirclient.StreamCallbacks{
		OnFirstPage: func(summaries []*dirclient.RecordSummary) {
			app.g.Update(func(g *gocui.Gui) error {
				if ctx.Err() != nil {
					return nil
				}
				app.state.records = append(app.state.records, summaries...)
				app.state.stream = streamStreaming
				app.applyNameFilter()
				app.renderRecordsView(g)
				app.autoPreviewRecord(g)
				return nil
			})
		},
		OnBatch: func(batch []*dirclient.RecordSummary) {
			app.g.Update(func(g *gocui.Gui) error {
				if ctx.Err() != nil {
					return nil
				}
				app.state.records = append(app.state.records, batch...)
				app.applyNameFilter()
				app.renderRecordsView(g)
				return nil
			})
		},
		OnDone: func(err error) {
			app.g.Update(func(g *gocui.Gui) error {
				if ctx.Err() != nil {
					return nil
				}
				if err != nil {
					app.state.stream = streamErrored
					app.state.streamErr = err.Error()
					app.state.recordInfoCID = ""
					app.state.recordInfoText = err.Error()
					app.state.recordInfoError = true
					app.state.recordInfoLoading = false
					app.openInfoPopup(g, viewRecords)
				} else {
					app.rebuildActiveFilterValues()
					app.renderFiltersView(g)
					app.state.stream = streamDone
				}
				app.renderRecordsView(g)
				return nil
			})
		},
	})
}

// matchesFilters checks whether a single record matches all the applied filter
// selections. Within each category the semantics are OR (match any selected
// value); across categories the semantics are AND (all categories must match).
// Trusted/Verified are excluded — they require server evaluation.
func matchesFilters(r *dirclient.RecordSummary, applied map[filterCategory]map[string]bool) bool {
	for cat, selected := range applied {
		if len(selected) == 0 {
			continue
		}
		if cat == filterTrustedVerified {
			continue
		}
		if !matchesCategory(r, cat, selected) {
			return false
		}
	}
	return true
}

func matchesCategory(r *dirclient.RecordSummary, cat filterCategory, selected map[string]bool) bool {
	switch cat {
	case filterSkills:
		return sliceMatchesAll(r.Skills, selected)
	case filterDomains:
		return sliceMatchesAll(r.Domains, selected)
	case filterModules:
		return sliceMatchesAll(r.Modules, selected)
	case filterOASFVersion:
		return selected[r.SchemaVersion]
	case filterVersion:
		return selected[r.Version]
	case filterAuthor:
		return sliceMatchesAll(r.Authors, selected)
	}
	return true
}

// sliceMatchesAll returns true only if values contains every key in selected.
func sliceMatchesAll(values []string, selected map[string]bool) bool {
	for k := range selected {
		found := false
		for _, v := range values {
			if v == k {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// applyNameFilter recomputes filteredRecords from records by applying only
// the local name query. The records slice has already been narrowed by
// applyFilters; the name query is intentionally local so the user can narrow
// incrementally without restarting the stream.
//
// Must be called from the GUI goroutine (g.Update callback or key handler).
func (app *Gui) applyNameFilter() {
	if app.state.filterQuery == "" {
		app.state.filteredRecords = app.state.records
	} else {
		q := strings.ToLower(app.state.filterQuery)
		out := make([]*dirclient.RecordSummary, 0, len(app.state.records))
		for _, r := range app.state.records {
			if strings.Contains(strings.ToLower(r.Name), q) {
				out = append(out, r)
			}
		}
		app.state.filteredRecords = out
	}
	app.buildRecordDisplayRows()
}

// syncCounts returns the number of records in fullCache that are syncing vs reconciling.
func (app *Gui) syncCounts() (syncing, reconciling int) {
	for _, r := range app.state.fullCache {
		switch r.Status {
		case dirclient.StatusSyncing:
			syncing++
		case dirclient.StatusReconciling:
			reconciling++
		}
	}
	return
}

// hasSyncingRecords returns true if any record in fullCache is still syncing or reconciling.
func (app *Gui) hasSyncingRecords() bool {
	s, r := app.syncCounts()
	return s+r > 0
}

// clearSyncState resets all sync-related tracking fields.
func (app *Gui) clearSyncState() {
	app.state.syncID = ""
	app.state.syncCancelFunc = nil
	app.state.syncCIDs = nil
}

// clearClipboard resets all clipboard-related fields.
func (app *Gui) clearClipboard() {
	app.state.clipboard = nil
	app.state.clipboardSource = ""
	app.state.clipboardSourceURL = ""
}

// buildRecordDisplayRows computes the grouped display rows from
// filteredRecords. Records sharing the same Name are grouped together with a
// collapsible header (similar to filter categories). Groups with a single
// record are shown flat without a header.
func (app *Gui) buildRecordDisplayRows() {
	records := app.state.filteredRecords
	if len(records) == 0 {
		app.state.recordDisplayRows = nil
		return
	}

	if app.state.recordGroupExpanded == nil {
		app.state.recordGroupExpanded = map[string]bool{}
	}

	// Build groups keyed by name, then sort alphabetically.
	type group struct {
		name    string
		records []*dirclient.RecordSummary
	}
	seen := map[string]int{} // name -> index into groups
	var groups []group
	for _, r := range records {
		name := r.Name
		if name == "" {
			name = r.CID
		}
		if idx, ok := seen[name]; ok {
			groups[idx].records = append(groups[idx].records, r)
		} else {
			seen[name] = len(groups)
			groups = append(groups, group{name: name, records: []*dirclient.RecordSummary{r}})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return strings.ToLower(groups[i].name) < strings.ToLower(groups[j].name)
	})

	var rows []recordDisplayRow
	for _, g := range groups {
		if len(g.records) == 1 {
			rows = append(rows, recordDisplayRow{record: g.records[0]})
			continue
		}
		sort.Slice(g.records, func(i, j int) bool {
			return compareVersions(g.records[i].Version, g.records[j].Version) > 0
		})
		rows = append(rows, recordDisplayRow{groupName: g.name})
		if app.state.recordGroupExpanded[g.name] {
			for _, r := range g.records {
				rows = append(rows, recordDisplayRow{record: r, grouped: true})
			}
		}
	}

	app.state.recordDisplayRows = rows
}

// compareVersions compares two version strings. It attempts semver-style
// numeric comparison (splitting on ".") and falls back to lexicographic order.
// Returns >0 if a > b, <0 if a < b, 0 if equal.
func compareVersions(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		var ap, bp string
		if i < len(aParts) {
			ap = aParts[i]
		}
		if i < len(bParts) {
			bp = bParts[i]
		}
		an, aErr := strconv.Atoi(ap)
		bn, bErr := strconv.Atoi(bp)
		if aErr == nil && bErr == nil {
			if an != bn {
				return an - bn
			}
			continue
		}
		if ap != bp {
			if ap < bp {
				return -1
			}
			return 1
		}
	}
	return 0
}

// openInput shows the shared input prompt, pre-fills it with initialValue,
// focuses it, and wires confirm/cancel/change callbacks. When onChange is
// non-nil the filter is applied live (debounced) as the user types.
func (app *Gui) openInput(title, initialValue string, onConfirm func(string), onCancel func(), onChange func(string)) {
	iv, err := app.g.View(viewInput)
	if err != nil {
		return
	}

	// Save the currently focused view so we can restore it on dismiss.
	if cv := app.g.CurrentView(); cv != nil {
		app.state.prevView = cv.Name()
	}

	app.state.inputVisible = true
	app.state.inputTitle = title
	app.state.onInputConfirm = onConfirm
	app.state.onInputCancel = onCancel
	app.state.onInputChange = onChange

	if onChange != nil {
		iv.Editor = &liveInputEditor{gui: app}
	} else {
		iv.Editor = gocui.DefaultEditor
	}

	iv.Title = title
	iv.Visible = true
	iv.Clear()
	iv.TextArea.Clear()
	iv.TextArea.TypeString(initialValue)
	iv.RenderTextArea()

	_, _ = app.g.SetCurrentView(viewInput)
}

// closeInput hides the input prompt and restores focus to the previous view.
func (app *Gui) closeInput() {
	if app.state.inputDebounce != nil {
		app.state.inputDebounce.Stop()
		app.state.inputDebounce = nil
	}
	app.state.onInputChange = nil

	iv, err := app.g.View(viewInput)
	if err != nil {
		return
	}
	iv.Visible = false
	iv.Editor = nil
	app.state.inputVisible = false

	target := app.state.prevView
	if target == "" {
		target = viewRecords
	}
	app.state.prevView = ""
	_ = app.focusTo(app.g, target)
}

// liveInputEditor wraps the default text-area editor and schedules a
// debounced onChange callback whenever the content changes.
type liveInputEditor struct {
	gui *Gui
}

func (e *liveInputEditor) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) bool {
	before := v.TextArea.GetContent()
	ret := gocui.DefaultEditor.Edit(v, key, ch, mod)
	if v.TextArea.GetContent() != before {
		e.gui.scheduleInputChange()
	}
	return ret
}

const defaultInputDebounceDelay = 150

// maybeStartClassEntriesFetch kicks off a background taxonomy fetch for
// skills/domains/modules when the first schema version is discovered in
// the record stream. It is a no-op if the fetch has already been started.
func (app *Gui) maybeStartClassEntriesFetch(summaries []*dirclient.RecordSummary) {
	if app.state.classEntriesVer != "" {
		return
	}
	for _, r := range summaries {
		if r.SchemaVersion != "" {
			app.state.classEntriesVer = r.SchemaVersion
			go app.fetchClassEntries(r.SchemaVersion)
			return
		}
	}
}

// fetchClassEntries fetches the full OASF taxonomy for all three class types
// (skills, domains, modules) and stores the flattened entries in app state.
func (app *Gui) fetchClassEntries(schemaVersion string) {
	client := app.state.oasfClient
	if client == nil {
		return
	}
	ctx := context.Background()

	for _, ct := range []oasf.ClassType{oasf.ClassTypeSkill, oasf.ClassTypeDomain, oasf.ClassTypeModule} {
		entries, err := client.FetchAll(ctx, ct, schemaVersion)
		if err != nil {
			continue
		}
		ct := ct
		app.g.Update(func(g *gocui.Gui) error {
			if app.state.classEntries == nil {
				app.state.classEntries = map[oasf.ClassType]map[string]oasf.ClassEntry{}
			}
			app.state.classEntries[ct] = entries
			app.renderFiltersView(g)
			app.refreshPreviewTree(g)
			return nil
		})
	}
}

// scheduleInputChange resets the debounce timer so the onChange callback
// fires inputDebounceDelay after the last keystroke.
func (app *Gui) scheduleInputChange() {
	if app.state.inputDebounce != nil {
		app.state.inputDebounce.Stop()
	}
	delay := app.cfg.InputDebounceDelay
	if delay <= 0 {
		delay = defaultInputDebounceDelay
	}
	app.state.inputDebounce = time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		app.g.Update(func(g *gocui.Gui) error {
			if app.state.onInputChange == nil {
				return nil
			}
			iv, err := g.View(viewInput)
			if err != nil {
				return nil
			}
			app.state.onInputChange(strings.TrimSpace(iv.TextArea.GetContent()))
			return nil
		})
	})
}
