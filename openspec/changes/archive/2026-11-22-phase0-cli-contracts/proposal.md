# Proposal

## Why

`subcli` は公式 CLI（`claude` / `codex` / `agy`）のフラグ名や出力形式を推測で実装せず、実機の挙動と最新規約に合わせて設計・実装する必要がある（`spce.md` §0, §9 Phase 0）。実装コードに着手する前に、各公式 CLI のバージョン・ヘルプ契約、実ヘッドレス出力 fixture、`agy` API キーモードの呼び出し単位切替可否、`UnderAgent` 検出用環境変数、および規約確認文書をリポジトリに確定させる。

## What Changes

- 各公式 CLI（`claude`, `codex`, `agy`）の `--version` および `--help`（`codex exec --help` 含む）出力を `docs/cli-contracts/<provider>-<version>.txt` に採取・記録する。
- 各プロバイダのヘッドレス出力実物（成功・ツール使用・エラーの 3 パターン、無害な短いプロンプト）を `testdata/<provider>/*.ndjson` として保存する。
- `agy` の API キーモード（Google AI Studio）を呼び出し単位（フラグ・環境変数・設定ディレクトリ）で切り替えられるか実機検証し、結果を `docs/cli-contracts/` に記録する。
- 各公式 CLI のシェル実行ツール上で `env` を実行し、`UnderAgent` 検出に使う環境変数名を確定して記録する。
- `spce.md` §2・付録 B の公式 URL を再確認し、`docs/COMPLIANCE.md`（規約要約・確認日・参照 URL）を作成する。

## Capabilities

### New Capabilities

- `cli-contracts`: 公式 CLI のコマンド契約（`--version`/`--help`）、ヘッドレス出力 fixture、環境変数調査結果、`agy` API キー切替検証結果、および規約確認文書 `docs/COMPLIANCE.md` の採取・記録仕様。

### Modified Capabilities

<!-- 既存スペックなし -->

## Impact

- 新規作成ファイル:
  - `docs/cli-contracts/<provider>-<version>.txt`（および調査メモ）
  - `docs/COMPLIANCE.md`
  - `testdata/claude/*.ndjson`, `testdata/codex/*.ndjson`（および `agy` API キーモード対応時は `testdata/agy/*.ndjson`）
- 後続 Change への影響:
  - `phase1-core-and-policy`（`policy/agents.go` の環境変数一覧、`doctor` の規約確認日チェック）
  - `phase2-headless-claude-codex`（各アダプタのフラグ・NDJSON パーサとテスト fixture）
  - `phase4-agy-api-key-mode`（`agy` CLI 呼び出し単位切替か `internal/geminiapi` フォールバックかの分岐確定）
