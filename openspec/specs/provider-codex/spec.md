# provider-codex Specification

## Purpose
OpenAI 公式 `codex` CLI を未改変のままヘッドレス（`codex exec`）およびセッション継続で起動し、環境変数の安全な分離・サンドボックス権限変換・JSONL イベント正規化・認証ファイル非接触を担保するための要件を定義する。

## Requirements

### Requirement: codex ヘッドレスおよび継続実行の引数構築
`codex` アダプタは、Phase 0 で確認した `codex exec --help` 契約に基づき、ヘッドレス実行用の引数（`exec --json`、モデル指定時の `-m`、作業ディレクトリ指定時の `-C`、およびセッション継続時の `exec resume <session_id> ...`）を構築しなければならない（SHALL）。権限オプション `--perm` は `read-only` を `--sandbox read-only`、`edit` を `--sandbox workspace-write` に変換し、`danger-full-access` 等の権限確認バイパスフラグは出力してはならない（MUST NOT）。

#### Scenario: edit 権限および作業ディレクトリ指定での引数生成
- **WHEN** `--cwd /path/to/repo` と `--perm edit` で `codex` のヘッドレス実行引数を生成する
- **THEN** `exec`、`--json`、`-C /path/to/repo`、`--sandbox workspace-write` が含まれ、`danger-full-access` 等の危険フラグは含まれない

#### Scenario: セッション継続時の引数生成
- **WHEN** 既存の `provider_session_id` を指定して `codex` の継続実行引数を生成する
- **THEN** Phase 0 契約に準拠した順序で `exec resume <provider_session_id>` を含む引数が構築される

### Requirement: codex 子プロセス環境変数の認証モード分離
`codex` アダプタは、`auth = "subscription"` 時に子プロセス環境変数から `CODEX_API_KEY` および `OPENAI_API_KEY` を必ず除去しなければならない（MUST）。`auth = "api_key"` 時は設定で指定された環境変数から値を読み取り、その呼び出しに限って `CODEX_API_KEY` として子プロセスへ渡さなければならない（SHALL）。

#### Scenario: サブスクモードでの API キー環境変数除去
- **WHEN** シェル環境に `CODEX_API_KEY` や `OPENAI_API_KEY` が設定されている状態で、`auth = "subscription"` の `codex` 子プロセス環境を構築する
- **THEN** 子プロセス環境から両環境変数が除去され、保存済みの公式 ChatGPT 認証のみが利用される

#### Scenario: API キーモードでの呼び出し単位キー注入
- **WHEN** `auth = "api_key"` および `api_key_env = "MY_OPENAI_KEY"` が設定されている状態で子プロセス環境を構築する
- **THEN** `MY_OPENAI_KEY` の値が当該子プロセスの `CODEX_API_KEY` にのみ設定される

### Requirement: codex JSONL 出力の正規化パース
`codex` アダプタは、`codex exec --json` が出力する JSONL の各行を Phase 0 fixture に照合して以下の正規化イベントへ変換しなければならない（SHALL）。
- `thread.started` → `session_started`（`thread_id` をセッション ID に設定）
- `item.*` で `item.type = agent_message` → `text`（`item.completed` で確定）
- `item.*` で `command_execution` / `file_change` / `mcp_tool_call` 等 → `tool_call` / `tool_result`
- `turn.completed`（`usage` を含む）→ `usage` および `completed`
- `turn.failed` / `error` → `error`

#### Scenario: Phase 0 fixture の全パターン解析
- **WHEN** `testdata/codex/` に保存された成功・ツール使用・エラーの fixture を 1 行ずつパーサへ入力する
- **THEN** `session_started`, `text`, `tool_call`, `tool_result`, `usage`, `completed`（およびエラー時の `error`）が期待どおり抽出され、未知イベントは `unknown` として保持される

### Requirement: codex 認証ファイルへの非アクセス
`codex` アダプタは、`~/.codex/auth.json` を読み取り・コピー・ログ出力してはならない（MUST NOT）。

#### Scenario: auth.json 非接触の確認
- **WHEN** `codex` アダプタの実装およびテストを実行する
- **THEN** `~/.codex/auth.json` へのファイルアクセスや参照は一切行われない
