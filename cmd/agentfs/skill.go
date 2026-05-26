package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/agent-os/agent-os/pkg/skill"
)

func runSkill(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: agentfs skill <list|activate|deactivate|install|show> [args]")
		os.Exit(1)
	}

	eng := skill.NewEngine("", "")
	if err := eng.LoadAll(); err != nil {
		fmt.Fprintf(os.Stderr, "load skills: %v\n", err)
		os.Exit(1)
	}
	eng.LoadState()

	switch args[0] {
	case "list":
		skills := eng.List()
		if len(skills) == 0 {
			fmt.Println("No skills available.")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTATE\tSOURCE\tDESCRIPTION")
		for _, s := range skills {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.State.String(), s.Source, s.Description)
		}
		w.Flush()

	case "activate":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs skill activate <name>")
			os.Exit(1)
		}
		if err := eng.Activate(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "activate: %v\n", err)
			os.Exit(1)
		}
		eng.SaveState()
		fmt.Printf("Skill %q activated\n", args[1])

	case "deactivate":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs skill deactivate <name>")
			os.Exit(1)
		}
		if err := eng.Deactivate(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "deactivate: %v\n", err)
			os.Exit(1)
		}
		eng.SaveState()
		fmt.Printf("Skill %q deactivated\n", args[1])

	case "show":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs skill show <name>")
			os.Exit(1)
		}
		def, err := eng.Get(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "show: %v\n", err)
			os.Exit(1)
		}
		info := skill.SkillInfo{
			Name:        def.Manifest.Name,
			Description: def.Manifest.Description,
			Version:     def.Manifest.Version,
			Author:      def.Manifest.Author,
			Tags:        def.Manifest.Tags,
			State:       def.State,
			Source:      def.Source,
		}
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(data))

		// Print context.
		fmt.Println("\n--- CONTEXT ---")
		ctx, err := eng.GetContext(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "context: %v\n", err)
		} else {
			fmt.Println(ctx)
		}

	case "install":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs skill install <path> [name]")
			os.Exit(1)
		}
		name := args[1]
		if len(args) > 2 {
			name = args[2]
		}
		if err := eng.Install(args[1], name); err != nil {
			fmt.Fprintf(os.Stderr, "install: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Skill %q installed\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown skill command: %s\n", args[0])
		fmt.Fprintln(os.Stderr, "Usage: agentfs skill <list|activate|deactivate|install|show> [args]")
		os.Exit(1)
	}
}
