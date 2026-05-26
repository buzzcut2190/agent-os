package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/agent-os/agent-os/pkg/provider"
)

func runProvider(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: agentfs provider <list|set|key|test|switch|init> [args]")
		os.Exit(1)
	}

	// Initialize provider infrastructure.
	reg := provider.NewRegistry()
	ks := provider.NewKeyStore(providerKeyStorePath(), nil)

	// Try to load existing config.
	cfgPath := providerConfigPath()
	cfg, err := reg.LoadConfig(cfgPath)
	if err != nil && args[0] != "init" {
		fmt.Fprintf(os.Stderr, "load config: %v (run 'agentfs provider init' to configure)\n", err)
		os.Exit(1)
	}

	router := provider.NewRouter(reg.All())

	// Sync API keys from keystore into providers.
	for _, p := range reg.All() {
		if key, ok := ks.Get(p.Name()); ok {
			p.SetAPIKey(key)
		}
	}

	switch args[0] {
	case "list":
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTYPE\tMODELS\tHEALTHY")
		for _, p := range reg.All() {
			modelIDs := make([]string, len(p.Models()))
			for i, m := range p.Models() {
				modelIDs[i] = m.ID
			}
			healthy := "ok"
			if err := p.Ping(context.Background()); err != nil {
				healthy = "error"
			}
			// Need to get type from config.
			pType := "unknown"
			if cfg != nil {
				for _, pc := range cfg.Providers {
					if pc.Name == p.Name() {
						pType = pc.Type
					}
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name(), pType, strings.Join(modelIDs, ","), healthy)
		}
		w.Flush()

	case "set":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs provider set <provider|model>")
			os.Exit(1)
		}
		if err := router.SetDefault(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "set: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Default set to: %s\n", args[1])

	case "key":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs provider key <provider>")
			os.Exit(1)
		}
		fmt.Printf("Enter API key for %s (input hidden): ", args[1])
		// Simple read without echo is platform-dependent; use bufio.
		reader := bufio.NewReader(os.Stdin)
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key == "" {
			fmt.Println("cancelled")
			os.Exit(0)
		}
		if err := ks.Set(args[1], key); err != nil {
			fmt.Fprintf(os.Stderr, "set key: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Key set for %s: %s\n", args[1], provider.Mask(key))

	case "test":
		if len(args) > 1 {
			p, ok := reg.Get(args[1])
			if !ok {
				fmt.Fprintf(os.Stderr, "provider %q not found\n", args[1])
				os.Exit(1)
			}
			if err := p.Ping(context.Background()); err != nil {
				fmt.Printf("%s: UNHEALTHY (%v)\n", args[1], err)
			} else {
				fmt.Printf("%s: HEALTHY\n", args[1])
			}
			return
		}
		fmt.Println("Testing all providers...")
		for _, p := range reg.All() {
			if err := p.Ping(context.Background()); err != nil {
				fmt.Printf("  %-20s UNHEALTHY (%v)\n", p.Name(), err)
			} else {
				fmt.Printf("  %-20s HEALTHY\n", p.Name())
			}
		}

	case "switch":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs provider switch <provider>")
			os.Exit(1)
		}
		if _, ok := reg.Get(args[1]); !ok {
			fmt.Fprintf(os.Stderr, "provider %q not found\n", args[1])
			os.Exit(1)
		}
		router.SetDefault(args[1])
		fmt.Printf("Switched to provider: %s\n", args[1])

	case "init":
		runProviderInit()

	default:
		fmt.Fprintf(os.Stderr, "Unknown provider command: %s\n", args[0])
		os.Exit(1)
	}
}

func runProviderInit() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("agentfs Provider Setup")
	fmt.Println(strings.Repeat("-", 40))

	fmt.Print("Provider name (e.g. deepseek): ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Println("cancelled")
		return
	}

	fmt.Print("Provider type (openai-compatible/anthropic): ")
	pType, _ := reader.ReadString('\n')
	pType = strings.TrimSpace(pType)
	if pType == "" {
		pType = "openai-compatible"
	}

	fmt.Print("Base URL: ")
	baseURL, _ := reader.ReadString('\n')
	baseURL = strings.TrimSpace(baseURL)

	fmt.Printf("Models (comma-separated): ")
	modelsStr, _ := reader.ReadString('\n')
	modelsStr = strings.TrimSpace(modelsStr)
	var models []string
	if modelsStr != "" {
		models = strings.Split(modelsStr, ",")
		for i := range models {
			models[i] = strings.TrimSpace(models[i])
		}
	}

	fmt.Print("API Key (input hidden): ")
	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	cfgPath := providerConfigPath()
	cfg := &provider.ConfigFile{
		Providers: []provider.ProviderConfig{
			{Name: name, Type: pType, BaseURL: baseURL, Models: models},
		},
		Agents: provider.AgentsConfig{Default: name},
		Router: provider.RouterConfig{Strategy: "priority", Fallback: true},
	}

	reg := provider.NewRegistry()
	if err := reg.SaveConfig(cfgPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return
	}

	if key != "" {
		ks := provider.NewKeyStore(providerKeyStorePath(), nil)
		ks.Set(name, key)
	}

	fmt.Println("\nConfiguration saved!")
	fmt.Printf("  Config: %s\n", cfgPath)
	fmt.Printf("  Default: %s\n", name)
}

func providerConfigPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "providers.yaml"
	}
	dir := filepath.Join(home, ".config", "agentfs")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "providers.yaml")
}

func providerKeyStorePath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "keys.json"
	}
	dir := filepath.Join(home, ".config", "agentfs")
	return filepath.Join(dir, "keys.json")
}
