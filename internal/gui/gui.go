// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
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

// menuOption describes one selectable row in a popup option menu.
type menuOption struct {
	label  string
	action func()
}

// menuState tracks the cursor and options for a navigable popup menu.
type menuState struct {
	cursor  int
	options []menuOption
	view    string // the gocui view name this menu is rendered in
}

// syncStatusEntry is a single transient sync-status overlay record: the
// in-flight status of a synced CID plus an optional error message.
type syncStatusEntry struct {
	status dirclient.RecordStatus
	err    string
}

// appState holds all mutable application state. Fields are only mutated on
// the GUI goroutine (inside g.Update callbacks or key handlers).
type appState struct {
	// When the currently-displayed records were fetched from the directory.
	// Set on a fresh stream completion and preserved across cache round-trips
	// so the records panel can show how stale the data is. Zero if never loaded.
	dataFetchedAt time.Time

	// Publish enrichment. RecordSummary.Published is filled in the background
	// from RoutingService.List after the first page lands. publishEnriched is
	// true once resolution completed for the current server; publishEnriching
	// guards against concurrent resolutions; publishCancel stops an in-flight
	// resolution.
	publishEnriched  bool
	publishEnriching bool
	publishCancel    context.CancelFunc
	// publishOverrides holds optimistic publish/unpublish results (CID → desired
	// Published state) that must survive an enrichment whose ListPublished
	// snapshot predates the change. Each entry is dropped once the server set
	// agrees with it (self-healing). Cleared on server switch.
	publishOverrides map[string]bool
	// publishedCIDs is the last-known published set from enrichment, retained so
	// pages appended later by loadNextPage (or a rebuild) can be colored without
	// re-querying the routing service. Cleared on server switch.
	publishedCIDs map[string]bool

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

	// syncStatus is a transient per-CID overlay of in-flight sync state, keyed by
	// record CID. It decorates matching rows in the current page; it is NOT a
	// record store. Cleared when a reconcile refetch lands or on server switch.
	syncStatus map[string]syncStatusEntry

	recordCursor int

	// recordDisplayRows are the flat display rows for the records panel: one
	// row per record in the current page (no version grouping).
	recordDisplayRows []recordDisplayRow

	// page holds the server-side pagination state for the records panel
	// (accumulated pages, offset, total, exhausted/loading flags).
	page pageState

	// queryGen is a monotonic query generation; loadNextPage drops results
	// from a superseded query.
	queryGen uint64

	// loadedOnce is true once a query has completed (first page landed), used
	// to gate the "synced N ago" staleness indicator.
	loadedOnce bool
	cancelLoad context.CancelFunc

	// inline record info toggle (records panel)
	recordInfoCID     string // CID of the record whose info is expanded, "" if none
	recordInfoText    string // cached description text
	recordInfoError   bool   // true when recordInfoText is an error message
	recordInfoLoading bool   // fetch in progress

	// distinct values usable as filter options for each category, as returned
	// by the server's ListFilterValues RPC (complete and present-only). Empty
	// until the first successful fetch, or when the server does not support it.
	filterValues *filterValueAggregator

	// ListFilterValues fetch state, reset per connection. filterValuesLoaded is
	// true once a fetch succeeded; filterValuesLoading guards against duplicate
	// fetches; filterValuesFailed records that the last fetch failed so
	// non-forced triggers do not retry it on every query (an older server
	// returns Unimplemented forever) and the Filters panel title can show a
	// notice ("unavailable" if nothing was ever loaded, "stale" if earlier
	// values are kept). filterValuesCancel stops an in-flight fetch.
	filterValuesLoaded  bool
	filterValuesLoading bool
	filterValuesFailed  bool
	filterValuesCancel  context.CancelFunc

	// classEntries caches enriched display info (ID, caption) for OASF
	// taxonomy classes. Records may span multiple OASF schema versions, so
	// entries are merged from every version seen in loaded pages or returned by
	// ListFilterValues; classEntriesVers tracks which versions have already
	// been fetched (or are in flight).
	classEntries     map[oasf.ClassType]map[string]oasf.ClassEntry
	classEntriesVers map[string]bool

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

	// generic option menu state (used by copy menu, confirm popup, etc.)
	menu menuState

	// confirmation popup state
	confirmPopupText string // rendered text shown inside the confirm popup

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
	PageSize           int
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

	// Refresh the records panel on a timer so the "synced N ago" staleness
	// indicator in its title stays current. Stopped when the main loop exits.
	uiStop := make(chan struct{})
	defer close(uiStop)
	go app.uiRefreshLoop(uiStop)

	if err := g.MainLoop(); !gocui.IsQuit(err) {
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
			app.state.loadedOnce = false
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
		app.state.client = c
		app.state.serverAddr = cfg.ServerAddress
		app.state.authMode = cfg.AuthMode
		app.state.dirLastCfg = &cfg
		app.state.dirStatus = connTrying
		app.renderDirectory(g)
		app.renderStatus(g)
		app.startQuery(true)
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

// uiRefreshLoop keeps the records panel's relative "synced N ago" staleness
// indicator current. It only rewrites the panel title (never clears/repaints
// the record list) and only when the title actually changes, so it stays cheap
// even for large lists. It runs for the lifetime of the GUI and exits when stop
// is closed.
func (app *Gui) uiRefreshLoop(stop chan struct{}) {
	// Coarse (minute-granular) label, so a low-frequency tick is plenty.
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			app.g.Update(func(g *gocui.Gui) error {
				if !app.state.loadedOnce || app.state.dataFetchedAt.IsZero() {
					return nil
				}
				v, err := g.View(viewRecords)
				if err != nil {
					return nil
				}
				if title := app.recordsTitle(); title != v.Title {
					v.Title = title
				}
				return nil
			})
		}
	}
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
					if prev != connOK && len(app.state.page.records) == 0 {
						app.startQuery(true)
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
			app.state.client = c
			app.state.dirStatus = connOK
			app.state.dirLastConnected = time.Now()
			app.state.dirError = ""
			if app.state.infoPopupPanel == viewDirectory {
				_ = app.closeInfoPopup(g, nil)
			}
			app.renderDirectory(g)
			app.startQuery(true)
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

// startQuery resets pagination and fetches the first page plus the total for
// the current filter + name-query selection. Runs the fetch on a background
// goroutine; results are applied on the GUI goroutine via g.Update.
//
// When resetCursor is true the selection resets to the first row; when false
// the current recordCursor is preserved (clamped by rebuildRecordRows). Both
// perform the same fetch.
func (app *Gui) startQuery(resetCursor bool) {
	client := app.state.client
	if client == nil {
		return
	}
	queries := buildServerQueries(app.state.filters, app.state.filterQuery)
	pageSize := app.pageSize()

	app.state.page.reset()
	app.state.queryGen++
	app.state.page.loading = true
	if resetCursor {
		app.state.recordCursor = 0
	}

	if app.state.cancelLoad != nil {
		app.state.cancelLoad()
	}
	ctx, cancel := context.WithCancel(context.Background())
	app.state.cancelLoad = cancel

	go func() {
		recs, exhausted, err := client.Page(ctx, queries, uint32(pageSize), 0)
		total, cntErr := client.Count(ctx, queries)
		app.g.Update(func(g *gocui.Gui) error {
			if ctx.Err() != nil {
				return nil // superseded by a newer query
			}
			app.state.page.loading = false
			if err != nil {
				// The query failed: surface it and start the health/reconnect
				// loop so the connection can recover (mirrors the old stream's
				// OnDone error branch).
				app.state.dirStatus = connFailed
				app.state.dirError = err.Error()
				app.startReconnectLoop()
				app.renderRecordsView(g)
				app.renderDirectory(g)
				return nil
			}
			app.state.page.appendPage(recs, exhausted)
			if cntErr == nil {
				app.state.page.total = total
				app.state.page.totalKnown = true
			}
			// A successful first page confirms the connection is healthy.
			// startQuery replaced the old records stream, so the side effects
			// that used to live in its OnDone (mark connected, start the
			// health/reconnect loop, publish enrichment, staleness stamp) are
			// re-established here. Both loops are idempotent (guarded against
			// duplicate goroutines), so firing them on every query is safe.
			app.state.dirStatus = connOK
			app.state.dirLastConnected = time.Now()
			app.state.dirError = ""
			if app.state.infoPopupPanel == viewDirectory {
				_ = app.closeInfoPopup(g, nil)
			}
			app.state.loadedOnce = true
			app.state.dataFetchedAt = time.Now()
			app.startReconnectLoop()
			app.startPublishEnrichment()
			app.startFilterValuesFetch(false)
			app.maybeStartClassEntriesFetch(recs)
			app.rebuildRecordRows() // flat renderer
			app.renderRecordsView(g)
			app.renderFiltersView(g)
			app.renderDirectory(g)
			// Only the cursor-resetting (explicit) path moves the preview;
			// the resetCursor=false path (background reconciliation) stays silent.
			if resetCursor {
				app.autoPreviewRecord(g)
			}
			return nil
		})
	}()
}

// loadNextPage fetches and appends the next page unless already loading or
// exhausted. Wired to the records-panel scroll trigger.
func (app *Gui) loadNextPage() {
	client := app.state.client
	if client == nil || app.state.page.loading || app.state.page.exhausted {
		return
	}
	queries := buildServerQueries(app.state.filters, app.state.filterQuery)
	pageSize := app.pageSize()
	offset := app.state.page.offset
	app.state.page.loading = true

	// Next-page fetches are not cancelled by cursor moves — only superseded by
	// a new startQuery. A superseded fetch is detected via the queryGen guard
	// below (rather than context cancellation), so this fetch uses a
	// background context.
	ctx := context.Background()
	gen := app.state.queryGen

	go func() {
		recs, exhausted, err := client.Page(ctx, queries, uint32(pageSize), offset)
		app.g.Update(func(g *gocui.Gui) error {
			if gen != app.state.queryGen {
				return nil // superseded by a newer query; drop these results
			}
			app.state.page.loading = false
			if err != nil {
				app.renderRecordsView(g)
				return nil
			}
			app.state.page.appendPage(recs, exhausted)
			app.maybeStartClassEntriesFetch(recs)
			app.rebuildRecordRows()
			app.renderRecordsView(g)
			app.renderFiltersView(g)
			return nil
		})
	}()
}

// pageSize returns the configured page size, defaulting defensively.
func (app *Gui) pageSize() int {
	if app.cfg.PageSize > 0 {
		return app.cfg.PageSize
	}
	return 50
}

// applyFilters re-runs the server-side query for the current selection,
// resetting the cursor to the first row.
func (app *Gui) applyFilters() { app.startQuery(true) }

// startFilterValuesFetch fetches the complete, present-only filter option
// values from the server's ListFilterValues RPC in the background and replaces
// app.state.filterValues with the result. Must be called on the GUI goroutine.
//
// Non-forced calls (the per-query trigger) are no-ops once values are loaded,
// while a fetch is in flight, or after a failure on the current connection, so
// an older server without the RPC is asked once per connection, not per query.
// Forced calls (refresh, delete, post-sync reconcile) always fetch: an
// in-flight fetch is cancelled and restarted so the result reflects the latest
// server state.
//
// On failure the connection status is left untouched and the Filters panel
// title shows a short notice (see applyFilterValuesResult for what happens to
// the options).
func (app *Gui) startFilterValuesFetch(force bool) {
	client := app.state.client
	if client == nil {
		return
	}
	if !force && (app.state.filterValuesLoaded || app.state.filterValuesLoading || app.state.filterValuesFailed) {
		return
	}
	if app.state.filterValuesCancel != nil {
		app.state.filterValuesCancel()
	}
	app.state.filterValuesLoading = true
	ctx, cancel := context.WithCancel(context.Background())
	app.state.filterValuesCancel = cancel

	go func() {
		values, err := client.ListFilterValues(ctx)

		app.g.Update(func(g *gocui.Gui) error {
			if ctx.Err() != nil {
				return nil // superseded by a newer fetch or a server switch
			}
			app.state.filterValuesLoading = false
			app.state.filterValuesCancel = nil
			cancel()
			next, loaded, failed := applyFilterValuesResult(app.state.filterValues, app.state.filterValuesLoaded, values, err)
			app.state.filterValues = next
			app.state.filterValuesLoaded = loaded
			app.state.filterValuesFailed = failed
			if err != nil {
				app.renderFiltersView(g)
				return nil
			}
			// Resolve captions for every schema version on the server, not just
			// the ones carried by loaded pages.
			app.startClassEntriesFetchFor(values[dirclient.FilterSchemaVersion])
			app.renderFiltersView(g)
			return nil
		})
	}()
}

// applyFilterValuesResult computes the next filter-option state from a
// ListFilterValues result. On success the options are replaced and the state
// is loaded and not failed. On failure, options already loaded on this
// connection are kept (shown as stale) so a transient error after a refetch
// does not wipe the pick-lists; if nothing was ever loaded (e.g. an older
// server returning Unimplemented) the options are empty.
func applyFilterValuesResult(
	prev *filterValueAggregator, loaded bool, values map[dirclient.FilterCategory][]string, err error,
) (next *filterValueAggregator, nextLoaded, failed bool) {
	if err == nil {
		return newFilterValuesFrom(values), true, false
	}
	if loaded && prev != nil {
		return prev, true, true
	}
	return newFilterValueAggregator(), false, true
}

// filterValuesNotice returns the Filters panel title suffix describing a
// failed ListFilterValues fetch, or "" when the last fetch succeeded.
func filterValuesNotice(loaded, failed bool) string {
	switch {
	case !failed:
		return ""
	case loaded:
		return "  [options stale]"
	default:
		return "  [options unavailable]"
	}
}

// syncCounts returns the number of overlay entries that are syncing vs reconciling.
func (app *Gui) syncCounts() (syncing, reconciling int) {
	for _, e := range app.state.syncStatus {
		switch e.status {
		case dirclient.StatusSyncing:
			syncing++
		case dirclient.StatusReconciling:
			reconciling++
		}
	}
	return
}

// hasSyncingRecords returns true if any overlay entry is still syncing or reconciling.
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

// rebuildRecordRows rebuilds the flat display rows from the current page. One
// row per record, in server (recency) order; no cross-version grouping. The
// cursor is clamped to remain within the new row set.
func (app *Gui) rebuildRecordRows() {
	// Color pages appended after enrichment (and rebuilt pages) from the
	// last-known published set; overrides re-apply any pending optimistic toggle.
	if app.state.publishedCIDs != nil {
		markPublished(app.state.page.records, app.state.publishedCIDs)
		app.applyPublishOverrides(app.state.publishedCIDs)
	}
	rows := make([]recordDisplayRow, 0, len(app.state.page.records))
	for _, r := range app.state.page.records {
		rows = append(rows, recordDisplayRow{record: r})
	}
	app.state.recordDisplayRows = rows
	if max := len(rows) - 1; app.state.recordCursor > max {
		app.state.recordCursor = max
	}
	if app.state.recordCursor < 0 {
		app.state.recordCursor = 0
	}
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
// skills/domains/modules for every OASF schema version present in the given
// summaries that has not already been fetched. Records can span multiple
// schema versions, so each distinct version needs its own taxonomy to resolve
// captions for values that only exist in that version.
//
// It must be called on the GUI goroutine so access to classEntriesVers is safe.
func (app *Gui) maybeStartClassEntriesFetch(summaries []*dirclient.RecordSummary) {
	app.startClassEntriesFetchFor(distinctNewSchemaVersions(summaries, app.state.classEntriesVers))
}

// startClassEntriesFetchFor kicks off a background taxonomy fetch for each
// given OASF schema version that is non-empty and not already fetched (or in
// flight). Must be called on the GUI goroutine.
func (app *Gui) startClassEntriesFetchFor(versions []string) {
	for _, v := range versions {
		if v == "" || app.state.classEntriesVers[v] {
			continue
		}
		if app.state.classEntriesVers == nil {
			app.state.classEntriesVers = map[string]bool{}
		}
		app.state.classEntriesVers[v] = true
		go app.fetchClassEntries(v)
	}
}

// distinctNewSchemaVersions returns the non-empty schema versions present in
// summaries that are absent from the fetched set, each listed once.
func distinctNewSchemaVersions(summaries []*dirclient.RecordSummary, fetched map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range summaries {
		v := r.SchemaVersion
		if v == "" || seen[v] || fetched[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// mergeClassEntries folds src into dst, keeping entries already present in dst.
// Captions for a given taxonomy name are stable across versions, so the first
// version that supplies a name wins and later versions only fill in the gaps.
func mergeClassEntries(dst, src map[string]oasf.ClassEntry) map[string]oasf.ClassEntry {
	if dst == nil {
		dst = make(map[string]oasf.ClassEntry, len(src))
	}
	for k, v := range src {
		if _, ok := dst[k]; !ok {
			dst[k] = v
		}
	}
	return dst
}

// fetchClassEntries fetches the full OASF taxonomy for all three class types
// (skills, domains, modules) and stores the flattened entries in app state.
// On any failure it clears the version from classEntriesVers so a later record
// batch carrying that version re-attempts the fetch, rather than leaving its
// captions permanently unresolved after a transient error.
func (app *Gui) fetchClassEntries(schemaVersion string) {
	client := app.state.oasfClient
	if client == nil {
		app.forgetClassEntriesVersion(schemaVersion)
		return
	}
	ctx := context.Background()

	failed := false
	for _, ct := range []oasf.ClassType{oasf.ClassTypeSkill, oasf.ClassTypeDomain, oasf.ClassTypeModule} {
		entries, err := client.FetchAll(ctx, ct, schemaVersion)
		if err != nil {
			failed = true
			continue
		}
		ct := ct
		app.g.Update(func(g *gocui.Gui) error {
			if app.state.classEntries == nil {
				app.state.classEntries = map[oasf.ClassType]map[string]oasf.ClassEntry{}
			}
			app.state.classEntries[ct] = mergeClassEntries(app.state.classEntries[ct], entries)
			app.renderFiltersView(g)
			app.refreshPreviewTree(g)
			return nil
		})
	}

	if failed {
		app.forgetClassEntriesVersion(schemaVersion)
	}
}

// forgetClassEntriesVersion clears a schema version from the fetched set so the
// next record batch that carries it triggers another taxonomy fetch. It runs on
// the GUI goroutine to keep classEntriesVers accessed from a single goroutine.
func (app *Gui) forgetClassEntriesVersion(schemaVersion string) {
	app.g.Update(func(g *gocui.Gui) error {
		delete(app.state.classEntriesVers, schemaVersion)
		return nil
	})
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
