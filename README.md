# agent-sync

CLI tool that syncs Codex configuration (`AGENTS.md`, `.agents/skills/*`) to Claude Code with a single command.

See [docs/README.md](docs/README.md) for usage and design (Japanese).

## Quick start

```bash
go install github.com/lef237/agent-sync@latest
agent-sync            # sync all targets
agent-sync --check    # detect differences; exit 1 if any (for CI)
agent-sync --dry-run  # show what would change without applying
agent-sync --version  # print the installed version
```

`--version` prints the tag for a binary installed with `go install`, and a
pseudo-version with the commit for one built from a working tree.

For a guided, self-contained example, see the [demo guide](demo/README.md). A [Japanese version](demo/README.ja.md) is also available.

## How it works

| Item | Codex side | Claude side | Strategy |
| --- | --- | --- | --- |
| Instructions | `AGENTS.md` | `CLAUDE.md` | generate a `@AGENTS.md` import adapter |
| Skills | `.agents/skills/<name>/` | `.claude/skills/<name>` | symlink |

- `AGENTS.md` is the source of truth; only a thin adapter is generated on the Claude side.
- If `AGENTS.override.md` exists, it is imported instead (matches Codex precedence).
- Nested `services/billing/AGENTS.md` gets a `CLAUDE.md` in the same directory.
- Only the `<!-- agent-sync:start -->` / `<!-- agent-sync:end -->` block is managed in `CLAUDE.md`; hand-written Claude-specific content is preserved.
- Delete an `AGENTS.md` and its block is withdrawn on the next sync: a `CLAUDE.md` that held nothing else is removed, one with hand-written content keeps it.
- Only what agent-sync created is tracked in `.claude/.agent-sync.json`, recorded per target. Stale links to removed skills are cleaned up; user-created `.claude/skills/` entries are never touched.
- If `.claude/skills/<name>` already exists and agent-sync did not create it, the skill is left alone and a warning is printed on stderr. Nothing is overwritten, and the exit code is unchanged.

### What gets scanned

`AGENTS.md` is looked for throughout the repository, except in:

- dot directories (`.git`, `.venv`, `.next`, ...) and `node_modules`, at any depth
- `build/`, `dist/`, `out/`, `target/`, `vendor/` **at the repository root only** — a nested `internal/target/AGENTS.md` is an ordinary source directory and is picked up
- anything `.gitignore` excludes; a force-added file still counts, and if git is unavailable nothing is excluded

A directory that cannot be read is different from one deliberately excluded: it is skipped with a warning so the rest of the sync still runs, but `--check` fails, because agent-sync cannot claim a subtree it never saw is in sync.

Skills are only read from `.agents/skills/` at the repository root, because Claude Code loads `.claude/skills` from the project root and nowhere else. A nested `.agents/skills` is reported as not synced rather than silently ignored.

The state file is at version 2. A version 1 file written by an earlier build is migrated in place on the next sync.

Skill syncing creates symlinks, which on Windows requires Developer Mode or an elevated shell. Instruction syncing works either way.

## Build & test

```bash
go build ./...
go test ./...
```

## Install

```bash
go install github.com/lef237/agent-sync@latest
```

## Layout

```text
main.go                 CLI entry point
internal/discovery/     repo root detection, AGENTS.md / skills discovery
internal/planner/       action types and managed-block editing
internal/target/        Target interface
internal/target/claude/ Claude adapter (desired-state planning)
internal/apply/         plan application and state file management
```

To add another target (e.g. Cursor), implement `internal/target.Target` and register it in `main.go`.
