# subcli

複数の公式コーディングエージェント CLI を、各社の利用規約の範囲内で安全に起動・切り替えするための**個人利用向けローカルランチャー**です。

## 公式 CLI を単体で使う場合との違い

`subcli` は公式 CLI を置き換えるものではありません。公式 CLI を子プロセスとして起動し、その前後に共通の操作方法と安全策を加えます。`open` は Unix ではプロセス置換で起動するため、対話中の操作は公式 CLI を直接起動した場合と変わりません。

| 観点 | 公式 CLI を単体で使う場合 | `subcli` を使う場合 |
| --- | --- | --- |
| 非対話実行の呼び出し方 | CLI ごとに書き方が異なる（`claude -p --output-format stream-json ...` / `codex exec --json ...`） | `subcli run <provider> "<prompt>"` で同じように呼び出せる |
| 権限の指定 | CLI ごとに異なる（`--permission-mode plan` / `--sandbox read-only` など）。`codex exec resume` は `--sandbox` を受け付けない | `--perm read-only\|edit`（既定は `read-only`）に統一し、各 CLI に合った指定方法に変換する |
| 出力形式 | `claude` と `codex` で JSON イベントの構造が異なる | 共通のイベント形式に正規化し、`--json` を付けるとどちらも同じ構造の JSONL を出力する |
| 失敗の判定 | 終了コードが 0 でも、結果がエラーの場合がある | `claude` の `is_error` や `codex` の `turn.failed` を検出し、終了コード 1 を返す |
| 複数 CLI での比較 | それぞれ手動で実行し、結果を見比べる | `compare` で並列実行し、結果を横並び（端末幅 100 桁以上）または縦積みで表示する。1 つが失敗しても残りは最後まで実行する |
| CLI 間の受け渡し | 出力を手でコピーして次の CLI に渡す | `chain` で前段の出力を `{{.Prev}}` として次段のプロンプトに埋め込み、`claude → codex → claude` のように 2〜3 段をつなぐ |
| セッション管理 | CLI ごとに ID の形式も保存場所も異なる | `claude` と `codex` のセッションを共通のローカル ID で一覧・表示できる。`resume <id>` だけでプロバイダと作業ディレクトリも復元する |
| エージェント間のメモリ | CLI ごとに別のファイル（`CLAUDE.md` / `AGENTS.md` など）で管理する | `.subcli/memory/MEMORY.md` があれば、`subcli open` 時に 3 つの CLI のどれにも自動で接続する。リポジトリ直下の `CLAUDE.md` / `AGENTS.md` は変更しない。`memory init` で `.subcli/` を `.gitignore` に追加し、Git が追跡している場合は接続しない |
| API キーの環境変数 | シェルの環境変数（`ANTHROPIC_API_KEY` など）がそのまま公式 CLI に渡る | `auth = "subscription"` では子プロセスの環境から `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` / `GEMINI_API_KEY` などを取り除き、サブスクリプションの認証で動くようにする。`auth = "api_key"` では設定した環境変数名から値を渡す |
| 権限バイパスのフラグ | `--dangerously-skip-permissions` などを付けて起動できる | `open` の追加引数に `--dangerously-skip-permissions` / `--yolo` / `danger-full-access` / `approval_policy=never` などがあれば、起動前に拒否する。`run` 系は決まったフラグしか渡さない |
| 規約で制限された操作 | 利用者が自分で判断する | プロバイダ・認証モード・操作の組み合わせを許可マトリクスで判定する。制限されている場合は子プロセスを起動せず、理由と参照 URL を表示する（例：`agy` サブスクのヘッドレス実行、他の AI エージェント配下からの `agy` サブスク起動） |
| 環境の確認 | CLI ごとにバージョンやログイン状態を確認する | `subcli doctor` で 3 つの CLI の有無・バージョン・ログイン状態・規約確認日（30 日以上で警告）をまとめて表示する |
| 中断とタイムアウト | タイムアウトは自分で設定する（`timeout` コマンドなど） | `run` 系は既定で 30 分のタイムアウト。中断時はプロセスグループ全体に SIGINT → SIGTERM → SIGKILL を順に送り（Windows は Job Object）、孫プロセスを残さない |

**注意**

- `run` / `resume` / `compare` / `chain` が使えるのは `claude` と `codex` だけです。`agy` は `login` と `open` だけに対応しています。
- `run` 系で指定できるのは `--model` / `--perm` / `--cwd` / `--timeout` などに限られ、公式 CLI の任意のフラグは渡せません。細かいオプションが必要な場合は、公式 CLI を直接使うか、`subcli open <provider> -- <extra-args>` で対話起動してください。

## 重要な前提と規約遵守方針

- **各公式 CLI で各自がログインすること**: `subcli` は独自認証フローを持たず、各公式 CLI（`claude` / `codex` / `agy`）の正規ログイン手順をそのまま起動します。利用者はご自身の環境で、各公式 CLI に対してご自身のアカウントでログインしてください。
- **`subcli` は認証情報に触れないこと**: `subcli` は各公式 CLI の認証ファイル・OAuth トークン・OS キーチェーンを一切読み取り、コピー、保存、仲介しません。
- **個人利用のローカルツールであること**: `subcli` は単一ユーザー・個人利用のローカル CLI です。サーバ化・HTTP/MCP としての外部公開・複数アカウントの自動切替機能は提供しません。
- **実行前の規約ガード**: `login`・`open`・`run`・`resume`・`compare`・`chain` は実行前に `internal/policy` の許可マトリクスを通過します。規約上制限された操作は子プロセスを起動せずに拒否します。`agy`（サブスク）の `login` / `open` は、さらに実行コンテキスト検査（標準入力・標準出力がともに TTY で、他 AI エージェント配下でないこと）を通過した場合のみ許可します。

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
