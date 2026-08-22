# agent-sync demo

[English guide](README.md)

このデモは、`agent-sync` を実行した後の状態をそのまま収録したものです。Codex 側の入力だけでなく、Claude Code 側に生成される `CLAUDE.md`、状態ファイル、実際の symlink も確認できます。

## 同期後の構成

```text
demo/
├── AGENTS.md                         # Codex 側の共通指示
├── CLAUDE.md                         # 生成された import adapter
├── services/billing/
│   ├── AGENTS.md                     # ネストした Codex 側の指示
│   └── CLAUDE.md                     # 生成された import adapter
├── .agents/skills/
│   ├── release/SKILL.md              # Codex 側の Skill
│   └── review/SKILL.md               # Codex 側の Skill
└── .claude/
    ├── .agent-sync.json              # 管理対象 symlink の状態
    └── skills/
        ├── release -> ../../.agents/skills/release
        └── review  -> ../../.agents/skills/review
```

`.agent-sync.json` には、agent-sync が所有しているものをターゲットごとに記録しています。自分が作った symlink と、管理ブロックを維持している対象ファイルです。この記録があるので後片付けができます。Skill や `AGENTS.md` を消せば、次の同期で自分が足したものだけを取り下げ、手で書いたものには触りません。

`AGENTS.md` が source of truth です。`CLAUDE.md` の管理ブロックは次のように、同じディレクトリの `AGENTS.md` を読み込みます。

```markdown
<!-- agent-sync:start -->
@AGENTS.md
<!-- agent-sync:end -->
```

ここでの symlink は、`.claude/skills/review` のようなファイルシステム上のリンクです。一方、`CLAUDE.md` の `@AGENTS.md` は Claude Code に読み込みを指示するテキストです。両者は別の仕組みです。

## デモを試す

このリポジトリ内で直接実行すると、`agent-sync` は親の `.git` を見つけてプロジェクト全体を対象にします。そのため、デモだけを一時ディレクトリへコピーして実行します。

```bash
# agent-sync リポジトリのルートで実行する
AGENT_SYNC_ROOT="$(pwd)"
DEMO_ROOT="$(mktemp -d)"
BUILD_ROOT="$(mktemp -d)"
AGENT_SYNC_BIN="$BUILD_ROOT/agent-sync"

go build -o "$AGENT_SYNC_BIN" .
cp -R "$AGENT_SYNC_ROOT/demo/." "$DEMO_ROOT/"
git -C "$DEMO_ROOT" init
cd "$DEMO_ROOT"
```

### 同期済みであることを確認する

デモはすでに同期済みなので、`--check` は終了コード `0` になります。

```bash
"$AGENT_SYNC_BIN" --check
echo "$?"   # 0
```

`--dry-run` も、実行予定のアクションがないため何も表示しません。

```bash
"$AGENT_SYNC_BIN" --dry-run
```

生成されたファイルを確認するには、次を実行します。

```bash
cat CLAUDE.md
cat services/billing/CLAUDE.md
readlink .claude/skills/review
cat .claude/.agent-sync.json
```

### Skill の追加を試す

Skill のディレクトリを追加すると、必要な symlink がまだないため `--check` が差分を報告します。

```bash
mkdir -p .agents/skills/audit
touch .agents/skills/audit/SKILL.md
"$AGENT_SYNC_BIN" --check
```

期待される出力は次のとおりです。

```text
ERROR: link .claude/skills/audit -> ../../.agents/skills/audit
```

通常の同期を実行すると、symlink と状態ファイルが更新されます。

```bash
"$AGENT_SYNC_BIN"
"$AGENT_SYNC_BIN" --check   # 何も表示されず、終了コードは0
```

Skill のファイル内容は symlink 経由で参照されるため、内容を変更してもリンク自体の作り直しは必要ありません。Skill のディレクトリを追加・削除した場合は、通常の同期で symlink も追加・削除されます。

### 後片付け

```bash
cd /
rm -rf "$DEMO_ROOT" "$BUILD_ROOT"
```

`DEMO_ROOT` と `BUILD_ROOT` は直前の `mktemp -d` で作った一時ディレクトリを指していることを確認してから削除してください。
