package gui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/akijakya/lazydir/internal/config"
	"github.com/akijakya/lazydir/internal/dirclient"
	"github.com/akijakya/lazydir/internal/oasf"
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

	// records: server already filtered them; we render this slice directly
	// (after the optional name query in filterQuery is applied).
	records         []*dirclient.RecordSummary
	filteredRecords []*dirclient.RecordSummary
	recordCursor    int

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

	// preview dimming: stored content so we can toggle dim without refetching
	previewSubtitle string
	previewContent  string // rendered (ANSI-colored) content
	previewDimmed   bool   // true when the preview is currently showing dimmed
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
			dirCfg.AuthMode = "insecure"
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

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}

		type snapshot struct {
			client *dirclient.Client
			cfg    *dirclient.Config
			status connStatus
		}
		ch := make(chan snapshot, 1)
		app.g.Update(func(g *gocui.Gui) error {
			ch <- snapshot{
				client: app.state.client,
				cfg:    app.state.dirLastCfg,
				status: app.state.dirStatus,
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

			app.g.Update(func(g *gocui.Gui) error {
				if app.state.client != snap.client {
					return nil
				}
				prev := app.state.dirStatus
				if err == nil {
					app.state.dirStatus = connOK
					app.state.dirLastConnected = time.Now()
					app.state.dirError = ""
				} else {
					app.state.dirStatus = connFailed
					app.state.dirError = err.Error()
				}
				if app.state.dirStatus != prev {
					app.renderDirectory(g)
				}
				if err == nil && prev != connOK && len(app.state.records) == 0 {
					app.startRecordsStream()
				}
				return nil
			})

			if err == nil {
				ticker.Reset(healthInterval)
			} else {
				ticker.Reset(retryInterval)
			}
			continue
		}

		if snap.cfg == nil {
			ticker.Reset(healthInterval)
			continue
		}

		// No client yet (initial connect failed) — try to establish one.
		c, err := dirclient.Connect(context.Background(), *snap.cfg)
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
// SearchRecords RPC for the current filter selection. It must run on the GUI
// goroutine (i.e. inside a g.Update callback or a key handler) because it
// touches state without taking state.mu.
func (app *Gui) startRecordsStream() {
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
	app.state.recordInfoCID = ""
	app.state.recordInfoText = ""
	app.state.recordInfoLoading = false
	app.applyNameFilter()
	app.renderRecordsView(app.g)
	app.renderDirectory(app.g)

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
				if len(summaries) > 0 {
					app.state.dirStatus = connOK
					app.state.dirLastConnected = time.Now()
					app.state.dirError = ""
					if app.state.infoPopupPanel == viewDirectory {
						_ = app.closeInfoPopup(g, nil)
					}
				}
				app.state.records = append(app.state.records, summaries...)
				for _, r := range summaries {
					app.state.filterValues.add(r)
				}
				app.maybeStartClassEntriesFetch(summaries)
				app.state.stream = streamStreaming
				app.applyNameFilter()
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
				app.state.records = append(app.state.records, batch...)
				for _, r := range batch {
					app.state.filterValues.add(r)
				}
				app.applyNameFilter()
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
					app.renderDirectory(g)
					app.startReconnectLoop()
				} else {
					app.state.stream = streamDone
					app.startReconnectLoop()
				}
				app.renderRecordsView(g)
				app.renderDirectory(g)
				return nil
			})
		},
	})
}

// applyNameFilter recomputes filteredRecords from records by applying only
// the local name query. Server-side filters have already been applied to
// records by the time they reach us; the name query is intentionally local
// so the user can narrow incrementally without restarting the stream.
//
// Must be called from the GUI goroutine (g.Update callback or key handler).
func (app *Gui) applyNameFilter() {
	if app.state.filterQuery == "" {
		app.state.filteredRecords = app.state.records
		return
	}
	q := strings.ToLower(app.state.filterQuery)
	out := make([]*dirclient.RecordSummary, 0, len(app.state.records))
	for _, r := range app.state.records {
		if strings.Contains(strings.ToLower(r.Name), q) {
			out = append(out, r)
		}
	}
	app.state.filteredRecords = out
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
