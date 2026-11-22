# Tasks

## 1. 公式 CLI の契約採取

- [x] 1.1 `claude --version` および `claude --help` の出力を取得し、`docs/cli-contracts/claude-<version>.txt` に保存してバージョンと主要フラグが記録されていることを確認する
- [x] 1.2 `codex --version`、`codex --help`、および `codex exec --help`（`codex exec resume --help` を含む）の出力を取得し、`docs/cli-contracts/codex-<version>.txt` に保存して引数順序とフラグが記録されていることを確認する
- [x] 1.3 `agy --version` および `agy --help` の出力を取得し、`docs/cli-contracts/agy-<version>.txt` に保存して対話・ヘッドレス関連フラグが記録されていることを確認する

## 2. ヘッドレス出力 fixture の採取と機密情報検査

- [x] 2.1 `claude` ヘッドレス実行（無害な短いプロンプト）で成功・ツール使用・エラーの 3 パターンを採取し、`testdata/claude/{success,tool_use,error}.ndjson` として保存して各行が有効な JSON であることを確認する
- [x] 2.2 `codex exec --json`（無害な短いプロンプト）で成功・ツール使用・エラーの 3 パターンを採取し、`testdata/codex/{success,tool_use,error}.ndjson` として保存して各行が有効な JSON であることを確認する
- [x] 2.3 全 fixture ファイルからシークレットパターン（`sk-`, `AIza`, トークン類）や個人固有情報が混入していないことをパターン検索および目視で確認する

## 3. 環境変数・agy API キー切替可否の調査記録

- [x] 3.1 `agy` の API キーモード（Google AI Studio）を `~/.gemini/antigravity-cli/settings.json` を書き換えずに呼び出し単位で切り替えられるかを検証し、結論と根拠（呼び出し単位の切替方法または `internal/geminiapi` フォールバック採用）を `docs/cli-contracts/research-notes.md` に記録する（`testdata/agy/{success,tool_use,error}.ndjson` の実機採取は `GEMINI_API_KEY` 用意後の `phase4-agy-api-key-mode` 着手時へ移管）
- [x] 3.2 各公式 CLI（`claude`, `codex`, `agy`）のシェル実行ツール上で `env` を確認し、`UnderAgent` 検出に利用する環境変数名（機密値は除く）を `docs/cli-contracts/research-notes.md` に一覧記録する

## 4. 規約確認文書の作成と完了検証

- [x] 4.1 `spce.md` 付録 B の全参照 URL（Anthropic / OpenAI / Google）を再確認し、各社の公式見解要約・許可マトリクス方針・機械可読な確認日（`YYYY-MM-DD`）・参照 URL 一覧を `docs/COMPLIANCE.md` に作成する
- [x] 4.2 Phase 0 の全成果物（`docs/cli-contracts/`, `docs/COMPLIANCE.md`, `testdata/`）が揃い、認証ファイル・トークン・`settings.json` 変更が含まれていないことを `git status` と `git diff` で検証する
