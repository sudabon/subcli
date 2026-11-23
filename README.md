# subcli

複数の公式コーディングエージェント CLI を、各社の利用規約の範囲内で安全に起動・切り替えするための**個人利用向けローカルランチャー**です。

## 重要な前提と規約遵守方針

- **各公式 CLI で各自がログインすること**: `subcli` は独自認証フローを持たず、各公式 CLI（`claude` / `codex` / `agy`）の正規ログイン手順をそのまま起動します。利用者はご自身の環境で、各公式 CLI に対してご自身のアカウントでログインしてください。
- **`subcli` は認証情報に触れないこと**: `subcli` は各公式 CLI の認証ファイル・OAuth トークン・OS キーチェーンを一切読み取り、コピー、保存、仲介しません。
- **個人利用のローカルツールであること**: `subcli` は単一ユーザー・個人利用のローカル CLI です。サーバ化・HTTP/MCP としての外部公開・複数アカウントの自動切替機能は提供しません。
- **実行前の規約ガード**: `login`・`open`・`run`・`resume`・`compare`・`chain` は実行前に `internal/policy` の許可マトリクスと実行コンテキスト検査（TTY・他 AI エージェント配下検出）を通過します。規約上制限された操作は子プロセスを起動せずに拒否します。

詳細は [docs/COMPLIANCE.md](docs/COMPLIANCE.md) を参照してください。

## アルファ版のコマンド

| コマンド | 説明 |
| --- | --- |
| `subcli doctor` | 各公式 CLI（`claude` / `codex` / `agy`）の検出、バージョン、公式ステータス確認（対応 CLI のみ）、および `docs/COMPLIANCE.md` の規約確認日（30 日以上で警告）を表示します |
| `subcli login <provider>` | 対象プロバイダの公式ログインコマンドを TTY/stdio 継承で起動し、終了コードのみを表示します |
| `subcli open <provider> [-- <extra-args>...]` | 対象プロバイダの対話 TUI を起動します（Unix では `syscall.Exec` でプロセス置換、Windows では stdio 継承で待機）。Git が追跡していないローカルの `.subcli/memory/MEMORY.md` があれば自動で共有メモリを接続します |
| `subcli memory <subcommand>` | リポジトリルート配下の `.subcli/memory/` でエージェント間共有メモリ（`.gitignore` 対象・チーム非共有）を初期化・追記・同期・インポートします |
| `subcli run <provider> "<prompt>"` | Claude または Codex で単発実行し、成功時にセッションを保存します |
| `subcli resume <id> "<prompt>"` | 保存したセッションで会話を続けます |
| `subcli sessions [list\|show\|rm]` | ローカルセッションを一覧・表示・削除します |
| `subcli compare --providers <p1,p2> "<prompt>"` | 同じプロンプトを複数のプロバイダで並列実行して結果を比較します |
| `subcli chain --steps <p1,p2> --prompt "<prompt>" --then "{{.Prev}}"` | 2〜3 段の処理を順に実行し、前段の出力を次段に渡します |

オプションと使用例は [使い方ガイド](USAGE.md) を参照してください。

## 設定ファイル

設定ファイルは `$XDG_CONFIG_HOME/subcli/config.toml`（未設定時は OS 標準の設定ディレクトリ配下の `subcli/config.toml`）から読み込みます。存在しない場合は全プロバイダが `auth = "subscription"`、`max_concurrency = 2` のデフォルト設定で動作します。

```toml
[providers.claude]
auth = "subscription"            # "subscription" | "api_key"
api_key_env = "MY_ANTHROPIC_KEY" # auth = "api_key" のときだけ参照する環境変数名
max_concurrency = 2

[providers.codex]
auth = "subscription"
api_key_env = "MY_OPENAI_KEY"
max_concurrency = 2

[providers.agy]
auth = "subscription"            # subscription では対話起動・ログイン（直接 TTY かつ非エージェント配下）のみ許可
api_key_env = "GEMINI_API_KEY"   # auth = "api_key" のとき参照する環境変数名
max_concurrency = 2
```

- 1 プロバイダ 1 アカウントのみ対応します（複数アカウント定義や未知フィールドはエラーになります）。
- 設定ファイルに API キーの生値は記述せず、参照する環境変数名（`api_key_env`）のみを指定してください。

## 開発・テスト

実公式 CLI や認証情報を使用せずに、ユニットテストおよび静的遵守テストを実行できます。

```bash
go vet ./...
go test -race ./...
go build ./cmd/subcli
```
