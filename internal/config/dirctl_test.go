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

	// current_context is "local", so it is moved to the front. The remaining
	// contexts keep their YAML order, giving: local, prod, staging.
	if entries[0].ContextName != "local" {
		t.Errorf("entries[0].ContextName = %q, want %q", entries[0].ContextName, "local")
	}
	if entries[0].Address != "localhost:8888" {
		t.Errorf("entries[0].Address = %q, want %q", entries[0].Address, "localhost:8888")
	}
	if entries[0].AuthMode != "insecure" {
		t.Errorf("entries[0].AuthMode = %q, want %q", entries[0].AuthMode, "insecure")
	}

	if entries[1].ContextName != "prod" {
		t.Errorf("entries[1].ContextName = %q, want %q", entries[1].ContextName, "prod")
	}
	if entries[1].Address != "prod.example.com:443" {
		t.Errorf("entries[1].Address = %q, want %q", entries[1].Address, "prod.example.com:443")
	}
	if entries[1].OIDCIssuer != "https://auth.example.com" {
		t.Errorf("entries[1].OIDCIssuer = %q, want %q", entries[1].OIDCIssuer, "https://auth.example.com")
	}
	if entries[1].OIDCClientID != "dirctl" {
		t.Errorf("entries[1].OIDCClientID = %q, want %q", entries[1].OIDCClientID, "dirctl")
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

func TestLoadDirctlContexts_CurrentContextMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
current_context: does-not-exist
contexts:
  first:
    server_address: first.example.com:443
    auth_mode: insecure
  second:
    server_address: second.example.com:443
    auth_mode: insecure
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadDirctlContexts(path)
	if err != nil {
		t.Fatalf("LoadDirctlContexts() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// current_context points to an unknown context: YAML order is preserved.
	if entries[0].ContextName != "first" {
		t.Errorf("entries[0].ContextName = %q, want %q", entries[0].ContextName, "first")
	}
	if entries[1].ContextName != "second" {
		t.Errorf("entries[1].ContextName = %q, want %q", entries[1].ContextName, "second")
	}
}

func TestLoadDirctlContexts_CurrentContextSkipped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// current_context points to a context that is present in YAML but skipped
	// because it has no server_address, so no reordering should happen.
	data := []byte(`
current_context: broken
contexts:
  broken:
    server_address: ""
    auth_mode: insecure
  first:
    server_address: first.example.com:443
    auth_mode: insecure
  second:
    server_address: second.example.com:443
    auth_mode: insecure
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadDirctlContexts(path)
	if err == nil {
		t.Error("expected warning error for skipped context, got nil")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// "broken" was skipped, so YAML order of the valid contexts is preserved.
	if entries[0].ContextName != "first" {
		t.Errorf("entries[0].ContextName = %q, want %q", entries[0].ContextName, "first")
	}
	if entries[1].ContextName != "second" {
		t.Errorf("entries[1].ContextName = %q, want %q", entries[1].ContextName, "second")
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
	t.Setenv("HOME", fakeHome)

	data := []byte(`
contexts:
  tilde-test:
    server_address: tilde.example.com:443
    auth_mode: insecure
`)
	if err := os.WriteFile(filepath.Join(fakeHome, "dirctl.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadDirctlContexts("~/dirctl.yaml")
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

func TestResolveDirectoryServers_DirctlPartialWarning(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
contexts:
  good:
    server_address: good.example.com:443
    auth_mode: tls
  bad:
    server_address: ""
    auth_mode: insecure
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
	if err == nil {
		t.Fatal("expected warning error for skipped context, got nil")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries (1 imported + 1 manual), got %d: %+v", len(got), got)
	}
	if got[0].ContextName != "good" || got[0].Address != "good.example.com:443" {
		t.Errorf("entries[0] = %+v, want imported 'good' context first", got[0])
	}
	if got[1].Address != "manual.example.com:443" {
		t.Errorf("entries[1].Address = %q, want %q", got[1].Address, "manual.example.com:443")
	}
}

func TestResolveDirectoryServers_ManualOverridesImported(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	data := []byte(`
contexts:
  shared:
    server_address: shared.example.com:443
    auth_mode: insecure
  unique:
    server_address: unique.example.com:443
    auth_mode: tls
`)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := ServerConfig{
		DirctlConfigPath: path,
		DirectoryServers: []DirectoryEntry{
			{Address: "shared.example.com:443", AuthMode: "oidc"},
			{Address: "extra.example.com:443"},
		},
	}

	got, err := s.ResolveDirectoryServers()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries (1 overridden + 1 imported-only + 1 manual-only), got %d: %+v", len(got), got)
	}
	if got[0].Address != "shared.example.com:443" || got[0].AuthMode != "oidc" {
		t.Errorf("entries[0] should be overridden manual entry, got %+v", got[0])
	}
	if got[0].ContextName != "" {
		t.Errorf("overridden entry should lose ContextName, got %q", got[0].ContextName)
	}
	if got[1].Address != "unique.example.com:443" {
		t.Errorf("entries[1].Address = %q, want %q", got[1].Address, "unique.example.com:443")
	}
	if got[2].Address != "extra.example.com:443" {
		t.Errorf("entries[2].Address = %q, want %q", got[2].Address, "extra.example.com:443")
	}
}

func TestLoadDirctlContexts_NonMappingRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("- item1\n- item2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadDirctlContexts(path)
	if err == nil {
		t.Error("expected error for non-mapping root YAML, got nil")
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
