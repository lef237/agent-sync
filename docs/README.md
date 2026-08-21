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
- 自分が作った symlink のみ `.claude/.agent-sync.json` で追跡し、消えた Skill の古い symlink だけを削除します。手動で作った `.claude/skills/` の中身には触りません。

## 使い方

```bash
agent-sync              # 全ターゲットを同期（現状は claude のみ）
agent-sync claude       # claude ターゲットだけを同期

agent-sync --check      # 差分を検出して exit 1（CI 用）
agent-sync --dry-run    # 変更内容だけ表示
agent-sync --verbose    # 詳細出力
```

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