# agent-sync

Codex 向けに配置した `AGENTS.md` や `.agents/skills/*` を、ワンコマンドで Claude Code にも適用する同期ツールです。

## 方針

| 対象 | Codex 側 | Claude 側 | 方式 |
| --- | --- | --- | --- |
| 指示ファイル | `AGENTS.md` | `CLAUDE.md` | `@AGENTS.md` import adapter を生成 |
| Skills | `.agents/skills/<name>/` | `.claude/skills/<name>` | symlink |

- `AGENTS.md` が正（source of truth）。Claude 側には薄い adapter だけを自動生成します。
- `AGENTS.override.md` が存在する場合は、それを import します（Codex の優先順位と一致）。
- ネストした `services/billing/AGENTS.md` にも、同じディレクトリに `CLAUDE.md` を生成します。
- `CLAUDE.md` の `<!-- agent-sync:start -->` / `<!-- agent-sync:end -->` マーカーの中だけを管理します。手書きの Claude 固有設定は保持されます。
- `AGENTS.md` を削除すると、次回の同期で対応する管理ブロックを取り下げます。管理ブロックしか無かった `CLAUDE.md` は削除し、手書き部分がある場合はそれを残します。
- agent-sync が作ったものだけを `.claude/.agent-sync.json` に、ターゲットごとに記録します。消えた Skill の古い symlink だけを削除し、手動で作った `.claude/skills/` の中身には触りません。
- `.claude/skills/<name>` が既に存在し agent-sync が作ったものでない場合は、その Skill をリンクせずに stderr へ警告を出します。上書きはせず、終了コードも変えません。

## 探索範囲

`AGENTS.md` はリポジトリ全体から探しますが、次は対象外です。

- ドットディレクトリ（`.git`, `.venv`, `.next` など）と `node_modules`（どの階層でも）
- `build/`, `dist/`, `out/`, `target/`, `vendor/` は**リポジトリ直下のみ**。ネストした `internal/target/AGENTS.md` は通常のソースディレクトリなので対象に含めます
- `.gitignore` で除外されているもの。ただし force-add 済みのファイルは対象に含め、git が使えない場合は除外しません

Skill はリポジトリ直下の `.agents/skills/` だけを読みます。Claude Code が `.claude/skills` をプロジェクト直下からしか読まないためです。ネストした `.agents/skills` は、黙って無視せず「同期対象外」と警告します。

状態ファイルは version 2 です。以前のビルドが書いた version 1 は、次回の同期時にその場で移行します。

## 使い方

```bash
agent-sync              # 全ターゲットを同期（現状は claude のみ）
agent-sync claude       # claude ターゲットだけを同期

agent-sync --check      # 差分を検出して exit 1（CI 用）
agent-sync --dry-run    # 変更内容だけ表示
agent-sync --verbose    # 詳細出力
```

同期後の状態を確認できるデモは、[日本語ガイド](../demo/README.ja.md) を参照してください。英語版は [demo/README.md](../demo/README.md) です。

## ビルド・テスト

```bash
go build ./...
go test ./...
```

## インストール

```bash
go install github.com/lef237/agent-sync@latest
```

## 構成

```text
main.go                CLI エントリポイント
internal/discovery/    repo root 検出・AGENTS.md / skills の探索
internal/planner/      アクション型と管理ブロックの編集
internal/target/       Target インターフェース
internal/target/claude/ Claude adapter（desired state の計画）
internal/apply/        計画の適用と状態ファイルの管理
```

将来 Cursor 等を足す場合は `internal/target/` に `Target` を実装し、`main.go` の `switch` に追加するだけです。
