package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/agent-os/agent-os/pkg/mcp"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("agentfs-mcp %s\n", version)

	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		transport := serveCmd.String("transport", "stdio", "Transport protocol: stdio or sse")
		port := serveCmd.Int("port", 8080, "Port for SSE transport")
		projectRoot := serveCmd.String("project-root", ".", "Root directory to serve")
		sessionDir := serveCmd.String("session-dir", "", "Session storage directory")
		_ = serveCmd.Parse(os.Args[2:])

		cfg := mcp.DefaultConfig()
		cfg.Transport = *transport
		cfg.Port = *port
		cfg.ProjectRoot = *projectRoot
		if *sessionDir != "" {
			cfg.SessionDir = *sessionDir
		}

		srv := mcp.New(cfg)
		srv.RegisterAll()
		if err := srv.Start(); err != nil {
			log.Fatalf("server: %v", err)
		}

	case "install":
		installCmd := flag.NewFlagSet("install", flag.ExitOnError)
		installAgent := installCmd.String("agent", "claude", "Target agent: claude or cursor")
		installGlobal := installCmd.Bool("global", false, "Install globally")
		installBinary := installCmd.String("binary", "", "Path to agentfs-mcp binary (auto-detected)")
		_ = installCmd.Parse(os.Args[2:])
		if err := mcp.InstallAgent(*installAgent, *installGlobal, *installBinary); err != nil {
			log.Fatalf("install: %v", err)
		}

	case "uninstall":
		uninstallCmd := flag.NewFlagSet("uninstall", flag.ExitOnError)
		uninstallAgent := uninstallCmd.String("agent", "claude", "Target agent: claude")
		uninstallGlobal := uninstallCmd.Bool("global", false, "Uninstall from global config")
		_ = uninstallCmd.Parse(os.Args[2:])
		if err := mcp.UninstallAgent(*uninstallAgent, *uninstallGlobal); err != nil {
			log.Fatalf("uninstall: %v", err)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: agentfs-mcp <command> [flags]

Commands:
  serve       Start the MCP server
  install     Generate MCP client configuration for an AI agent
  uninstall   Remove previously installed MCP client configuration
  version     Print version and exit

Serve flags:
  --transport string   Transport: "stdio" or "sse" (default "stdio")
  --port int           Port for SSE transport (default 8080)
  --project-root string Root directory to serve (default ".")
  --session-dir string Session storage directory
`)
}
