package gui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/akijakya/lazydir/internal/config"
	"github.com/jesseduffield/gocui"
)

// Theme holds all color values used throughout the TUI.
// Each slot defaults to a base16 terminal color and can be overridden in
// the config file (~/.config/lazydir/config.yml) under gui.theme.
type Theme struct {
	Color1  string // default: yellow       — skills, annotations
	Color2  string // default: cyan         — domains, class tree, accents
	Color3  string // default: magenta      — modules, timestamps
	Color4  string // default: green        — connected, OASF version, loading
	Color5  string // default: blue         — version, options bar, section headers
	Color6  string // default: red          — disconnected indicator
	Color7  string // default: bright red   — author filter
	Color8  string // default: bright yellow— trusted filter
	Color9  string // default: bright green — verified filter
	Color10 string // default: bright black — dim/muted (IDs, gray)
	Reset   string // ANSI reset sequence

	ActiveBorderColor gocui.Attribute // focused panel border + cursor row foreground
	SelectedRowBg     gocui.Attribute // highlighted row background in list panels
	DimCode           string          // ANSI sequence applied to preview text when dimmed
	DimFrameColor     gocui.Attribute // gocui color for preview frame/title when dimmed
}

var defaultTheme = Theme{
	Color1:  "\033[33m",
	Color2:  "\033[36m",
	Color3:  "\033[35m",
	Color4:  "\033[32m",
	Color5:  "\033[34m",
	Color6:  "\033[31m",
	Color7:  "\033[91m",
	Color8:  "\033[93m",
	Color9:  "\033[92m",
	Color10: "\033[90m",
	Reset:   "\033[0m",

	ActiveBorderColor: gocui.ColorGreen,
	SelectedRowBg:     gocui.Get256Color(8),
}

func newTheme(cfg config.ThemeConfig, dimLevel float64) Theme {
	t := defaultTheme
	t.Color1 = config.ResolveColor(cfg.Color1, t.Color1)
	t.Color2 = config.ResolveColor(cfg.Color2, t.Color2)
	t.Color3 = config.ResolveColor(cfg.Color3, t.Color3)
	t.Color4 = config.ResolveColor(cfg.Color4, t.Color4)
	t.Color5 = config.ResolveColor(cfg.Color5, t.Color5)
	t.Color6 = config.ResolveColor(cfg.Color6, t.Color6)
	t.Color7 = config.ResolveColor(cfg.Color7, t.Color7)
	t.Color8 = config.ResolveColor(cfg.Color8, t.Color8)
	t.Color9 = config.ResolveColor(cfg.Color9, t.Color9)
	t.Color10 = config.ResolveColor(cfg.Color10, t.Color10)
	if cfg.ActiveBorderColor != "" {
		t.ActiveBorderColor = resolveGocuiColor(cfg.ActiveBorderColor, t.ActiveBorderColor)
	}
	if cfg.SelectedRowBgColor != "" {
		t.SelectedRowBg = resolveGocuiColor(cfg.SelectedRowBgColor, t.SelectedRowBg)
	}

	if dimLevel < 0 {
		dimLevel = 0
	}
	if dimLevel > 1 {
		dimLevel = 1
	}
	if dimLevel > 0 {
		// 256-color grayscale ramp: 232 (#080808, near-black) … 255 (#eeeeee).
		colorIdx := 255 - int(dimLevel*23)
		if colorIdx < 232 {
			colorIdx = 232
		}
		t.DimCode = fmt.Sprintf("\033[38;5;%dm", colorIdx)
		t.DimFrameColor = gocui.Get256Color(int32(colorIdx))
	} else {
		t.DimCode = ""
		t.DimFrameColor = gocui.ColorDefault
	}
	return t
}

func resolveGocuiColor(name string, fallback gocui.Attribute) gocui.Attribute {
	name = strings.TrimSpace(strings.ToLower(name))
	switch name {
	case "black":
		return gocui.ColorBlack
	case "red":
		return gocui.ColorRed
	case "green":
		return gocui.ColorGreen
	case "yellow":
		return gocui.ColorYellow
	case "blue":
		return gocui.ColorBlue
	case "magenta":
		return gocui.ColorMagenta
	case "cyan":
		return gocui.ColorCyan
	case "white":
		return gocui.ColorWhite
	case "default":
		return gocui.ColorDefault
	}
	if n, err := strconv.Atoi(name); err == nil && n >= 0 && n <= 255 {
		return gocui.Get256Color(int32(n))
	}
	return fallback
}

// filterColor returns the ANSI color code for a given filter category.
func (t Theme) filterColor(c filterCategory) string {
	switch c {
	case filterSkills:
		return t.Color1
	case filterDomains:
		return t.Color2
	case filterModules:
		return t.Color3
	case filterOASFVersion:
		return t.Color4
	case filterVersion:
		return t.Color5
	case filterAuthor:
		return t.Color7
	case filterTrustedVerified:
		return t.Color8
	}
	return ""
}
