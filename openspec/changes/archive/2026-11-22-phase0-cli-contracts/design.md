# Design

## Context

`subcli` の実装方針（`spce.md` §0, §9 Phase 0）では、公式 CLI（`claude`, `codex`, `agy`）のフラグ名・NDJSON/JSONL 出力スキーマ・環境変数名を推測でコーディングすることを禁止している。モチベーションは `proposal.md` を参照。本 Change はコード実装前の事実調査と契約固定に特化する。

## Goals / Non-Goals

**Goals:**
- `claude`, `codex`, `agy` の実機 `--version` / `--help` をバージョン付きテキストとしてリポジトリに固定する。
- `testdata/<provider>/*.ndjson` に成功・ツール使用・エラーの 3 パターンの実出力 fixture を配置し、Phase 2/4 のパーサ実装とオフラインテスト（CI）の根拠とする。
- `agy` API キーモードの呼び出し単位切替可否を事実ベースで確定し、Phase 4 の実装方式（CLI 起動 vs `internal/geminiapi` フォールバック）を事前決定する。
- `UnderAgent` 検出に使う環境変数名と `docs/COMPLIANCE.md` を確定する。

**Non-Goals:**
- Go の実装コード（`cmd/`, `internal/`）の作成。
- ユーザーの `~/.gemini/antigravity-cli/settings.json` や各公式 CLI の認証ファイルの読み取り・変更・コミット。
- `agy` サブスクリプションでのヘッドレス実行の調査・利用（`agy` ヘッドレス採取は API キーモードが呼び出し単位で安全に使える場合のみ実施）。

## Decisions

### 1. 契約ファイルの保存形式と命名
- **決定**: `docs/cli-contracts/<provider>-<version>.txt` に `--version` と対象 `--help` 出力をまとめる。追加の調査結果（`UnderAgent` 環境変数一覧、`agy` 切替可否）は `docs/cli-contracts/research-notes.md` に集約する。
- **理由**: バージョン単位で CLI 仕様の差分を追跡でき、後の CLI メジャー更新時（`spce.md` §12）に比較しやすい。
- **代替案**: README や設計書にインライン記載する案はバージョンごとの出力差分が追いづらく却下。

### 2. fixture の採取とサニタイズ方針
- **決定**: 無害な短いプロンプト（例: 単一テキスト応答、読み取り専用ツール 1 回呼び出し、無効モデル/引数によるエラー）で採取し、セッション中のパスやアカウント識別子・トークン類が含まれていないか目視およびパターン検索で検査してからコミットする。
- **理由**: 偽 CLI テストと CI（認証なし）で再利用する公開可能な fixture として安全性を担保するため。

### 3. `docs/COMPLIANCE.md` の日付フォーマット
- **決定**: `docs/COMPLIANCE.md` 内に機械可読な確認日（例: `Confirmed-Date: YYYY-MM-DD` または明示的メタデータ行）を含める。
- **理由**: Phase 1 の `subcli doctor` が規約確認日から 30 日経過警告を確実に判定できるようにするため。

## Risks / Trade-offs

- [公式 CLI の一部がローカル環境に未インストールまたは未ログイン] → 採取可能な CLI から順に記録し、未導入の CLI がある場合は作業を一時停止して開発者へ確認する（推測で補完しない）。
- [実出力 fixture に意図せずローカルパスや識別子が混入] → コミット前にシークレットパターン（`sk-`, `AIza`, OAuth トークン形式等）および機微情報を検査・マスクする。
- [各社規約が調査時点から更新されている] → §12 の手順に従い、変更があれば `docs/COMPLIANCE.md` に反映したうえで後続 Phase の設計（特に `policy/matrix.go`）へ反映する。
