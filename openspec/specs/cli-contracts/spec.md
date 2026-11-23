# cli-contracts Specification

## Purpose
公式 CLI（`claude`, `codex`, `agy`）の実機バージョン・ヘルプ契約、ヘッドレス出力 fixture、環境変数調査結果、`agy` API キー切替検証結果、および規約確認文書を実装開始前にリポジトリへ記録・固定するための契約仕様を定義する。

## Requirements

### Requirement: 公式 CLI のバージョンとヘルプ契約の保存
本リポジトリは、`claude`、`codex`、`agy` 各公式 CLI の `--version` およびヘルプ出力（`claude --help`、`codex --help`、`codex exec --help`、`agy --help`）をバージョン付きファイルとして `docs/cli-contracts/<provider>-<version>.txt` に保存しなければならない（SHALL）。フラグ仕様やコマンド構造は推測で記述してはならない（MUST NOT）。

#### Scenario: 各プロバイダの契約ファイル採取
- **WHEN** Phase 0 の調査作業で各公式 CLI のバージョン確認およびヘルプ出力を取得する
- **THEN** `docs/cli-contracts/claude-<version>.txt`、`docs/cli-contracts/codex-<version>.txt`、`docs/cli-contracts/agy-<version>.txt` が作成され、実際の `--version` および `--help`（`codex` は `codex exec --help` を含む）の出力が記録される

### Requirement: ヘッドレス出力 fixture の採取と機密情報排除
本リポジトリは、Phase 0 のヘッドレス実行対象プロバイダ（`claude`, `codex`）の実出力を `testdata/<provider>/*.ndjson` として保存しなければならない（SHALL）。各プロバイダについて、無害な短いプロンプトを用いた「成功」「ツール使用」「エラー」の 3 パターンを採取し、認証情報・API キー・個人固有の機密情報を含めてはならない（MUST NOT）。`agy` の API キーモード用 fixture（`testdata/agy/*.ndjson`）は `GEMINI_API_KEY` が利用可能な環境で `phase4-agy-api-key-mode` 着手時に採取するものとし、サブスクリプション認証での採取は行ってはならない（MUST NOT）。

#### Scenario: 3 パターンの実出力 fixture 保存
- **WHEN** Phase 0 対象プロバイダ（`claude`, `codex`）のヘッドレスコマンドを無害な短いプロンプトで実行して出力を記録する
- **THEN** `testdata/<provider>/` 配下に成功・ツール使用・エラーの 3 パターンに対応する `.ndjson` ファイルが保存され、トークンや API キーが含まれないことが確認されている

### Requirement: agy API キーモードの呼び出し単位切替可否の検証記録
Phase 0 調査では、`agy` の API キーモード（Google AI Studio）をユーザーのグローバル設定ファイル（`~/.gemini/antigravity-cli/settings.json`）を書き換えることなく呼び出し単位（フラグ・環境変数・設定ディレクトリ指定等）で切り替えられるかを検証し、その結果を `docs/cli-contracts/` に記録しなければならない（SHALL）。

#### Scenario: 呼び出し単位の切替手段の判定
- **WHEN** `agy` CLI のヘルプおよび実機挙動を確認して呼び出し単位の API キー切替手段の有無を調査する
- **THEN** 切替手段の具体的な方法（存在する場合）または直接呼び出し不可のため Gemini API フォールバックを採用する判断（存在しない場合）が `docs/cli-contracts/` に記録され、ユーザーの `settings.json` は変更されない

### Requirement: UnderAgent 検出用環境変数の実機確認
Phase 0 調査では、各公式 CLI のシェル実行ツール上から環境変数を取得し、AI エージェント配下での実行を識別する環境変数名（例: `CLAUDECODE`、`CODEX_SANDBOX*` 等）を実機確認して記録しなければならない（SHALL）。

#### Scenario: 各 CLI 配下での環境変数一覧の確定
- **WHEN** 各公式 CLI のシェル実行機能から環境変数を確認する
- **THEN** エージェント配下判定に使用する環境変数名の一覧が機密値を除いた形で `docs/cli-contracts/` に記録される

### Requirement: 規約確認文書 docs/COMPLIANCE.md の整備
本リポジトリは、各社（Anthropic / OpenAI / Google）の公式見解要約、許可マトリクスの方針、規約確認日、および参照 URL を記載した `docs/COMPLIANCE.md` を保持しなければならない（SHALL）。

#### Scenario: 規約 URL の再確認と確認日の記録
- **WHEN** 参照 URL（Anthropic / OpenAI / Google の公式ドキュメントおよびフォーラム回答）を再確認する
- **THEN** `docs/COMPLIANCE.md` に各社の規約要約、最新の確認日（YYYY-MM-DD 形式）、および参照 URL 一覧が記録されリポジトリにコミットされる
