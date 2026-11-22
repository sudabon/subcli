# provider-claude Specification

## Purpose
Anthropic 公式 `claude` CLI を未改変のままヘッドレスおよびセッション継続で起動し、環境変数の安全な分離・権限モード変換・stream-json イベント正規化・規約上の禁止事項を担保するための要件を定義する。

## Requirements

### Requirement: claude ヘッドレスおよび継続実行の引数構築
`claude` アダプタは、Phase 0 で確認した実 CLI 契約に基づき、ヘッドレス実行用の引数（`-p "<prompt>" --output-format stream-json --verbose`、モデル指定時の `--model`、およびセッション継続時の `--resume <session_id>`）を構築しなければならない（SHALL）。権限オプション `--perm` は `read-only` を `--permission-mode plan`、`edit` を `--permission-mode acceptEdits` に変換し、権限確認をバイパスするフラグ（`bypassPermissions`、`--dangerously-skip-permissions` 等）はいかなる条件でも出力してはならない（MUST NOT）。

#### Scenario: read-only 権限でのヘッドレス実行引数生成
- **WHEN** モデル指定と `--perm read-only` で `claude` のヘッドレス実行引数を生成する
- **THEN** `-p`、`--output-format stream-json`、`--verbose`、`--model`、`--permission-mode plan` が含まれ、危険フラグは含まれない

#### Scenario: セッション継続時の引数生成
- **WHEN** 既存の `provider_session_id` を指定して `claude` の継続実行引数を生成する
- **THEN** ヘッドレス引数に `--resume <provider_session_id>` が追加される

### Requirement: claude 子プロセス環境変数の認証モード分離
`claude` アダプタは、`auth = "subscription"` 時に子プロセス環境変数から `ANTHROPIC_API_KEY` および `ANTHROPIC_AUTH_TOKEN` を必ず除去しなければならない（MUST）。`auth = "api_key"` 時は設定で指定された環境変数から値を読み取り、`ANTHROPIC_API_KEY` として子プロセスへ渡さなければならない（SHALL）。

#### Scenario: サブスクモードでの API キー環境変数除去
- **WHEN** シェル環境に `ANTHROPIC_API_KEY` と `ANTHROPIC_AUTH_TOKEN` が設定されている状態で、`auth = "subscription"` の `claude` 子プロセス環境を構築する
- **THEN** 子プロセス環境から両環境変数が除去され、意図しない API 課金が発生しない

### Requirement: claude stream-json 出力の正規化パース
`claude` アダプタは、`stream-json` の各行を Phase 0 fixture に照合して以下の正規化イベントへ変換しなければならない（SHALL）。
- `type=system, subtype=init`（`session_id` を含む）→ `session_started`
- `type=assistant` の `text` コンテンツ → `text`
- `type=assistant` の `tool_use` コンテンツ → `tool_call`
- `type=user` の `tool_result` コンテンツ → `tool_result`
- `type=result`（`usage`, `total_cost_usd`, `session_id`）→ `usage` および `completed`（エラー系 `subtype` の場合は `error`）

#### Scenario: Phase 0 fixture の全パターン解析
- **WHEN** `testdata/claude/` に保存された成功・ツール使用・エラーの fixture を 1 行ずつパーサへ入力する
- **THEN** `session_started`, `text`, `tool_call`, `tool_result`, `usage`, `completed`（およびエラー時の `error`）が期待どおり抽出され、未知イベントは `unknown` として保持される

### Requirement: claude 認証情報および規約禁止事項の遵守
`claude` アダプタは、`~/.claude/` 配下の認証ファイルや macOS キーチェーンを読み取ってはならず（MUST NOT）、`CLAUDE_CODE_OAUTH_TOKEN` の設定や `claude setup-token` の代行を行ってはならない（MUST NOT）。

#### Scenario: 認証ファイル・OAuth トークン非干渉の確認
- **WHEN** `claude` アダプタのコードおよび実行時環境変数を検証する
- **THEN** 認証ファイルやキーチェーンへのアクセス、および `CLAUDE_CODE_OAUTH_TOKEN` の設定が一切存在しない
