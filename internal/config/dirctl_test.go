// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirctlContexts_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
current_context: local
contexts:
  prod:
    server_address: prod.example.com:443
    auth_mode: oidc
    oidc_issuer: https://auth.example.com
    oidc_client_id: dirctl
    tls_skip_verify: false
  local:
    server_address: localhost:8888
    auth_mode: insecure
  staging:
    server_address: staging.example.com:443
    auth_mode: tls
    tls_ca_file: /path/to/ca.pem
    tls_cert_file: /path/to/cert.pem
    tls_key_file: /path/to/key.pem
    tls_skip_verify: true
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadDirctlContexts(path)
	if err != nil {
		t.Fatalf("LoadDirctlContexts() error = %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Order preserved from YAML: prod, local, staging
	if entries[0].ContextName != "prod" {
		t.Errorf("entries[0].ContextName = %q, want %q", entries[0].ContextName, "prod")
	}
	if entries[0].Address != "prod.example.com:443" {
		t.Errorf("entries[0].Address = %q, want %q", entries[0].Address, "prod.example.com:443")
	}
	if entries[0].OIDCIssuer != "https://auth.example.com" {
		t.Errorf("entries[0].OIDCIssuer = %q, want %q", entries[0].OIDCIssuer, "https://auth.example.com")
	}
	if entries[0].OIDCClientID != "dirctl" {
		t.Errorf("entries[0].OIDCClientID = %q, want %q", entries[0].OIDCClientID, "dirctl")
	}

	if entries[1].ContextName != "local" {
		t.Errorf("entries[1].ContextName = %q, want %q", entries[1].ContextName, "local")
	}
	if entries[1].Address != "localhost:8888" {
		t.Errorf("entries[1].Address = %q, want %q", entries[1].Address, "localhost:8888")
	}
	if entries[1].AuthMode != "insecure" {
		t.Errorf("entries[1].AuthMode = %q, want %q", entries[1].AuthMode, "insecure")
	}

	if entries[2].ContextName != "staging" {
		t.Errorf("entries[2].ContextName = %q, want %q", entries[2].ContextName, "staging")
	}
	if entries[2].TLSCAFile != "/path/to/ca.pem" {
		t.Errorf("entries[2].TLSCAFile = %q, want %q", entries[2].TLSCAFile, "/path/to/ca.pem")
	}
	if entries[2].TLSCertFile != "/path/to/cert.pem" {
		t.Errorf("entries[2].TLSCertFile = %q, want %q", entries[2].TLSCertFile, "/path/to/cert.pem")
	}
	if entries[2].TLSKeyFile != "/path/to/key.pem" {
		t.Errorf("entries[2].TLSKeyFile = %q, want %q", entries[2].TLSKeyFile, "/path/to/key.pem")
	}
	if !entries[2].TLSSkipVerify {
		t.Error("entries[2].TLSSkipVerify = false, want true")
	}
}

func TestLoadDirctlContexts_SkipsEmptyAddress(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
contexts:
  empty:
    server_address: ""
    auth_mode: insecure
  valid:
    server_address: localhost:9999
    auth_mode: insecure
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadDirctlContexts(path)
	if err == nil {
		t.Error("expected warning error for empty server_address, got nil")
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ContextName != "valid" {
		t.Errorf("entries[0].ContextName = %q, want %q", entries[0].ContextName, "valid")
	}
}

func TestLoadDirctlContexts_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := LoadDirctlContexts("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadDirctlContexts_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("\t- :\n\t\t::"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadDirctlContexts(path)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestLoadDirctlContexts_EmptyContexts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
current_context: local
contexts: {}
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadDirctlContexts(path)
	if err != nil {
		t.Fatalf("LoadDirctlContexts() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %+v", entries)
	}
}

func TestLoadDirctlContexts_TildeExpansion(t *testing.T) {
	fakeHome := t.TempDir()
	dirctlDir := filepath.Join(fakeHome, ".config", "dirctl")
	if err := os.MkdirAll(dirctlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`
contexts:
  tilde-test:
    server_address: tilde.example.com:443
    auth_mode: insecure
`)
	if err := os.WriteFile(filepath.Join(dirctlDir, "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", fakeHome)

	entries, err := LoadDirctlContexts("~/.config/dirctl/config.yaml")
	if err != nil {
		t.Fatalf("LoadDirctlContexts() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Address != "tilde.example.com:443" {
		t.Errorf("entries[0].Address = %q, want %q", entries[0].Address, "tilde.example.com:443")
	}
}

func TestResolveDirectoryServers_DirctlMerge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
contexts:
  imported:
    server_address: imported.example.com:443
    auth_mode: tls
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := ServerConfig{
		DirctlConfigPath: path,
		DirectoryServers: []DirectoryEntry{{Address: "manual.example.com:443"}},
	}

	got, err := s.ResolveDirectoryServers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].ContextName != "imported" || got[0].Address != "imported.example.com:443" {
		t.Errorf("entries[0] = %+v, want imported context first", got[0])
	}
	if got[1].Address != "manual.example.com:443" {
		t.Errorf("entries[1].Address = %q, want %q", got[1].Address, "manual.example.com:443")
	}
}

func TestResolveDirectoryServers_DirctlMissing(t *testing.T) {
	t.Parallel()

	s := ServerConfig{
		DirctlConfigPath: "/nonexistent/dirctl/config.yaml",
		DirectoryServers: []DirectoryEntry{{Address: "fallback.example.com:443"}},
	}

	got, err := s.ResolveDirectoryServers()
	if err == nil {
		t.Error("expected error for missing dirctl config, got nil")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 entry despite error, got %d", len(got))
	}
	if got[0].Address != "fallback.example.com:443" {
		t.Errorf("expected fallback entry, got %+v", got[0])
	}
}

func TestDirectoryEntry_Label_WithContext(t *testing.T) {
	t.Parallel()

	e := DirectoryEntry{Address: "example.com:443", ContextName: "prod"}
	want := "prod (example.com:443)"
	if got := e.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestDirectoryEntry_Label_WithoutContext(t *testing.T) {
	t.Parallel()

	e := DirectoryEntry{Address: "example.com:443"}
	want := "example.com:443"
	if got := e.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}
