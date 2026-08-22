package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lef237/agent-sync/internal/apply"
	"github.com/lef237/agent-sync/internal/discovery"
	"github.com/lef237/agent-sync/internal/target"
	"github.com/lef237/agent-sync/internal/target/claude"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	targetName := "all"
	targetSet := false
	filteredArgs := make([]string, 0, len(args))
	for _, a := range args {
		if !targetSet && !strings.HasPrefix(a, "-") {
			targetName = a
			targetSet = true
			continue
		}
		filteredArgs = append(filteredArgs, a)
	}
	args = filteredArgs

	fs := flag.NewFlagSet("agent-sync", flag.ContinueOnError)
	check := fs.Bool("check", false, "check for differences without applying; exit 1 if any")
	dry := fs.Bool("dry-run", false, "show what would be done without applying")
	verbose := fs.Bool("verbose", false, "verbose output")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: agent-sync [claude|all] [--check] [--dry-run] [--verbose]\n")
		fs.PrintDefaults()
	}
	// flag writes both the help text and parse errors to the same stream.
	// Help was explicitly asked for, so it belongs on stdout; a parse error
	// does not.
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stdout)
			fs.Usage()
			return 0
		}
		fs.SetOutput(os.Stderr)
		fmt.Fprintln(os.Stderr, "agent-sync:", err)
		fs.Usage()
		return 2
	}
	fs.SetOutput(os.Stderr)
	if rest := fs.Args(); len(rest) > 0 {
		fmt.Fprintln(os.Stderr, "agent-sync: unexpected argument:", strings.Join(rest, " "))
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-sync:", err)
		return 1
	}
	root, err := discovery.RepoRoot(cwd)
	if err != nil {
		root = cwd
	}
	src, err := discovery.Discover(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-sync:", err)
		return 1
	}
	for _, w := range src.Warnings {
		fmt.Fprintln(os.Stderr, "agent-sync: warning:", w)
	}
	if *verbose {
		// Which directory agent-sync decided to treat as the repository is
		// the least obvious part of a run, and the usual reason output is
		// not what the user expected.
		fmt.Printf("root: %s\n", root)
	}

	var targets []target.Target
	switch targetName {
	case "claude", "all":
		targets = []target.Target{claude.New()}
	default:
		fmt.Fprintln(os.Stderr, "agent-sync: unknown target:", targetName)
		return 2
	}

	exit := 0
	if *check && src.DiscoveryIncomplete {
		// Say so on stdout alongside the per-action failures. The stderr
		// warning explains which directory, but "warning" reads as survivable
		// and would leave a red run with no ERROR line to find.
		fmt.Println("ERROR: discovery was incomplete; cannot verify the repository is in sync")
		exit = 1
	}
	for _, t := range targets {
		plan, err := t.Plan(root, src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent-sync:", err)
			return 1
		}
		for _, w := range plan.Warnings {
			fmt.Fprintln(os.Stderr, "agent-sync: warning:", w)
		}
		switch {
		case *check:
			for _, a := range plan.Actions {
				fmt.Printf("ERROR: %s\n", a)
				exit = 1
			}
		case *dry:
			for _, a := range plan.Actions {
				fmt.Printf("WOULD %s\n", a)
			}
		default:
			st, err := apply.LoadState(root)
			if err != nil {
				fmt.Fprintln(os.Stderr, "agent-sync:", err)
				return 1
			}
			if err := apply.Apply(root, t.Name(), plan, st); err != nil {
				fmt.Fprintln(os.Stderr, "agent-sync:", err)
				return 1
			}
			for _, a := range plan.Actions {
				fmt.Printf("%s\n", a)
			}
		}
		if *verbose && plan.Empty() {
			fmt.Printf("%s: already in sync\n", t.Name())
		}
	}
	return exit
}
