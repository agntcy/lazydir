package main

import (
	"fmt"
	"os"

	"github.com/akijakya/lazydir/internal/config"
	"github.com/akijakya/lazydir/internal/dirclient"
	"github.com/akijakya/lazydir/internal/gui"
	"github.com/akijakya/lazydir/internal/oasf"
)

func main() {
	userCfg := config.Load()
	cfg := parseFlags(userCfg)

	if err := gui.New(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func parseFlags(userCfg config.Config) gui.Config {
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

	cfg := gui.Config{
		Directory: dirclient.Config{
			ServerAddress: dirServers[0].Address,
			OIDCIssuer:    dirServers[0].OIDCIssuer,
			OIDCClientID:  dirServers[0].OIDCClientID,
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

	// Honour the environment variables also used by dirctl and the OASF SDK.
	if addr := os.Getenv("DIRECTORY_CLIENT_SERVER_ADDRESS"); addr != "" {
		cfg.Directory.ServerAddress = addr
	}
	if addr := os.Getenv("OASF_SERVER_ADDRESS"); addr != "" {
		cfg.OASF.ServerAddress = addr
	}

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server-addr", "-s":
			if i+1 < len(args) {
				i++
				cfg.Directory.ServerAddress = args[i]
			}
		case "--oasf-addr", "-o":
			if i+1 < len(args) {
				i++
				cfg.OASF.ServerAddress = args[i]
			}
		case "--auth-mode", "-a":
			if i+1 < len(args) {
				i++
				cfg.Directory.AuthMode = args[i]
			}
		case "--auth-token":
			if i+1 < len(args) {
				i++
				cfg.Directory.AuthToken = args[i]
			}
		case "--tls-ca-file":
			if i+1 < len(args) {
				i++
				cfg.Directory.TLSCAFile = args[i]
			}
		case "--tls-cert-file":
			if i+1 < len(args) {
				i++
				cfg.Directory.TLSCertFile = args[i]
			}
		case "--tls-key-file":
			if i+1 < len(args) {
				i++
				cfg.Directory.TLSKeyFile = args[i]
			}
		case "--tls-skip-verify":
			cfg.Directory.TLSSkipVerify = true
		case "--help", "-h":
			printUsage()
			os.Exit(0)
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			printUsage()
			os.Exit(1)
		}
	}

	return cfg
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `lazydir - TUI for AGNTCY Directory

Usage:
  lazydir [flags]

Flags:
  --server-addr, -s   <addr>    Directory server address (default: localhost:8888)
  --oasf-addr,    -o   <url>    OASF schema server URL (default: %s)
  --auth-mode,    -a   <mode>   Auth mode: insecure|tls|oidc|jwt|x509 (default: auto-detect)
  --auth-token        <token>   Pre-issued Bearer token
  --tls-ca-file       <path>    TLS CA certificate file
  --tls-cert-file     <path>    TLS client certificate file
  --tls-key-file      <path>    TLS client key file
  --tls-skip-verify             Skip TLS certificate verification
  --help, -h                    Show this help

Environment:
  DIRECTORY_CLIENT_SERVER_ADDRESS  Default Directory server address
  OASF_SERVER_ADDRESS              Default OASF schema server URL

Key Bindings (inside the TUI):
  tab / shift+tab    Cycle panel focus
  1 / 2 / 3 / 0      Jump to panel
  ↑ ↓ / j k          Navigate list items
  enter              Open filter category / toggle option / preview record
  tab (in Filters)   Toggle option in the options view
  esc (in Filters)   Return from options view to filter list
  /                  Filter records by name
  esc (in Records)   Clear name filter
  c                  Open connect dialog (Connections panel → Directory)
  o                  Open connect dialog (Connections panel → OASF server)
  r                  Refresh records
  ?                  Show keybinding help
  q / ctrl+c         Quit
`, oasf.DefaultServerAddress)
}
