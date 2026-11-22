# Proposal

## Why

`subcli` は複数社のサブスクリプション CLI を利用規約の範囲内で安全に起動するローカルランチャーであり、ヘッドレス実行などの高度な機能を追加する前に、規約違反操作を実行前に必ずブロックする安全装置（`policy` と静的遵守テスト）および基本コマンド骨格（`doctor`, `login`, `open`）を確立する必要がある（`spce.md` §9 Phase 1）。

## What Changes

- Go 1.24+ モジュール（`github.com/sudabon/subcli`）と `cobra` ベースの CLI 骨格（`cmd/subcli/main.go`, `internal/cli/`）、および基本 `provider` インターフェース／レジストリを導入する。
- TOML 設定読み込み（`$XDG_CONFIG_HOME/subcli/config.toml`、1 プロバイダ 1 アカウント制約）とシークレットをマスクするログ出力（`$XDG_STATE_HOME/subcli/subcli.log`）を実装する。
- 規約ガード `internal/policy`（許可マトリクス `policy/matrix.go`、`policy.Check`、`UnderAgent` 検出 `policy/agents.go`、`agy` サブスクの `StdinIsTTY && !UnderAgent` 制約）を実装する。
- 基本コマンド `doctor`, `login`, `open`（Unix は `syscall.Exec` でプロセス置換、Windows は stdio 継承 `exec.Command`）を実装する。
- §7.4 の静的遵守テスト（`TestNoCredentialAccess`, `TestNetHTTPOnlyInGeminiAPI`, `TestNoDangerousFlags`, `TestPolicyMatrix`）と、実 CLI／認証を使わない GitHub Actions CI、および §11 準拠の README 規約表記を整備する。

## Capabilities

### New Capabilities

- `policy-guard`: 許可マトリクスによる操作可否判定、`UnderAgent` ヒューリスティック検出、`agy` サブスク保護、1 アカウント制限、および CI で強制する静的遵守テスト。
- `config-and-logging`: `$XDG_CONFIG_HOME/subcli/config.toml` の読み込み・検証と、`$XDG_STATE_HOME/subcli/subcli.log` へのシークレットマスク付きログ出力。
- `interactive-commands`: `doctor` / `login` / `open` コマンド、プロバイダレジストリ基盤、CI ワークフローおよび README の規約表記。

### Modified Capabilities

<!-- 既存スペックなし -->

## Impact

- 新規作成コード・設定:
  - `go.mod`, `go.sum`, `cmd/subcli/main.go`
  - `internal/cli/`, `internal/config/`, `internal/policy/`, `internal/provider/`（`provider.go`, `claude/`, `codex/`, `agy/` の基本定義）
  - `.github/workflows/ci.yml`, `README.md`
- 主な外部依存:
  - `github.com/spf13/cobra`
  - `github.com/pelletier/go-toml/v2`
  - `github.com/shirou/gopsutil/v4/process`
- 前提 Change: `phase0-cli-contracts`（`docs/COMPLIANCE.md`、各 CLI の契約、`UnderAgent` 環境変数一覧）
