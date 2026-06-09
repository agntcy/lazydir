// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// dirctlContext mirrors a single named context in the dirctl config.
type dirctlContext struct {
	ServerAddress string `yaml:"server_address"`
	AuthMode      string `yaml:"auth_mode"`
	AuthToken     string `yaml:"auth_token"`
	TLSCAFile     string `yaml:"tls_ca_file"`
	TLSCertFile   string `yaml:"tls_cert_file"`
	TLSKeyFile    string `yaml:"tls_key_file"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
	OIDCIssuer    string `yaml:"oidc_issuer"`
	OIDCClientID  string `yaml:"oidc_client_id"`
}

// LoadDirctlContexts reads and parses a dirctl config file, returning the
// contexts as DirectoryEntry values in the order they appear in the YAML file.
// The path supports a leading "~/" which is expanded to the user's home
// directory.
//
// Returns an error if the file cannot be read or parsed; callers should treat
// errors as non-fatal (log and continue with manually configured servers).
func LoadDirctlContexts(path string) ([]DirectoryEntry, error) {
	path = expandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading dirctl config: %w", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing dirctl config: %w", err)
	}

	contextsNode := findMapValue(&root, "contexts")
	if contextsNode == nil {
		return nil, nil
	}
	if contextsNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("parsing dirctl config: \"contexts\" is not a mapping")
	}

	var warnings []string
	entries := make([]DirectoryEntry, 0, len(contextsNode.Content)/2)
	for i := 0; i+1 < len(contextsNode.Content); i += 2 {
		keyNode := contextsNode.Content[i]
		valNode := contextsNode.Content[i+1]

		name := keyNode.Value

		var ctx dirctlContext
		if err := valNode.Decode(&ctx); err != nil {
			warnings = append(warnings, fmt.Sprintf("context %q: %v", name, err))
			continue
		}
		if ctx.ServerAddress == "" {
			warnings = append(warnings, fmt.Sprintf("context %q: missing server_address", name))
			continue
		}

		entries = append(entries, DirectoryEntry{
			Address:       ctx.ServerAddress,
			AuthMode:      ctx.AuthMode,
			AuthToken:     ctx.AuthToken,
			TLSCAFile:     ctx.TLSCAFile,
			TLSCertFile:   ctx.TLSCertFile,
			TLSKeyFile:    ctx.TLSKeyFile,
			TLSSkipVerify: ctx.TLSSkipVerify,
			OIDCIssuer:    ctx.OIDCIssuer,
			OIDCClientID:  ctx.OIDCClientID,
			ContextName:   name,
		})
	}

	var warnErr error
	if len(warnings) > 0 {
		warnErr = fmt.Errorf("dirctl config: %s", strings.Join(warnings, "; "))
	}
	return entries, warnErr
}

// findMapValue walks a yaml.Node tree to locate the value node for a given
// top-level mapping key.
func findMapValue(root *yaml.Node, key string) *yaml.Node {
	if root == nil {
		return nil
	}
	// Unwrap document node.
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i+1]
		}
	}
	return nil
}

// expandHome replaces a leading "~/" with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
