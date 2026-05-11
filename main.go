// Copyright AGNTCY Contributors (https://github.com/agntcy)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/agntcy/lazydir/internal/config"
	"github.com/agntcy/lazydir/internal/dirclient"
	"github.com/agntcy/lazydir/internal/gui"
	"github.com/agntcy/lazydir/internal/oasf"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printUsage()
		os.Exit(0)
	}

	userCfg := config.Load()
	cfg := buildConfig(userCfg)

	if err := gui.New(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func buildConfig(userCfg config.Config) gui.Config {
	dirServers := userCfg.Server.ResolveDirectoryServers()
	if len(dirServers) == 0 {
		dirServers = []config.DirectoryEntry{{Address: "localhost:8888"}}
	}
	oasfServers := userCfg.Server.ResolveOASFServers()
	if len(oasfServers) == 0 {
		oasfServers = []string{oasf.DefaultServerAddress}
	}

	dimLevel := 0.6
	if userCfg.GUI.DimLevel != nil {
		dimLevel = *userCfg.GUI.DimLevel
	}

	first := dirServers[0]
	cfg := gui.Config{
		Directory: dirclient.Config{
			ServerAddress: first.Address,
			AuthMode:      first.AuthMode,
			AuthToken:     first.AuthToken,
			TLSCAFile:     first.TLSCAFile,
			TLSCertFile:   first.TLSCertFile,
			TLSKeyFile:    first.TLSKeyFile,
			TLSSkipVerify: first.TLSSkipVerify,
			OIDCIssuer:    first.OIDCIssuer,
			OIDCClientID:  first.OIDCClientID,
		},
		OASF: oasf.Config{
			ServerAddress: oasfServers[0],
			Timeout:       userCfg.Server.OASFTimeout,
		},
		DirectoryServers:   dirServers,
		OASFServers:        oasfServers,
		Theme:              userCfg.GUI.Theme,
		ScrollStep:         userCfg.GUI.ScrollStep,
		SplitRatio:         userCfg.GUI.SplitRatio,
		InputDebounceDelay: userCfg.GUI.InputDebounceDelay,
		DimLevel:           dimLevel,
		FirstPageSize:      userCfg.Stream.FirstPageSize,
		BatchSize:          userCfg.Stream.BatchSize,
	}

	// Honour environment variables also used by dirctl and the OASF SDK.
	if addr := os.Getenv("DIRECTORY_CLIENT_SERVER_ADDRESS"); addr != "" {
		cfg.Directory.ServerAddress = addr
	}
	if addr := os.Getenv("OASF_SERVER_ADDRESS"); addr != "" {
		cfg.OASF.ServerAddress = addr
	}

	return cfg
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `lazydir - TUI for AGNTCY Directory

Usage:
  lazydir [-h|--help]

All configuration is read from ~/.config/lazydir/config.yml (or config.yaml).
See config.example.yml for a complete annotated template.

Environment variables (override config file):
  DIRECTORY_CLIENT_SERVER_ADDRESS  Directory server address
  OASF_SERVER_ADDRESS              OASF schema server URL
  DEBUG                            Enable debug logging to lazydir_debug.log
`)
}
