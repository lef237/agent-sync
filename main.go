package main

import (
	"flag"
	"fmt"
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
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
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

	var targets []target.Target
	switch targetName {
	case "claude", "all":
		targets = []target.Target{claude.New()}
	default:
		fmt.Fprintln(os.Stderr, "agent-sync: unknown target:", targetName)
		return 2
	}

	exit := 0
	for _, t := range targets {
		plan, err := t.Plan(root, src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent-sync:", err)
			return 1
		}
		if *check {
			if plan.Empty() {
				continue
			}
			for _, a := range plan.Actions {
				fmt.Printf("ERROR: %s\n", a)
				exit = 1
			}
			continue
		}
		if *dry {
			for _, a := range plan.Actions {
				fmt.Printf("WOULD %s\n", a)
			}
			continue
		}
		st, err := apply.LoadState(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "agent-sync:", err)
			return 1
		}
		if err := apply.Apply(root, t.Name(), plan, st); err != nil {
			fmt.Fprintln(os.Stderr, "agent-sync:", err)
			return 1
		}
		if *verbose || !plan.Empty() {
			for _, a := range plan.Actions {
				fmt.Printf("%s\n", a)
			}
		}
	}
	return exit
}
