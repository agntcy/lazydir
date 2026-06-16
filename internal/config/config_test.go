// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveColor_Named(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		fallback string
		want     string
	}{
		{"red", "red", "fb", "\033[31m"},
		{"green", "green", "fb", "\033[32m"},
		{"brightCyan", "brightCyan", "fb", "\033[96m"},
		{"with spaces", "  yellow  ", "fb", "\033[33m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveColor(tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("ResolveColor(%q, %q) = %q, want %q", tt.input, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestResolveColor_256Index(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"0", "\033[38;5;0m"},
		{"42", "\033[38;5;42m"},
		{"255", "\033[38;5;255m"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := ResolveColor(tt.input, "fb")
			if got != tt.want {
				t.Errorf("ResolveColor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveColor_Hex(t *testing.T) {
	t.Parallel()

	got := ResolveColor("#ff8800", "fb")
	want := "\033[38;2;255;136;0m"
	if got != want {
		t.Errorf("ResolveColor(#ff8800) = %q, want %q", got, want)
	}
}

func TestResolveColor_Fallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"unknown name", "neon"},
		{"out of range", "256"},
		{"negative", "-1"},
		{"bad hex", "#gggggg"},
		{"short hex", "#fff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ResolveColor(tt.input, "fallback")
			if got != "fallback" {
				t.Errorf("ResolveColor(%q) = %q, want %q", tt.input, got, "fallback")
			}
		})
	}
}

func TestResolveDirectoryServers(t *testing.T) {
	t.Parallel()

	t.Run("list takes precedence", func(t *testing.T) {
		t.Parallel()
		s := ServerConfig{
			DirectoryServers: []DirectoryEntry{{Address: "a:1"}, {Address: "b:2"}},
			DirectoryAddress: "c:3",
		}
		got, err := s.ResolveDirectoryServers()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].Address != "a:1" {
			t.Errorf("expected list servers, got %+v", got)
		}
	})

	t.Run("deprecated fallback", func(t *testing.T) {
		t.Parallel()
		s := ServerConfig{DirectoryAddress: "legacy:8888"}
		got, err := s.ResolveDirectoryServers()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Address != "legacy:8888" {
			t.Errorf("expected deprecated address, got %+v", got)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		s := ServerConfig{}
		got, err := s.ResolveDirectoryServers()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})
}

func TestResolveOASFServers(t *testing.T) {
	t.Parallel()

	t.Run("list takes precedence", func(t *testing.T) {
		t.Parallel()
		s := ServerConfig{
			OASFServers: []string{"https://a.com", "https://b.com"},
			OASFAddress: "https://c.com",
		}
		got := s.ResolveOASFServers()
		if len(got) != 2 || got[0] != "https://a.com" {
			t.Errorf("expected list, got %v", got)
		}
	})

	t.Run("deprecated fallback", func(t *testing.T) {
		t.Parallel()
		s := ServerConfig{OASFAddress: "https://legacy.com"}
		got := s.ResolveOASFServers()
		if len(got) != 1 || got[0] != "https://legacy.com" {
			t.Errorf("expected deprecated, got %v", got)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		s := ServerConfig{}
		if got := s.ResolveOASFServers(); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestDirectoryEntry_Label(t *testing.T) {
	t.Parallel()
	e := DirectoryEntry{Address: "example.com:443"}
	if got := e.Label(); got != "example.com:443" {
		t.Errorf("Label() = %q, want %q", got, "example.com:443")
	}
}

func TestDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	got := Dir()
	want := "/tmp/test-xdg/lazydir"
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestLoad_MissingDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/nonexistent/path/that/does/not/exist")
	cfg := Load()
	if cfg.GUI.ScrollStep != 0 || len(cfg.Server.DirectoryServers) != 0 {
		t.Errorf("expected zero-value config, got %+v", cfg)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgDir := filepath.Join(dir, "lazydir")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	data := []byte(`
gui:
  scrollStep: 5
  splitRatio: 0.4
server:
  oasfServers:
    - "https://test.example.com"
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Load()
	if cfg.GUI.ScrollStep != 5 {
		t.Errorf("ScrollStep = %d, want 5", cfg.GUI.ScrollStep)
	}
	if cfg.GUI.SplitRatio != 0.4 {
		t.Errorf("SplitRatio = %f, want 0.4", cfg.GUI.SplitRatio)
	}
	if len(cfg.Server.OASFServers) != 1 || cfg.Server.OASFServers[0] != "https://test.example.com" {
		t.Errorf("OASFServers = %v, want [https://test.example.com]", cfg.Server.OASFServers)
	}
}
