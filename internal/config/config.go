// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Config mirrors the YAML structure of ~/.config/lazydir/config.yml.
type Config struct {
	GUI    GUIConfig    `yaml:"gui"`
	Server ServerConfig `yaml:"server"`
	Stream StreamConfig `yaml:"stream"`
}

// GUIConfig groups all visual/TUI settings.
type GUIConfig struct {
	Theme              ThemeConfig `yaml:"theme"`
	ScrollStep         int         `yaml:"scrollStep"`
	SplitRatio         float64     `yaml:"splitRatio"`
	InputDebounceDelay int         `yaml:"inputDebounceDelay"`
	DimLevel           *float64    `yaml:"dimLevel"`
}

// ThemeConfig lets users override the base16 color palette used throughout the
// TUI. Each field accepts a color name ("red", "brightCyan", …), a 256-color
// index ("42"), or a hex true-color value ("#ff8800").
type ThemeConfig struct {
	Color1             string `yaml:"color1"`
	Color2             string `yaml:"color2"`
	Color3             string `yaml:"color3"`
	Color4             string `yaml:"color4"`
	Color5             string `yaml:"color5"`
	Color6             string `yaml:"color6"`
	Color7             string `yaml:"color7"`
	Color8             string `yaml:"color8"`
	Color9             string `yaml:"color9"`
	Color10            string `yaml:"color10"`
	ActiveBorderColor  string `yaml:"activeBorderColor"`
	SelectedRowBgColor string `yaml:"selectedRowBgColor"`
}

// DirectoryEntry describes a predefined directory server.
type DirectoryEntry struct {
	Address       string `yaml:"address"`
	AuthMode      string `yaml:"authMode"`
	AuthToken     string `yaml:"authToken"`
	TLSCAFile     string `yaml:"tlsCAFile"`
	TLSCertFile   string `yaml:"tlsCertFile"`
	TLSKeyFile    string `yaml:"tlsKeyFile"`
	TLSSkipVerify bool   `yaml:"tlsSkipVerify"`
	OIDCIssuer    string `yaml:"oidcIssuer"`
	OIDCClientID  string `yaml:"oidcClientID"`

	// ContextName is set for entries imported from a dirctl config file.
	ContextName string `yaml:"-"`
}

// Label returns a human-readable label for display in the server popup.
// For entries imported from dirctl, the context name is shown as a prefix.
func (e DirectoryEntry) Label() string {
	if e.ContextName != "" {
		return e.ContextName + " (" + e.Address + ")"
	}
	return e.Address
}

// ServerConfig holds server addresses. Each list may contain multiple
// predefined servers the user can switch between in the TUI; the first
// entry in each list is used as the initial/default connection target.
type ServerConfig struct {
	DirctlConfigPath string           `yaml:"dirctlConfigPath"`
	DirectoryServers []DirectoryEntry `yaml:"directoryServers"`
	OASFServers      []string         `yaml:"oasfServers"`
	OASFTimeout      int              `yaml:"oasfTimeout"`

	// Deprecated single-address fields kept for backward compatibility.
	DirectoryAddress string `yaml:"directoryAddress"`
	OASFAddress      string `yaml:"oasfAddress"`
}

// ResolveDirectoryServers returns the effective directory server list.
// When DirctlConfigPath is set, imported contexts are prepended before
// the manually configured entries. The deprecated single-address field
// is promoted if no other entries exist.
func (s ServerConfig) ResolveDirectoryServers() []DirectoryEntry {
	var merged []DirectoryEntry

	if s.DirctlConfigPath != "" {
		imported, err := LoadDirctlContexts(s.DirctlConfigPath)
		if err == nil {
			merged = append(merged, imported...)
		}
	}

	merged = append(merged, s.DirectoryServers...)

	if len(merged) > 0 {
		return merged
	}
	if s.DirectoryAddress != "" {
		return []DirectoryEntry{{Address: s.DirectoryAddress}}
	}
	return nil
}

// ResolveOASFServers returns the effective OASF server list, promoting
// the deprecated single-address field if the list is empty.
func (s ServerConfig) ResolveOASFServers() []string {
	if len(s.OASFServers) > 0 {
		return s.OASFServers
	}
	if s.OASFAddress != "" {
		return []string{s.OASFAddress}
	}
	return nil
}

// StreamConfig controls record streaming batch sizes.
type StreamConfig struct {
	FirstPageSize int `yaml:"firstPageSize"`
	BatchSize     int `yaml:"batchSize"`
}

// Dir returns the lazydir configuration directory, respecting XDG_CONFIG_HOME.
func Dir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "lazydir")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "lazydir")
}

// Load reads the config file from the standard location. If the file does not
// exist or cannot be parsed, a zero-value Config is returned (all defaults).
func Load() Config {
	var cfg Config
	dir := Dir()
	if dir == "" {
		return cfg
	}
	for _, name := range []string{"config.yml", "config.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		_ = yaml.Unmarshal(data, &cfg)
		return cfg
	}
	return cfg
}

// colorNames maps base16 color names to ANSI SGR foreground escape codes.
var colorNames = map[string]string{
	"black":         "\033[30m",
	"red":           "\033[31m",
	"green":         "\033[32m",
	"yellow":        "\033[33m",
	"blue":          "\033[34m",
	"magenta":       "\033[35m",
	"cyan":          "\033[36m",
	"white":         "\033[37m",
	"brightBlack":   "\033[90m",
	"brightRed":     "\033[91m",
	"brightGreen":   "\033[92m",
	"brightYellow":  "\033[93m",
	"brightBlue":    "\033[94m",
	"brightMagenta": "\033[95m",
	"brightCyan":    "\033[96m",
	"brightWhite":   "\033[97m",
}

// ResolveColor converts a user-supplied color value to an ANSI escape code.
// Accepted formats:
//   - Color name: "red", "brightCyan", …
//   - 256-color index: "42"
//   - Hex true-color: "#ff8800"
//
// If the value is empty or unrecognised, fallback is returned unchanged.
func ResolveColor(value, fallback string) string {
	if value == "" {
		return fallback
	}
	value = strings.TrimSpace(value)
	if code, ok := colorNames[value]; ok {
		return code
	}
	if n, err := strconv.Atoi(value); err == nil && n >= 0 && n <= 255 {
		return fmt.Sprintf("\033[38;5;%dm", n)
	}
	if strings.HasPrefix(value, "#") && len(value) == 7 {
		r, err1 := strconv.ParseUint(value[1:3], 16, 8)
		g, err2 := strconv.ParseUint(value[3:5], 16, 8)
		b, err3 := strconv.ParseUint(value[5:7], 16, 8)
		if err1 == nil && err2 == nil && err3 == nil {
			return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
		}
	}
	return fallback
}
