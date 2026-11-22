# Proposal

## Why

`claude` と `codex` は未改変の公式 CLI を起動し認証情報に触れない限り、サブスクリプション認証でのヘッドレス実行を含めて規約の範囲内で利用できる（`spce.md` §2.2）。これら 2 プロバイダのヘッドレス実行・セッション継続・履歴管理を統一フォーマットで提供し、偽 CLI ハーネスで決定的にテスト可能にする必要がある（`spce.md` §9 Phase 2）。

## What Changes

- 正規化イベント型 `internal/event`（未知フィールド・未知型を `Unknown` + `Raw` として保持）と子プロセスランナー `internal/runner`（`bufio.Reader.ReadBytes('\n')`、64KB stderr リングバッファ、`Setpgid` シグナル転送、タイムアウト、終了コード伝播）を実装する。
- `claude` および `codex` のヘッドレスアダプタ（フラグ変換、`--perm` マッピング、`ChildEnv` での意図しない API キー除去、Phase 0 fixture に準拠した NDJSON/JSONL パーサ、危険フラグ・認証ファイルアクセス禁止）を実装する。
- SQLite（`modernc.org/sqlite`、パーミッション `0600`）によるセッションストア `internal/session`（`$XDG_DATA_HOME/subcli/sessions.db`、認証情報非保存）と `subcli sessions [list|show|rm]` を実装する。
- `subcli run` / `subcli resume` コマンドと `internal/render`（TTY ストリーミング表示・サブスク参考値注記・`--json` JSONL 出力）を実装する。
- `testdata/bin/` に偽 CLI を配置したユニット・ランナーテストを追加する。

## Capabilities

### New Capabilities

- `headless-runner`: 正規化イベントモデル、安全な子プロセス実行・シグナル転送・エラーバッファリング・終了コード伝播、および偽 CLI テストハーネス。
- `provider-claude`: `claude` CLI のヘッドレス・継続実行アダプタ、環境変数サニタイズ、stream-json パーサ、および規約上の禁止事項遵守。
- `provider-codex`: `codex` CLI のヘッドレス・継続実行アダプタ、環境変数サニタイズ、JSONL パーサ、および規約上の禁止事項遵守。
- `session-store`: ローカル SQLite セッションメタデータ管理（`0600`、トークン非保存）と `subcli sessions` サブコマンド。
- `headless-commands`: `subcli run` / `subcli resume` コマンドおよび TTY / JSONL レンダリング。

### Modified Capabilities

<!-- 既存スペックなし -->

## Impact

- 新規・更新コード:
  - `internal/event/`, `internal/runner/`, `internal/render/`, `internal/session/`
  - `internal/provider/claude/`, `internal/provider/codex/`
  - `internal/cli/`（`run`, `resume`, `sessions` コマンド追加）
  - `testdata/bin/`（テスト用偽 CLI スクリプト）
- 主な追加依存:
  - `modernc.org/sqlite`（cgo 不要）
- 前提 Change: `phase0-cli-contracts`, `phase1-core-and-policy`
