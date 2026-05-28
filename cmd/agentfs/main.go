package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/agent-os/agent-os/pkg/fs"
	"github.com/agent-os/agent-os/pkg/kernel"
	"github.com/agent-os/agent-os/pkg/sandbox"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("agentfs %s\n", version)

	case "mount":
		mountCmd := flag.NewFlagSet("mount", flag.ExitOnError)
		allowOther := mountCmd.Bool("allow_other", false, "allow other users to access the mount")
		if err := mountCmd.Parse(os.Args[2:]); err != nil {
			log.Fatalf("parse flags: %v", err)
		}

		args := mountCmd.Args()
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs mount [flags] <source_dir> <mount_point>")
			os.Exit(1)
		}
		if err := runMount(args[0], args[1], *allowOther); err != nil {
			log.Fatalf("mount failed: %v", err)
		}

	case "unmount":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs unmount <mount_point>")
			os.Exit(1)
		}
		if err := runUnmount(os.Args[2]); err != nil {
			log.Fatalf("unmount failed: %v", err)
		}

	case "session":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs session <start|list|get|open|mount|commit|discard|diff> [args]")
			os.Exit(1)
		}
		runSession(os.Args[2:])

	case "init":
		projectDir := "."
		if len(os.Args) > 2 {
			projectDir = os.Args[2]
		}
		if err := runInit(projectDir); err != nil {
			log.Fatalf("init: %v", err)
		}

	case "integrate":
		integrateCmd := flag.NewFlagSet("integrate", flag.ExitOnError)
		integrateAgent := integrateCmd.String("agent", "claude", "agent to integrate (claude)")
				_ = integrateCmd.Parse(os.Args[2:])
		if err := runIntegrate(".", *integrateAgent); err != nil {
			log.Fatalf("integrate: %v", err)
		}

	case "skill":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs skill <list|activate|deactivate|install|show> [args]")
			os.Exit(1)
		}
		runSkill(os.Args[2:])

	case "provider":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs provider <list|set|key|test|switch|init> [args]")
			os.Exit(1)
		}
		runProvider(os.Args[2:])

	case "kernel":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs kernel <start|list|stop|status> [args]")
			os.Exit(1)
		}
		runKernel(os.Args[2:])

	case "workspace":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: agentfs workspace <list|open|info> [args]")
			os.Exit(1)
		}
		runWorkspace(os.Args[2:])

	default:
		printUsage()
		os.Exit(1)
	}
}

func runMount(sourceDir, mountPoint string, allowOther bool) error {
	fsys, err := fs.NewFileSystem(sourceDir)
	if err != nil {
		return fmt.Errorf("create filesystem: %w", err)
	}

	server := fuseutil.NewFileSystemServer(fsys)

	mountConfig := &fuse.MountConfig{
		FSName:   "agentfs",
		ReadOnly: false,
	}
	_ = allowOther // TODO: add allow_other via MountConfig.Options map

	mfs, err := fuse.Mount(mountPoint, server, mountConfig)
	if err != nil {
		return fmt.Errorf("mount: %w", err)
	}

	log.Printf("agentfs mounted %s -> %s", sourceDir, mountPoint)
	return mfs.Join(context.Background())
}

func runUnmount(mountPoint string) error {
	return fuse.Unmount(mountPoint)
}

func runSession(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: agentfs session <start|list|get|open|mount|unmount|commit|discard|diff> [args]")
		os.Exit(1)
	}

	// Parse --daemon flag (for mount subcommand)
	daemonFlag := false
	remaining := []string{}
	for _, a := range args {
		if a == "--daemon" {
			daemonFlag = true
		} else {
			remaining = append(remaining, a)
		}
	}
	args = remaining

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: agentfs session <start|list|get|open|mount|unmount|commit|discard|diff> [args]")
		os.Exit(1)
	}

	mgr := sandbox.NewManager(sandbox.DefaultBaseDir())

	switch args[0] {
	case "start":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session start <project_dir>")
		}
		sess, err := mgr.StartSession(args[1])
		if err != nil {
			log.Fatalf("start session: %v", err)
		}
		fmt.Printf("Session %s started\n", sess.ID)
		fmt.Printf("  Project:    %s\n", sess.Project)
		fmt.Printf("  Workspace:  %s\n", sess.Workspace)

	case "list":
		sessions, err := mgr.ListSessions()
		if err != nil {
			log.Fatalf("list sessions: %v", err)
		}
		if len(sessions) == 0 {
			fmt.Println("No active sessions")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tPROJECT\tWORKSPACE\tSTATUS\tCREATED")
		for _, s := range sessions {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.ID[:8]+"...", s.Project, s.Workspace, s.Status,
				s.Created.Format(time.RFC3339))
		}
		w.Flush()

	case "commit":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session commit <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("commit: %v", err)
		}
		if err := mgr.CommitSession(sess.ID); err != nil {
			log.Fatalf("commit: %v", err)
		}
		fmt.Println("Session committed")

	case "discard":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session discard <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("discard: %v", err)
		}
		if err := mgr.DiscardSession(sess.ID); err != nil {
			log.Fatalf("discard: %v", err)
		}
		fmt.Println("Session discarded")

	case "diff":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session diff <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("diff: %v", err)
		}
		changes, err := mgr.DiffSession(sess.ID)
		if err != nil {
			log.Fatalf("diff: %v", err)
		}
		if len(changes) == 0 {
			fmt.Println("No changes detected")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "PATH\tSTATUS")
		for _, c := range changes {
			fmt.Fprintf(w, "%s\t%s\n", c.Path, c.Status)
		}
		w.Flush()

	case "get":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session get <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("get session: %v", err)
		}
		fmt.Printf("ID:        %s\n", sess.ID)
		fmt.Printf("Project:   %s\n", sess.Project)
		fmt.Printf("Workspace: %s\n", sess.Workspace)
		fmt.Printf("Status:    %s\n", sess.Status)
		fmt.Printf("Created:   %s\n", sess.Created.Format(time.RFC3339))

	case "open":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session open <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("get session: %v", err)
		}
		fmt.Printf("Workspace: %s\n", sess.Workspace)
		fmt.Println("Hint: xdg-open <workspace> to open in your file manager, or cd <workspace> to enter the directory.")

	case "mount":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session mount <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("get session: %v", err)
		}
		if sess.Status != sandbox.StatusActive {
			log.Fatalf("session %s is not active (status: %s)", sess.ID, sess.Status)
		}
		mountPoint := filepath.Join(sess.Workspace, ".agentfs", "mnt")
		// Clean up stale mount
		_ = fuse.Unmount(mountPoint)
		if err := os.MkdirAll(mountPoint, 0755); err != nil {
			log.Fatalf("create mount point: %v", err)
		}
		if daemonFlag {
			// Background mount: start FUSE in background and print mount point
			fsys, err := fs.NewFileSystem(sess.Workspace)
			if err != nil {
				log.Fatalf("create filesystem: %v", err)
			}
			server := fuseutil.NewFileSystemServer(fsys)
			mountConfig := &fuse.MountConfig{FSName: "agentfs", ReadOnly: false}
			mfs, err := fuse.Mount(mountPoint, server, mountConfig)
			if err != nil {
				log.Fatalf("mount: %v", err)
			}
			go func() {
				if err := mfs.Join(context.Background()); err != nil {
					log.Printf("session mount for %s exited: %v", sess.ID, err)
				}
			}()
			// Wait a moment for mount to stabilize
			time.Sleep(500 * time.Millisecond)
			fmt.Println(mountPoint)
			return
		}
		// Foreground mount (blocks until unmounted)
		fmt.Printf("Mounting workspace FUSE at %s... Press Ctrl+C to unmount.\n", mountPoint)
		if err := runMount(sess.Workspace, mountPoint, false); err != nil {
			log.Fatalf("mount: %v", err)
		}

	case "unmount":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs session unmount <session_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("get session: %v", err)
		}
		mountPoint := filepath.Join(sess.Workspace, ".agentfs", "mnt")
		if err := fuse.Unmount(mountPoint); err != nil {
			log.Fatalf("unmount: %v", err)
		}
		fmt.Println("Unmounted")

	default:
		fmt.Fprintf(os.Stderr, "Unknown session command: %s\n", args[0])
		os.Exit(1)
	}
}


// runKernel manages the agent kernel lifecycle with persistent state.
func runKernel(args []string) {
	store := kernel.NewStateStore(filepath.Join(sandbox.DefaultBaseDir(), "kernel.jsonl"))
	mgr := kernel.NewLifecycleManager()
	_ = store.Restore(mgr) // ignore error on first run

	var kargs []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			kargs = append(kargs, a)
		}
	}
	if len(kargs) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: agentfs kernel <start|list|stop|status> [args]")
		os.Exit(1)
	}

	switch kargs[0] {
	case "start":
		if len(kargs) < 2 {
			log.Fatal("Usage: agentfs kernel start <type> [--model <m>] [--provider <p>]")
		}
		agentType := kernel.AgentType(kargs[1])
		cfg := kernel.AgentConfig{MaxTokens: 200000}
		for idx := 2; idx < len(args); idx++ {
			switch args[idx] {
			case "--model":
				if idx+1 < len(args) { idx++; cfg.Model = args[idx] }
			case "--provider":
				if idx+1 < len(args) { idx++; cfg.Provider = args[idx] }
			}
		}
		id, err := mgr.Spawn(agentType, cfg)
		if err != nil { log.Fatalf("spawn agent: %v", err) }
		if err := mgr.Run(id); err != nil { log.Fatalf("run agent: %v", err) }
		_ = store.Snapshot(mgr)
		fmt.Printf("Agent %s spawned (type=%s, state=running)\n", id, agentType)

	case "list":
		agents := mgr.List("")
		if len(agents) == 0 { fmt.Println("No agents"); return }
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTYPE\tSTATE\tMODEL\tPROVIDER")
		for _, a := range agents {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				a.ID, a.Type, a.State, a.Config.Model, a.Config.Provider)
		}
		w.Flush()

	case "stop":
		if len(kargs) < 2 { log.Fatal("Usage: agentfs kernel stop <agent_id>") }
		if err := mgr.Kill(kernel.AgentID(kargs[1])); err != nil {
			log.Fatalf("stop agent: %v", err)
		}
		_ = store.Snapshot(mgr)
		fmt.Printf("Agent %s stopped\n", kargs[1])

	case "status":
		if len(kargs) < 2 { log.Fatal("Usage: agentfs kernel status <agent_id>") }
		a, ok := mgr.Get(kernel.AgentID(kargs[1]))
		if !ok { log.Fatalf("agent %s not found", kargs[1]) }
		data, _ := json.MarshalIndent(a, "", "  ")
		fmt.Println(string(data))

	default:
		fmt.Fprintf(os.Stderr, "Unknown kernel command: %s\n", kargs[0])
		os.Exit(1)
	}
}

// runWorkspace provides workspace management commands.
func runWorkspace(args []string) {
	mgr := sandbox.NewManager(sandbox.DefaultBaseDir())

	switch args[0] {
	case "list":
		sessions, err := sandbox.ListAllWorkspaces(sandbox.DefaultBaseDir())
		if err != nil {
			log.Fatalf("list workspaces: %v", err)
		}
		if len(sessions) == 0 {
			fmt.Println("No workspaces found")
			return
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tPATH\tLAST_ACTIVE")
		for _, s := range sessions {
			info, _ := os.Stat(s.Workspace)
			t := "-"
			if info != nil {
				t = info.ModTime().Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				filepath.Base(s.Workspace), s.Workspace, t)
		}
		w.Flush()

	case "open":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs workspace open <name_or_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("get workspace: %v", err)
		}
		fmt.Printf("Workspace: %s\n", sess.Workspace)
		fmt.Println("Hint: xdg-open <workspace> or agentfs-dir <workspace>")

	case "info":
		if len(args) < 2 {
			log.Fatal("Usage: agentfs workspace info <name_or_id>")
		}
		sess, err := mgr.GetSessionByPrefix(args[1])
		if err != nil {
			log.Fatalf("get workspace: %v", err)
		}
		fmt.Printf("ID:        %s\n", sess.ID)
		fmt.Printf("Project:   %s\n", sess.Project)
		fmt.Printf("Workspace: %s\n", sess.Workspace)
		fmt.Printf("Status:    %s\n", sess.Status)
		fmt.Printf("Created:   %s\n", sess.Created.Format(time.RFC3339))

		// Count files in workspace
		if entries, err := os.ReadDir(sess.Workspace); err == nil {
			var files, dirs int
			for _, e := range entries {
				if e.Type().IsDir() {
					dirs++
				} else {
					files++
				}
			}
			fmt.Printf("Contents:  %d files, %d dirs\n", files, dirs)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown workspace command: %s\n", args[0])
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: agentfs <command> [args]

Commands:
  version              Print version
  mount <src> <mnt>    Mount source directory at mount point
  unmount <mnt>        Unmount a previously mounted filesystem
  session              Manage sessions (Module 3)
    start <dir>         Start a new session for a project directory
    list                List all sessions
    get <id>            Show full details of a session
    open <id>           Print workspace path (use xdg-open to open in file manager)
    mount [--daemon] <id>  Mount FUSE semantic filesystem on session workspace
    unmount <id>        Unmount session FUSE filesystem
    commit <id>         Commit session changes back to project
    discard <id>        Discard session changes
    diff <id>           Show changes vs original project
  init                 Initialize project (Module 4)
  integrate            Configure agent integration (Module 4)
  kernel               Manage agent kernel (start, list, stop, status)
  skill                Manage skills (Module 6)
  provider             Manage providers (Module 7)
  workspace            Manage workspaces (list, open, info)

Mount flags:
  -allow_other         Allow other users to access the mount
`)
}