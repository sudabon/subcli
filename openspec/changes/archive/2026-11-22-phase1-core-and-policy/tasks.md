# Tasks

## 1. Go モジュールとプロジェクト骨格の初期化

- [x] 1.1 `go.mod`（Go 1.24+、`github.com/sudabon/subcli`）を作成し、`github.com/spf13/cobra`、`github.com/pelletier/go-toml/v2`、`github.com/shirou/gopsutil/v4` を追加して `go test ./...` が通る初期状態を確認する
- [x] 1.2 `cmd/subcli/main.go` と `internal/cli/root.go` の cobra 骨格、および `internal/provider/provider.go`（`AuthMode`, `Perm`, `RunRequest`, `Provider` インターフェースとレジストリ）を作成し、`go build ./cmd/subcli` が成功することを確認する

## 2. 設定読み込みとシークレットマスク付きロガー

- [x] 2.1 `internal/config` に `$XDG_CONFIG_HOME/subcli/config.toml` の読み込み・デフォルト値適用・バリデーション（`auth` 値検証、`max_concurrency > 0`、未知フィールド拒否による 1 プロバイダ 1 アカウント制限）を実装し、正常系・未存在・不正設定・複数アカウント拒否のユニットテストが通ることを確認する
- [x] 2.2 `$XDG_STATE_HOME/subcli/subcli.log` へ書き出すロガーとシークレットマスク処理（`sk-`, `AIza` 等のマスク、環境変数丸出し禁止）を実装し、シークレット文字列がログ出力時にマスクされることをユニットテストで検証する

## 3. 規約ガード（policy）と UnderAgent 検出

- [x] 3.1 `internal/policy/matrix.go` に §2.3 の許可マトリクスをデータテーブルとして定義し、`internal/policy/policy.go` に `Check(provider, auth, op, cc) Decision`（日本語拒否理由と公式参照 URL 付き）を実装し、`TestPolicyMatrix` で全セルと `agy` サブスクの `StdinIsTTY && !UnderAgent` 制約を検証する
- [x] 3.2 `internal/policy/agents.go` に Phase 0 で確定した環境変数チェックと `gopsutil/v4/process` による祖先プロセス最大 10 段走査を実装し、環境変数検出・プロセスチェーン検出・非エージェント環境のユニットテストが通ることを確認する

## 4. プロバイダ基本定義と対話コマンド（doctor / login / open）

- [x] 4.1 `internal/provider/{claude,codex,agy}` に Phase 0 契約に基づく `ID`, `Binary`, `Version`, `LoginArgv`, `InteractiveArgv`, `ChildEnv`（`subscription` 時の API キー環境変数除去を含む）、および暫定 `HeadlessArgv`（`ErrUnsupported`）を実装し、ユニットテストで `ChildEnv` と引数を検証する
- [x] 4.2 `internal/cli/doctor.go` を実装し、各公式 CLI の `exec.LookPath`・`--version`・公式ログインステータス確認（存在しない場合は未確認メッセージ、認証ファイル非参照）・`docs/COMPLIANCE.md` 30 日超過警告をユニットテストで検証する
- [x] 4.3 `internal/cli/login.go` を実装し、事前の `policy.Check(OpLogin)`、TTY/stdio 継承起動、終了コードのみ表示（ログイン結果非読取）をユニットテストで検証する
- [x] 4.4 `internal/cli/open.go` と OS 別実行ファイル（Unix `syscall.Exec` / Windows stdio 継承 `exec.Command`）を実装し、事前の `policy.Check(OpOpen)` と `--` 以降の追加引数転送をユニットテストおよび手動確認（他 AI エージェント配下からの `subcli open agy` 拒否と直接ターミナルからの起動）で検証する

## 5. 静的遵守テスト・CI・README 整備

- [x] 5.1 §7.4 の静的遵守テスト（`TestNoCredentialAccess`, `TestNetHTTPOnlyInGeminiAPI`, `TestNoDangerousFlags`, `TestPolicyMatrix`）を実装し、`go test ./...` で全合格することを確認する
- [x] 5.2 実 CLI・認証なしで `go test ./...` と `go vet ./...` を実行する `.github/workflows/ci.yml` を作成し、ローカルで同等コマンドがエラーなく通ることを確認する
- [x] 5.3 `README.md` に「各公式 CLI で各自がログインすること」「`subcli` は認証情報に触れないこと」「個人利用のローカルツールであること」を明記し、製品名・コマンド名に各社ブランド名が含まれないことを確認する
