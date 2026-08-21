# agent-sync

CLI tool that syncs Codex configuration (`AGENTS.md`, `.agents/skills/*`) to Claude Code with a single command.

See [docs/README.md](docs/README.md) for usage and design (Japanese).

## Quick start

```bash
go install github.com/lef237/agent-sync@latest
agent-sync            # sync all targets
agent-sync --check    # detect differences; exit 1 if any (for CI)
agent-sync --dry-run  # show what would change without applying
```

## How it works

| Item | Codex side | Claude side | Strategy |
| --- | --- | --- | --- |
| Instructions | `AGENTS.md` | `CLAUDE.md` | generate a `@AGENTS.md` import adapter |
| Skills | `.agents/skills/<name>/` | `.claude/skills/<name>` | symlink |

- `AGENTS.md` is the source of truth; only a thin adapter is generated on the Claude side.
- If `AGENTS.override.md` exists, it is imported instead (matches Codex precedence).
- Nested `services/billing/AGENTS.md` gets a `CLAUDE.md` in the same directory.
- Only the `<!-- agent-sync:start -->` / `<!-- agent-sync:end -->` block is managed in `CLAUDE.md`; hand-written Claude-specific content is preserved.
- Only symlinks it created are tracked in `.claude/.agent-sync.json`; stale links to removed skills are cleaned up, user-created `.claude/skills/` entries are untouched.

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