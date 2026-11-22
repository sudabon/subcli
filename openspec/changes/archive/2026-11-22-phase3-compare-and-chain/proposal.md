# Proposal

## Why

複数の公式 CLI（`claude` / `codex`、および将来的に `agy` API キーモード）に同一プロンプトを並列投入して出力を比較したり、ある AI の出力を別 AI のレビュー入力へ最大 3 段まで一方向に引き継ぐワークフローは、個人開発の生産性を大きく高める。一方で、コンシューマー向け `agy` サブスクを並列比較や AI 間連携へ組み込むことや自動閉ループの構築は規約リスクが高いため、ポリシーガードと構造制限付きで実装する必要がある（`spce.md` §2.3, §5.7, §5.8, §9 Phase 3）。

## What Changes

- `subcli compare --providers <list> "<prompt>"` を追加し、指定プロバイダへ並列実行（プロバイダごとの `max_concurrency` を遵守）して結果を横並び TTY または JSONL で出力する。
- `subcli chain --steps <list> --prompt "<最初の依頼>" --then "<テンプレート>"` を追加し、前段の最終テキストを `{{.Prev}}` に展開して次段へ渡す 2〜3 段の非循環パイプラインを実装する（上限 3 段、自動ループ禁止）。
- `compare` / `chain` ともに、参加プロバイダのいずれかが `policy.Check` で拒否される場合（例: `agy` サブスクを含む場合）は、1 プロセスも起動せず実行前に全体を拒否する。

## Capabilities

### New Capabilities

- `multi-provider-workflows`: 並列比較コマンド `subcli compare` および 2〜3 段の非循環直列連携コマンド `subcli chain` の実行制御・同時実行制限・レンダリング・事前ポリシー拒否仕様。

### Modified Capabilities

<!-- 既存スペックなし -->

## Impact

- 新規・更新コード:
  - `internal/cli/`（`compare`, `chain` コマンド追加）
  - `internal/runner/` またはワークフロー調整パッケージ（`max_concurrency` セマフォ制御・並列実行・直列テンプレート展開）
  - `internal/render/`（`compare` 横並び TTY / JSONL 表示、`chain` ステップ進捗表示）
- 前提 Change: `phase1-core-and-policy`, `phase2-headless-claude-codex`
