# Phase 0 CLI Contracts Research Notes

Confirmed-Date: 2026-11-22

本ドキュメントは Phase 0（`phase0-cli-contracts`）の実機調査結果（`agy` API キーモードの呼び出し単位切替検証、`UnderAgent` 検出用環境変数一覧、および後続 Phase 向けのスキーマ・CLI 契約メモ）を記録する。

---

## 1. `agy` API キーモードの呼び出し単位切替検証（Task 3.1）

### 結論

**`agy` CLI（確認バージョン `1.5.14`）は、`~/.gemini/antigravity-cli/settings.json` を書き換えることなく、呼び出し単位で Google AI Studio（Gemini API キー）モードへ安全に切り替え可能である。**

したがって、Phase 4（`phase4-agy-api-key-mode`）では `internal/geminiapi` フォールバックではなく、`agy` CLI の呼び出し単位切替を第一方式として採用できる。

### 呼び出し単位の切替方法

1. CLI フラグに `--settings '{"modelProvider":"gemini"}'`（または一時 JSON ファイルパス）を渡す。
   - `agy --help`（`1.5.14`）の定義:
     `--settings  Settings for this session only, never saved: a JSON object of settings.json keys, inline or as a file path`
   - 公式リファレンス（`https://antigravity.google/docs/cli/reference.md`）の `settings.json` キー定義:
     `modelProvider`: `"gemini"` sends model requests directly to the Gemini API with the key in `GEMINI_API_KEY`.
2. 子プロセス環境変数に `GEMINI_API_KEY=<value>`（`config.toml` の `api_key_env` で指定された環境変数から取得）を設定する。
3. ユーザーの `~/.gemini/antigravity-cli/settings.json` は一切読み取り・変更しない。

### 実機検証の根拠

`GEMINI_API_KEY` を未設定にした状態（かつネットワーク遮断・ワークスペース外書き込み不可のサンドボックス内）で以下を実行した:

```bash
env -u GEMINI_API_KEY agy --settings '{"modelProvider":"gemini"}' --print-timeout 5s -p "hi" --output-format stream-json
```

実測出力（終了コード `1`）:

```text
modelProvider is set to "gemini" in settings.json, but the GEMINI_API_KEY environment variable is not set. Set GEMINI_API_KEY to your Gemini API key, or remove "modelProvider" from settings.json to use the default backend.
```

この挙動により以下が確認できた:

- `--settings '{"modelProvider":"gemini"}'` が当該呼び出しのみに適用される。
- `GEMINI_API_KEY` が未設定の場合、サブスクリプション認証へ暗黙フォールバックせず即座にエラー終了する（意図しないサブスク消費が起きない）。
- `--settings` は「for this session only, never saved」であり、ユーザーの `settings.json` を変更しない。

### `testdata/agy/*.ndjson` 採取状況（Phase 4 への移管）

- 本調査時点のローカル環境では `GEMINI_API_KEY` が未設定であったため、API キーモードによる `agy -p --output-format stream-json` の実機 fixture（`testdata/agy/{success,tool_use,error}.ndjson`）の採取は **`phase4-agy-api-key-mode` 着手時（API キー用意後）へ移管**する方針とした。
- 規約方針（`docs/COMPLIANCE.md` §3）に従い、サブスクリプション認証による `agy -p` の実行・採取は行っていない。
- Phase 4 着手時の採取コマンド例（`GEMINI_API_KEY` を設定したシェルで実行、`~/.gemini/antigravity-cli/settings.json` は変更不要）:
  ```bash
  agy --settings '{"modelProvider":"gemini"}' -p "Reply with exactly: hello" --output-format stream-json > testdata/agy/success.ndjson
  agy --settings '{"modelProvider":"gemini"}' --mode plan -p "Read README.md and reply with its first line only." --output-format stream-json > testdata/agy/tool_use.ndjson
  agy --settings '{"modelProvider":"gemini"}' -p "hi" --model invalid-model-xyz-000 --output-format stream-json > testdata/agy/error.ndjson
  ```
- なお、公式ドキュメント（`https://antigravity.google/docs/cli/headless.md`）には `stream-json` のイベント形式（`init` / `step_update` / `result`、`tool_info`、`status` 値）が記載されている。

---

## 2. `UnderAgent` 検出用環境変数の実機確認（Task 3.2）

各公式 CLI のシェル実行ツール上で `env | cut -d= -f1 | sort` を実行し、エージェント配下実行時に設定される環境変数名を確認した（機密値を含む環境変数の値は一切取得・記録していない）。

### 2.1 `claude`（Claude Code `2.1.330`）の Bash ツール内で確認された変数名

- `CLAUDECODE`
- `CLAUDE_CODE_ENTRYPOINT`
- `CLAUDE_CODE_EXECPATH`
- `CLAUDE_CODE_SESSION_ID`
- `CLAUDE_CODE_SESSION_ATTENDED`
- `CLAUDE_CODE_CHILD_SESSION`
- `CLAUDE_CODE_CHILD_NOTES_DIR`
- `CLAUDE_CODE_MESSAGING_SOCKET`
- `CLAUDE_CODE_MESSAGING_TOKEN`（※トークン系のため存在チェックのみ行い値は参照・出力しない）
- `CLAUDE_PID`
- `CLAUDE_EFFORT`
- `CLAUDE_PLUGIN_DATA`
- `AI_AGENT`

### 2.2 `codex`（Codex CLI `0.171.0`）のシェル実行内で確認された変数名

- `CODEX_CI`
- `CODEX_SANDBOX`
- `CODEX_SANDBOX_NETWORK_DISABLED`
- `CODEX_SESSION_ID`
- `CODEX_THREAD_ID`
- `CODEX_TOOL_CALL_ID`
- `CODEX_VERSION`
- `CODEX_APPLY_PATCH_PRESERVE_LINE_ENDINGS`

### 2.3 `agy`（Antigravity CLI `1.5.14`）の `run_command` ツール内で確認された変数名

- `ANTIGRAVITY_AGENT`
- `ANTIGRAVITY_AGENTAPI_EXE`
- `ANTIGRAVITY_APP_DATA_DIR`
- `ANTIGRAVITY_CONVERSATION_ID`
- `ANTIGRAVITY_CSRF_TOKEN`（※トークン系のため存在チェックのみ行い値は参照・出力しない）
- `ANTIGRAVITY_LS_ADDRESS`
- `ANTIGRAVITY_LS_VERSION`
- `ANTIGRAVITY_PROJECT_ID`
- `ANTIGRAVITY_SOURCE_METADATA`
- `ANTIGRAVITY_TRAJECTORY_ID`

### 2.4 Phase 1 `policy/agents.go` への推奨リスト

誤検知を避けつつ確実に検出するため、以下の環境変数名（完全一致またはプレフィックス一致）を採用することを推奨する:

- **Claude Code**: `CLAUDECODE`, `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_SESSION_ID`, `CLAUDE_PID`
- **Codex CLI**: `CODEX_SANDBOX`, `CODEX_SANDBOX_NETWORK_DISABLED`, `CODEX_THREAD_ID`, `CODEX_SESSION_ID`, `CODEX_CI`
- **Antigravity CLI**: `ANTIGRAVITY_AGENT`, `ANTIGRAVITY_CONVERSATION_ID`, `ANTIGRAVITY_PROJECT_ID`, `ANTIGRAVITY_TRAJECTORY_ID`
- **汎用 AI エージェント指標**: `AI_AGENT`

※これに加え、`openspec/specs/policy-guard/spec.md` に従い `gopsutil` による親プロセスチェーン検査（`claude`, `codex`, `agy`, `opencode`, `openclaw`, `cursor-agent` 等）を併用する。

---

## 3. 後続 Phase 向けの CLI 契約・NDJSON スキーマ重要発見事項

実機採取（`docs/cli-contracts/` および `testdata/`）を通じて、当初の想定に対して実装時に注意すべき実機仕様が確認された。

### 3.1 公式ログイン・認証ステータス確認サブコマンド（Phase 1 `doctor` / `login` 向け）

- **`claude` (`2.1.330`)**:
  - ログイン: `claude auth login`（`claude auth --help` で確認済み）
  - ステータス確認: `claude auth status`（公式サブコマンドあり。認証ファイルを直接読まずこれを利用可能）
- **`codex` (`0.171.0`)**:
  - ログイン: `codex login`
  - ステータス確認: `codex login status`（公式サブコマンドあり）
- **`agy` (`1.5.14`)**:
  - ログイン: `agy`（専用 `login` サブコマンドなし、対話起動でサインインフロー）
  - ステータス確認: 専用サブコマンドなし（`doctor` では未確認表示とし、認証ファイルは読まない）

### 3.2 `codex exec resume` の引数順序とフラグ差分（Phase 2 向け）

- `codex exec resume --help` の実機構文:
  `codex exec resume [OPTIONS] [SESSION_ID] [PROMPT]`
- `codex exec` と異なり、`codex exec resume` には `-s/--sandbox` および `-C/--cd` オプションが存在しない（`-m/--model`, `--json`, `-c/--config` は利用可能）。
- そのため `resume` 時に作業ディレクトリを指定する場合は `exec.Cmd.Dir` を設定し、サンドボックス設定が必要な場合は `-c` オプション等の扱いを確認する必要がある。

### 3.3 `claude` `stream-json` のエラーイベント形式（Phase 2 パーサ向け）

- `testdata/claude/error.ndjson` で確認された通り、無効なモデル指定時の `type="result"` 行は:
  - `"is_error": true`
  - `"terminal_reason": "api_error"`
  - `"api_error_status": 404`
  - **`"subtype": "success"`**（エラー時でも `subtype` が `"success"` になり得る）
- また直前の `type="assistant"` 行にも `"error": "model_not_found"`, `"is_api_error_message": true` が含まれる。
- **Phase 2 実装への影響**: `result` 行のエラー判定は `subtype` だけでなく `is_error == true` を必ずチェックすること。
- その他、`type="system", subtype="thinking_tokens"`、`type="system", subtype="hook_started" / "hook_response"`、`type="rate_limit_event"`、`content[].type="thinking"` が実出力に現れるため、`Unknown` イベント保持設計（`openspec/specs/provider-claude/spec.md`）で安全に扱うこと。

### 3.4 `codex exec --json` のイベント形式（Phase 2 パーサ向け）

- `testdata/codex/{success,tool_use,error}.ndjson` で確認されたイベント種別:
  - `thread.started`（`thread_id`）
  - `turn.started`
  - `item.started` / `item.completed`（`item.type`: `agent_message`, `command_execution`, `error`）
  - `turn.completed`（`usage`: `input_tokens`, `cached_input_tokens`, `cache_write_input_tokens`, `output_tokens`, `reasoning_output_tokens`）
  - `error`（トップレベル `message`）
  - `turn.failed`（`error.message`）
