// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package gui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/agntcy/lazydir/internal/config"
	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/oasf"
	"github.com/jesseduffield/gocui"
)

// ── Connections panel cursor ──────────────────────────────────────────────────

func (app *Gui) connCursorUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.connCursor > 0 {
		app.state.connCursor--
		app.renderDirectory(g)
	}
	return nil
}

func (app *Gui) connCursorDown(g *gocui.Gui, v *gocui.View) error {
	if app.state.connCursor < 1 {
		app.state.connCursor++
		app.renderDirectory(g)
	}
	return nil
}

func (app *Gui) connToggleInfo(g *gocui.Gui, v *gocui.View) error {
	if app.state.infoPopupPanel == viewDirectory {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewDirectory)
	}
	app.openInfoPopup(g, viewDirectory)
	_, _ = g.SetCurrentView(viewInfoPopup)
	return nil
}

func (app *Gui) connDismissInfo(g *gocui.Gui, v *gocui.View) error {
	if app.state.infoPopupPanel == viewDirectory {
		_ = app.closeInfoPopup(g, v)
		return app.focusTo(g, viewDirectory)
	}
	return nil
}

// connInfoText builds the info popup content for the selected connection row.
func (app *Gui) connInfoText() (string, bool) {
	t := app.theme
	var sb strings.Builder
	hasError := false
	if app.state.connCursor == 0 {
		fmt.Fprintf(&sb, "%sServer:%s    %s", t.Color1, t.Reset, app.state.serverAddr)
		if app.state.activeDir.OIDCIssuer != "" {
			fmt.Fprintf(&sb, "\n%sAuth:%s      oidc (%s)", t.Color4, t.Reset, app.state.activeDir.OIDCIssuer)
		} else if app.state.authMode != "" {
			fmt.Fprintf(&sb, "\n%sAuth:%s      %s", t.Color4, t.Reset, app.state.authMode)
		} else {
			fmt.Fprintf(&sb, "\n%sAuth:%s      insecure", t.Color4, t.Reset)
		}
		if !app.state.dirLastConnected.IsZero() {
			fmt.Fprintf(&sb, "\n%sConnected:%s %s", t.Color3, t.Reset, app.state.dirLastConnected.Format("15:04:05"))
		}
		if app.state.dirError != "" {
			fmt.Fprintf(&sb, "\n\n%sError:%s %s", t.Color6, t.Reset, app.state.dirError)
			hasError = true
		}
	} else {
		fmt.Fprintf(&sb, "%sServer:%s    %s", t.Color1, t.Reset, app.state.oasfAddr)
		if !app.state.oasfLastConnected.IsZero() {
			fmt.Fprintf(&sb, "\n%sConnected:%s %s", t.Color3, t.Reset, app.state.oasfLastConnected.Format("15:04:05"))
		}
		if app.state.oasfError != "" {
			fmt.Fprintf(&sb, "\n\n%sError:%s %s", t.Color6, t.Reset, app.state.oasfError)
			hasError = true
		}
	}
	return sb.String(), hasError
}

// ── Server selection popup ───────────────────────────────────────────────────

func (app *Gui) openServerSelectPopup(g *gocui.Gui, v *gocui.View) error {
	var items []string
	if app.state.connCursor == 0 {
		for _, entry := range app.cfg.DirectoryServers {
			label := entry.Address
			if entry.OIDCIssuer != "" {
				label += " (oidc)"
			}
			items = append(items, label)
		}
	} else {
		items = append(items, app.cfg.OASFServers...)
	}
	items = append(items, "Custom...")

	app.state.serverMenuItems = items
	app.state.serverMenuCursor = 0
	app.state.serverMenuVisible = true

	smv, err := g.View(viewServerMenu)
	if err == nil {
		smv.Clear()
		for _, item := range items {
			fmt.Fprintln(smv, " "+item)
		}
		_ = smv.SetCursor(0, 0)
	}
	app.showPopup(g, viewServerMenu)
	return nil
}

func (app *Gui) serverMenuUp(g *gocui.Gui, v *gocui.View) error {
	if app.state.serverMenuCursor > 0 {
		app.state.serverMenuCursor--
		_ = v.SetCursor(0, app.state.serverMenuCursor)
	}
	return nil
}

func (app *Gui) serverMenuDown(g *gocui.Gui, v *gocui.View) error {
	if app.state.serverMenuCursor < len(app.state.serverMenuItems)-1 {
		app.state.serverMenuCursor++
		_ = v.SetCursor(0, app.state.serverMenuCursor)
	}
	return nil
}

func (app *Gui) serverMenuClose(g *gocui.Gui, v *gocui.View) error {
	app.state.serverMenuVisible = false
	app.hidePopup(g, viewServerMenu)
	return nil
}

func (app *Gui) serverMenuSelect(g *gocui.Gui, v *gocui.View) error {
	idx := app.state.serverMenuCursor
	if idx < 0 || idx >= len(app.state.serverMenuItems) {
		return app.serverMenuClose(g, v)
	}

	selected := app.state.serverMenuItems[idx]

	app.state.serverMenuVisible = false
	app.hidePopup(g, viewServerMenu)

	if selected == "Custom..." {
		if app.state.connCursor == 0 {
			return app.openConnectDialog(g, nil)
		}
		return app.openOASFDialog(g, nil)
	}

	if app.state.connCursor == 0 {
		if idx < len(app.cfg.DirectoryServers) {
			app.connectToDirectory(g, app.cfg.DirectoryServers[idx])
		}
	} else {
		if idx < len(app.cfg.OASFServers) {
			app.connectToOASF(g, app.cfg.OASFServers[idx])
		}
	}
	return nil
}

// ── Connection flows ─────────────────────────────────────────────────────────

func (app *Gui) connectToDirectory(g *gocui.Gui, entry config.DirectoryEntry) {
	app.stopReconnectLoop()
	if app.state.cancelLoad != nil {
		app.state.cancelLoad()
		app.state.cancelLoad = nil
	}
	if app.state.client != nil {
		app.state.client.Close()
		app.state.client = nil
	}
	app.state.activeDir = entry
	app.state.serverAddr = entry.Address
	app.state.dirStatus = connTrying
	app.state.stream = streamLoading
	app.state.fullCache = nil
	app.state.records = nil
	app.state.filteredRecords = nil
	app.state.recordDisplayRows = nil
	app.state.recordGroupExpanded = map[string]bool{}
	app.state.recordCursor = 0
	app.state.filters = newFilterState()
	app.state.filterQuery = ""
	app.state.filterValues = newFilterValueAggregator()
	app.state.classEntries = nil
	app.state.classEntriesVer = ""
	app.renderDirectory(g)
	app.renderRecordsView(g)
	app.renderFiltersView(g)

	if entry.OIDCIssuer != "" {
		go app.connectWithOIDC(entry)
	} else {
		cfg := dirclient.Config{
			ServerAddress: entry.Address,
			AuthMode:      "insecure",
			TLSSkipVerify: app.cfg.Directory.TLSSkipVerify,
			TLSCAFile:     app.cfg.Directory.TLSCAFile,
			TLSCertFile:   app.cfg.Directory.TLSCertFile,
			TLSKeyFile:    app.cfg.Directory.TLSKeyFile,
		}
		go app.connect(cfg)
	}
}

func (app *Gui) connectWithOIDC(entry config.DirectoryEntry) {
	ctx := context.Background()

	writer := &oidcDeviceWriter{gui: app.g, app: app}

	token, err := dirclient.EnsureOIDCToken(ctx, entry.OIDCIssuer, entry.OIDCClientID, writer)

	app.g.Update(func(g *gocui.Gui) error {
		app.closeAuthPopup(g)
		return nil
	})

	if err != nil {
		app.g.Update(func(g *gocui.Gui) error {
			app.state.dirStatus = connFailed
			app.state.dirError = err.Error()
			app.state.stream = streamIdle
			app.renderDirectory(g)
			app.openInfoPopup(g, viewDirectory)
			return nil
		})
		return
	}

	cfg := dirclient.Config{
		ServerAddress: entry.Address,
		AuthMode:      "oidc",
		TLSSkipVerify: app.cfg.Directory.TLSSkipVerify,
		TLSCAFile:     app.cfg.Directory.TLSCAFile,
		TLSCertFile:   app.cfg.Directory.TLSCertFile,
		TLSKeyFile:    app.cfg.Directory.TLSKeyFile,
		AuthToken:     token,
		OIDCIssuer:    entry.OIDCIssuer,
		OIDCClientID:  entry.OIDCClientID,
	}
	app.connect(cfg)
}

// oidcDeviceWriter intercepts device flow output, extracts the URL and code,
// copies the code to clipboard, opens the browser, and shows an auth popup.
type oidcDeviceWriter struct {
	gui  *gocui.Gui
	app  *Gui
	buf  string
	done bool
}

func (w *oidcDeviceWriter) Write(p []byte) (n int, err error) {
	w.buf += string(p)

	if w.done {
		return len(p), nil
	}

	url, code := parseDeviceFlowOutput(w.buf)
	if url == "" || code == "" {
		return len(p), nil
	}
	w.done = true

	_ = writeClipboard(code)
	openBrowser(url)

	w.gui.Update(func(g *gocui.Gui) error {
		w.app.showAuthPopup(g, url, code)
		return nil
	})

	return len(p), nil
}

func parseDeviceFlowOutput(s string) (url, code string) {
	const prefix = "Enter this code at "
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return "", ""
	}
	rest := s[idx+len(prefix):]

	colonNewline := strings.Index(rest, ":\n")
	if colonNewline < 0 {
		return "", ""
	}
	url = strings.TrimSpace(rest[:colonNewline])

	rest = rest[colonNewline+2:]
	waitIdx := strings.Index(rest, "Waiting for authorization")
	if waitIdx < 0 {
		return url, ""
	}
	code = strings.TrimSpace(rest[:waitIdx])
	return url, code
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (app *Gui) showAuthPopup(g *gocui.Gui, url, code string) {
	content := fmt.Sprintf(
		" Authenticate in your browser:\n\n   %s\n\n Code: %s  (copied to clipboard)\n\n Waiting for authorization...",
		url, code,
	)
	app.state.authPopupText = content

	v, err := g.View(viewAuthPopup)
	if err != nil {
		return
	}
	v.Clear()
	fmt.Fprint(v, content)
	app.showPopup(g, viewAuthPopup)
}

func (app *Gui) closeAuthPopup(g *gocui.Gui) {
	app.state.authPopupText = ""
	app.hidePopup(g, viewAuthPopup)
}

func (app *Gui) dismissAuthPopup(g *gocui.Gui, v *gocui.View) error {
	app.closeAuthPopup(g)
	return nil
}

func (app *Gui) connectToOASF(g *gocui.Gui, addr string) {
	app.stopOASFReconnectLoop()
	client, err := oasf.NewClient(oasf.Config{ServerAddress: addr})
	if err != nil {
		app.state.oasfStatus = connFailed
		app.state.oasfError = err.Error()
		app.state.oasfAddr = addr
		app.renderDirectory(g)
		return
	}
	app.state.oasfClient = client
	app.state.oasfAddr = addr
	app.state.oasfStatus = connTrying
	app.state.oasfError = ""
	app.state.classEntries = nil
	app.state.classEntriesVer = ""
	app.renderDirectory(g)
	go app.pingOASF(client)
}

func (app *Gui) openConnectDialog(g *gocui.Gui, v *gocui.View) error {
	app.openInput("Connect to directory (enter addr)", app.state.serverAddr,
		func(addr string) {
			if addr == "" {
				return
			}
			app.connectToDirectory(g, config.DirectoryEntry{Address: addr})
		},
		nil, nil,
	)
	return nil
}

func (app *Gui) openOASFDialog(g *gocui.Gui, v *gocui.View) error {
	app.openInput("Connect to OASF server (enter URL)", app.state.oasfAddr,
		func(addr string) {
			if addr == "" {
				return
			}
			app.connectToOASF(g, addr)
		},
		nil, nil,
	)
	return nil
}
