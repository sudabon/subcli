# Design

## Context

Phase 0（`phase0-cli-contracts`）で採取した各公式 CLI の契約・環境変数・`docs/COMPLIANCE.md` を前提に、Go 1.24+ で `subcli` の骨格と安全装置を構築する。モチベーションは `proposal.md`、要求仕様は `specs/` 配下の 3 ファイルを参照。

## Goals / Non-Goals

**Goals:**
- `cmd/subcli/main.go` および `internal/{cli,config,policy,provider,logging}` のパッケージ構成を `spce.md` §4.1 に準拠して確立する。
- 許可マトリクスをデータテーブルとして一元管理し、全コマンドが子プロセス起動前に必ず `policy.Check` を通過する構造にする。
- 静的遵守テスト（§7.4）を最初から CI に組み込み、以降の Phase での規約逸脱を自動防止する。
- Unix の `open` で `syscall.Exec` によるプロセス置換を実現し、`subcli` が対話 TUI の入出力経路に介在しないことを保証する。

**Non-Goals:**
- ヘッドレス実行（`run`, `resume`, `sessions`, `compare`, `chain`）および NDJSON パーサ・セッション DB（Phase 2〜3）。
- `agy` API キーモードのスモークテストおよび `internal/geminiapi`・PTY 実行（Phase 4）。
- 複数アカウント切替、HTTP/MCP サーバ化、独自認証フロー実装。

## Decisions

### 1. ディレクトリ・パッケージ設計（`spce.md` §4.1 準拠）
- モジュールパス: `github.com/sudabon/subcli`（`git remote` に一致）。
- 主要パッケージ:
  - `cmd/subcli/main.go`: エントリポイント。
  - `internal/cli/`: `cobra` コマンド定義（`root`, `doctor`, `login`, `open`）。
  - `internal/config/`: `github.com/pelletier/go-toml/v2` で `$XDG_CONFIG_HOME/subcli/config.toml` を読み込み。未知フィールドを厳格拒否（`DisallowUnknownFields`）することで複数アカウント設定の混入を防止。
  - `internal/policy/`:
    - `matrix.go`: `map[matrixKey]Decision` のデータ駆動テーブルで §2.3 の全セルを定義（if 文の分散を禁止）。
    - `agents.go`: Phase 0 で確認した環境変数一覧と `github.com/shirou/gopsutil/v4/process` による祖先プロセス走査（最大 10 段）。
    - `policy.go`: `Check(provider, auth, op, cc) Decision` および `DetectCallContext()`。
  - `internal/provider/`: `Provider` インターフェース（`ID`, `Binary`, `Version`, `LoginArgv`, `InteractiveArgv`, `HeadlessArgv`, `ChildEnv`, `NeedsPTY`, `Parse`）とレジストリ。Phase 1 時点では `claude` / `codex` / `agy` の基本実装（`HeadlessArgv` は Phase 2/4 まで `ErrUnsupported`）を提供。
  - `internal/logging/`（または `internal/config` 隣接）: `$XDG_STATE_HOME/subcli/subcli.log` へ書き出すマスク済みロガー。正規表現で `sk-[A-Za-z0-9_-]+` や `AIza[A-Za-z0-9_-]+`、Bearer トークン様文字列を `[REDACTED]` へ置換。

### 2. `open` コマンドの OS 別プロセス起動（ビルドタグ分離）
- **決定**: `exec_unix.go`（`//go:build unix`）で `syscall.Exec(binPath, argv, env)`、`exec_windows.go`（`//go:build windows`）で `exec.Command` + stdio 継承を実装する。
- **理由**: Unix ではシェルのエイリアスと同等に `subcli` プロセス自体を置換でき、入出力や認証フローに一切介在しない。Windows は `syscall.Exec` が利用できないため代替実装を分離する。
- **注意**: `ChildEnv` を適用した環境変数を渡すことで、`subscription` モード時に意図しない API キー環境変数が対話セッションへ漏れることも防ぐ。

### 3. `doctor` の規約確認日パースとログイン状態確認
- **決定**: `docs/COMPLIANCE.md` は実行ファイルと同じリポジトリまたは埋め込み（`//go:embed` でビルド時基準日を保持しつつ、リポジトリ内に `docs/COMPLIANCE.md` があればそちらを優先）から確認日を読み、30 日超過で警告を出す。
- **理由**: `go install` やバイナリ単体配置時でも `subcli doctor` が規約確認日チェックを機能させられる。
- ログイン状態は Phase 0 で各 CLI に公式ステータス確認サブコマンドが存在すると確認されたものだけを呼び出し、存在しない場合は「未確認（`subcli login <provider>` で確認してください）」を出力する。

### 4. §7.4 静的遵守テストの実装方法
- `TestNoCredentialAccess`: `go/ast` またはファイル走査で `cmd/` と `internal/`（`*_test.go` 除く）をスキャンし、テストファイル内にのみ定義した禁止語リスト（認証ファイル名、キーチェーン API 等）が含まれないことを検証。
- `TestNetHTTPOnlyInGeminiAPI`: `go list -f '{{.ImportPath}}: {{.Imports}}' ./cmd/... ./internal/...`（および推移的チェックまたは直接/間接の境界チェック）で `net/http` を import するパッケージが `internal/geminiapi` のみであることを検証（注: 標準ライブラリやサードパーティ依存の間接 import とプロジェクト内直接 import の区別は `go list` の出力形式を明示設計し、Phase 4 の `google.golang.org/genai` 追加時にも意図通り機能させる）。
- `TestNoDangerousFlags`: 各プロバイダの `InteractiveArgv` / `HeadlessArgv` / `LoginArgv` の出力に危険フラグ（`bypassPermissions`, `danger-full-access`, `--dangerously-skip-permissions` 等）が含まれないこと、および `open` の追加引数経由での扱いを確認。
- `TestPolicyMatrix`: §2.3 全セル × TTY/UnderAgent の組み合わせを table-driven テストで網羅。

## Risks / Trade-offs

- [`gopsutil` のプロセス名取得が OS 間で微妙に異なる（macOS の 16 文字制限や `.exe` 拡張子）] → プロセス名の小文字正規化・拡張子除去・部分一致/ベース名比較を行い、環境変数検出と二重の防御線にする。
- [`TestNetHTTPOnlyInGeminiAPI` がサードパーティ依存（例: `cobra` や `sqlite`）の推移的 `net/http` で誤検知する可能性] → 自前パッケージ（`github.com/sudabon/subcli/...`）の直接 import と推移的 import を検証し、`internal/geminiapi` 以外から `net/http` を直接 import・使用させない検査と `go list -deps` 検査の境界を明確化する。
- [公式 CLI のログインコマンドやステータス確認コマンドが Phase 0 の実機と異なる] → Phase 0 で確定した `docs/cli-contracts/` を参照して `LoginArgv()` と `doctor` を実装する。
