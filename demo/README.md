# agent-sync demo

[Japanese guide](README.ja.md)

This demo captures the state of the directory after `agent-sync` has run. It includes both the Codex-side inputs and the generated Claude Code files: `CLAUDE.md`, the state file, and real symlinks.

## Post-sync layout

```text
demo/
├── AGENTS.md                         # Shared Codex instructions
├── CLAUDE.md                         # Generated import adapter
├── services/billing/
│   ├── AGENTS.md                     # Nested Codex instructions
│   └── CLAUDE.md                     # Generated import adapter
├── .agents/skills/
│   ├── release/SKILL.md              # Codex-side skill
│   └── review/SKILL.md               # Codex-side skill
└── .claude/
    ├── .agent-sync.json              # Managed symlink state
    └── skills/
        ├── release -> ../../.agents/skills/release
        └── review  -> ../../.agents/skills/review
```

`.agent-sync.json` records what agent-sync owns, per target: the symlinks it created and the files whose managed block it maintains. That record is what lets it clean up after itself — remove a skill or an `AGENTS.md` and the next sync withdraws exactly what it had added, leaving anything you wrote alone.

`AGENTS.md` is the source of truth. The managed block in `CLAUDE.md` imports the `AGENTS.md` in the same directory:

```markdown
<!-- agent-sync:start -->
@AGENTS.md
<!-- agent-sync:end -->
```

The symlink here is a filesystem link such as `.claude/skills/review`. The `@AGENTS.md` line in `CLAUDE.md` is text that tells Claude Code to load another file. They are two different mechanisms.

## Try the demo

If you run the command directly inside this repository, `agent-sync` finds the parent `.git` directory and treats the whole project as the target. Copy the demo to a temporary directory first so it becomes an independent repository.

```bash
# Run these commands from the agent-sync repository root.
AGENT_SYNC_ROOT="$(pwd)"
DEMO_ROOT="$(mktemp -d)"
BUILD_ROOT="$(mktemp -d)"
AGENT_SYNC_BIN="$BUILD_ROOT/agent-sync"

go build -o "$AGENT_SYNC_BIN" .
cp -R "$AGENT_SYNC_ROOT/demo/." "$DEMO_ROOT/"
git -C "$DEMO_ROOT" init
cd "$DEMO_ROOT"
```

### Verify the synchronized state

The checked-in demo is already synchronized, so `--check` exits with code `0`.

```bash
"$AGENT_SYNC_BIN" --check
echo "$?"   # 0
```

`--dry-run` also prints nothing because there are no pending actions:

```bash
"$AGENT_SYNC_BIN" --dry-run
```

Inspect the generated files with:

```bash
cat CLAUDE.md
cat services/billing/CLAUDE.md
readlink .claude/skills/review
cat .claude/.agent-sync.json
```

### Try adding a skill

Adding a skill directory creates a pending symlink, so `--check` reports a difference:

```bash
mkdir -p .agents/skills/audit
touch .agents/skills/audit/SKILL.md
"$AGENT_SYNC_BIN" --check
```

Expected output:

```text
ERROR: link .claude/skills/audit -> ../../.agents/skills/audit
```

Run a normal sync to create the symlink and update the state file:

```bash
"$AGENT_SYNC_BIN"
"$AGENT_SYNC_BIN" --check   # no output; exit code 0
```

Skill contents are accessed through the symlink, so editing a skill file does not require recreating the link. Adding or removing a skill directory does require a normal sync.

### Clean up

```bash
cd /
rm -rf "$DEMO_ROOT" "$BUILD_ROOT"
```

Before removing anything, confirm that `DEMO_ROOT` and `BUILD_ROOT` still refer to the temporary directories created by `mktemp -d`.
