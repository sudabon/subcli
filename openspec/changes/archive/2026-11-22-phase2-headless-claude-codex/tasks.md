# Tasks

## 1. 正規化イベントと子プロセスランナー基盤

- [x] 1.1 `internal/event` に正規化イベント型（`Kind`, `Event`, `UsageInfo`）と JSON シリアライズを実装し、未知型 `Unknown` で `Raw` が保持されることをユニットテストで検証する
- [x] 1.2 `internal/runner` に `exec.CommandContext` ベースの子プロセスランナー（`ChildEnv` 適用、`bufio.Reader.ReadBytes('\n')` による長大行対応、64KB stderr リングバッファ、`Setpgid` による SIGINT→5s SIGTERM→5s SIGKILL シグナル転送、タイムアウト、終了コード伝播）を実装する
- [x] 1.3 `testdata/bin/` に偽 CLI スクリプト（`claude`, `codex` の fixture 再生・長大行出力・シグナル耐性・非ゼロ終了再現）を配置し、`internal/runner` のテストで長大行読み取り・stderr 64KB 保持・シグナル転送・タイムアウト・終了コード伝播がすべて通ることを確認する

## 2. claude / codex ヘッドレスアダプタ

- [x] 2.1 `internal/provider/claude` に Phase 0 契約準拠の `HeadlessArgv`（`-p`, `--output-format stream-json --verbose`, `--model`, `--permission-mode plan|acceptEdits`, `--resume`）、`ChildEnv`（`subscription` 時の `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN` 除去、`api_key` 時の指定環境変数転送）、および `Parse`（stream-json 正規化・未知型フォールバック）を実装し、Phase 0 fixture（成功・ツール使用・エラー）と未知行に対するユニットテストが通ることを確認する
- [x] 2.2 `internal/provider/codex` に Phase 0 契約準拠の `HeadlessArgv`（`exec --json`, `-m`, `-C`, `--sandbox read-only|workspace-write`, `exec resume`）、`ChildEnv`（`subscription` 時の `CODEX_API_KEY`/`OPENAI_API_KEY` 除去、`api_key` 時の呼び出し単位 `CODEX_API_KEY` 設定）、および `Parse`（JSONL 正規化・未知型フォールバック）を実装し、Phase 0 fixture（成功・ツール使用・エラー）と未知行に対するユニットテストが通ることを確認する
- [x] 2.3 Phase 1 の静的遵守テスト（`TestNoCredentialAccess`, `TestNoDangerousFlags`, `TestNetHTTPOnlyInGeminiAPI`）を実行し、`claude` / `codex` ヘッドレスアダプタ追加後も全合格することを確認する

## 3. セッションストア（SQLite）と sessions コマンド

- [x] 3.1 `modernc.org/sqlite` を追加し、`internal/session` に `$XDG_DATA_HOME/subcli/sessions.db`（パーミッション `0600`）の CRUD（作成・一覧・詳細・最終実行日時更新・削除、プロンプト先頭 80 文字ルーン切詰め＋シークレットマスク、トークン非保存）を実装し、パーミッションと CRUD をユニットテストで検証する
- [x] 3.2 `internal/cli/sessions.go` に `subcli sessions [list|show <id>|rm <id>]` を実装し、一覧・詳細・削除および未存在 ID エラーのユニットテストが通ることを確認する

## 4. レンダラーと run / resume コマンド

- [x] 4.1 `internal/render` に TTY レンダラー（プロバイダ別色付きプレフィックス、テキスト逐次表示、ツール呼び出し 1 行要約、usage 表示、サブスク時の「参考値（実際の請求ではありません）」注記）と `--json` JSONL レンダラーを実装し、出力形式をユニットテストで検証する
- [x] 4.2 `internal/cli/run.go`（`--model`, `--cwd`, `--perm read-only|edit` 既定 `read-only`, `--json`, `--no-store`, `--timeout` 既定 `30m`）および `internal/cli/resume.go`（`<local-session-id> "<prompt>"`）を実装し、実行前 `policy.Check`、偽 CLI を使った `run` → `sessions` → `resume` 往復テスト、および不正 `--perm` 拒否テストが通ることを確認する
- [x] 4.3 `go test ./...` と `go vet ./...` を実行して Phase 2 の全ユニット・ランナー・静的遵守テストが合格することを確認し、実機環境で `claude` / `codex` の `run` → `resume` 往復を手動確認する
