# Tasks

## 1. 複数プロバイダ実行制御とレンダリング拡張

- [x] 1.1 プロバイダ別 `max_concurrency` セマフォを用いた並列実行オーケストレータを実装し、偽 CLI を使ったテストで同時起動プロセス数が `max_concurrency` を超えないことを検証する
- [x] 1.2 2〜3 段固定の非循環テンプレート展開エンジン（`text/template` による `{{.Prev}}` 展開、段数 `<2` または `>3` の事前拒否、前段失敗時の後続未起動）を実装し、ユニットテストで正常 2 段・3 段・段数超過・前段失敗時中断を検証する
- [x] 1.3 `internal/render` に `compare` 用の横並び TTY 表示（狭幅時の縦並びフォールバック含む）およびプロバイダ識別付き JSONL 出力を追加し、出力フォーマットをユニットテストで検証する

## 2. compare / chain コマンド実装

- [x] 2.1 `internal/cli/compare.go` に `subcli compare --providers <list> "<prompt>"`（`--cwd`, `--perm`, `--json`, `--timeout` 等）を実装し、実行前の全参加者 `policy.Check(OpCompare)` 一括検証と、`agy`（`auth=subscription`）を含む場合に 1 プロセスも起動せず全体拒否されることを偽 CLI テストで検証する
- [x] 2.2 `internal/cli/chain.go` に `subcli chain --steps <list> --prompt "<最初の依頼>" --then "<テンプレート>"` を実装し、実行前の全参加者 `policy.Check(OpChain)` 一括検証と、`agy`（`auth=subscription`）を含む場合に 1 プロセスも起動せず全体拒否されることを偽 CLI テストで検証する

## 3. 統合検証

- [x] 3.1 `go test ./...`（静的遵守テスト、偽 CLI を使った `compare` / `chain` 統合テスト含む）と `go vet ./...` が全合格することを確認し、手動で `claude` と `codex` の `compare` および `chain` の動作を検証する
