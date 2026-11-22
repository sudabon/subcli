# subcli 使い方ガイド（USAGE）

`subcli` は、複数の公式コーディングエージェント CLI（`claude` / `codex` / `agy`）を、**各社の利用規約の範囲内で安全に起動・実行・比較・連携するための個人利用向けローカルランチャー**です。

- 認証はすべて各公式 CLI の正規ログインフローに委ね、`subcli` 自身は認証ファイル・OAuth トークン・OS キーチェーンに一切触れません。
- 規約上制限された操作や危険な権限バイパスフラグは、子プロセスを起動する前にコード側（ポリシーガード）で自動的に拒否されます。

---

## 1. できること（プロバイダ別対応表）

デフォルト設定（各プロバイダ `auth = "subscription"`）で利用できる機能の一覧です。

| 機能 / コマンド | 説明 | `claude` (Claude Pro/Max 等) | `codex` (ChatGPT Plus/Pro 等) | `agy` (Google AI Pro/Ultra) |
| --- | --- | :---: | :---: | :---: |
| `doctor` | 公式 CLI の検出・バージョン・ログイン状態・規約確認日の診断 | ✅ | ✅ | ✅ |
| `login` | 各公式 CLI の正規ログイン画面を起動 | ✅ | ✅ | ✅ ※1 |
| `open` | 各公式 CLI の対話 TUI を起動（Unix はプロセス置換・`.subcli/memory` 自動連携） | ✅ | ✅ | ✅ ※1 |
| `memory` | リポジトリローカル共有メモリ（`.subcli/memory`、`.gitignore` 対象）の管理 | ✅ | ✅ | ✅ |
| `run` | 非対話（ヘッドレス）で単発プロンプトを実行 | ✅ | ✅ | ❌ ※2 |
| `resume` | 保存済みローカルセッション ID から会話を継続 | ✅ | ✅ | ❌ ※2 |
| `sessions` | 保存済みセッションの一覧・詳細表示・削除 | ✅ | ✅ | — |
| `compare` | 同じプロンプトを複数プロバイダへ並列実行して比較 | ✅ | ✅ | ❌ ※2 |
| `chain` | 前段の出力を次段の入力へ渡す 2〜3 段の直列連携 | ✅ | ✅ | ❌ ※2 |

- **※1**: `agy`（サブスク）の `login` / `open` は、**対話ターミナルから人間が直接起動した場合（標準入力が TTY かつ他の AI エージェント配下でない場合）**のみ許可されます。Claude Code や Codex のシェルツール内から実行した場合は自動的に拒否されます。
- **※2**: Google Antigravity のコンシューマー向けサブスクリプションでは、外部ツールからのヘッドレスラップや他 AI との自動連携が規約上の制限対象となるため、`subcli` 側で実行前に拒否します。

---

## 2. セットアップとビルド

### 前提条件
- Go 1.26 以上
- 利用したい公式 CLI が `PATH` 上にインストールされていること（`claude` / `codex` / `agy`）

### ビルド方法

```bash
# カレントディレクトリにバイナリを作成
go build -o subcli ./cmd/subcli

# $GOPATH/bin（通常 ~/go/bin）へインストールして PATH から実行できるようにする場合
go install ./cmd/subcli
```

### 初回セットアップ手順

```bash
# 1. 各公式 CLI の検出状況・バージョン・規約確認日を確認
subcli doctor

# 2. 各公式 CLI でご自身のアカウントにログイン（未ログインのもののみ）
subcli login claude
subcli login codex
subcli login agy
```

---

## 3. コマンド別リファレンス

### 3.1 `subcli doctor` — 環境と規約確認日の診断

各公式 CLI のインストール場所・バージョン・公式ログイン状態（`claude` / `codex`）および [docs/COMPLIANCE.md](docs/COMPLIANCE.md) の規約確認日を表示します。確認日から **30 日以上** 経過している場合は再確認を促す警告を表示します。

```bash
subcli doctor
```

---

### 3.2 `subcli login <provider>` — 公式ログインの起動

対象プロバイダの公式ログインフローを TTY/stdio 継承で起動します。`auth = "subscription"` 時は、意図しない API 課金を防ぐため子プロセスから API キー系環境変数を自動除外します。

```bash
subcli login claude   # claude auth login を起動
subcli login codex    # codex login を起動
subcli login agy      # agy 対話サインインを起動（直接 TTY のみ）
```

---

### 3.3 `subcli open <provider> [--memory | --no-memory] [-- <extra-args>...]` — 対話 TUI の起動

各公式 CLI の対話 TUI を起動します。Unix（macOS / Linux）では `syscall.Exec` により `subcli` プロセス自体を公式 CLI に置き換えるため、入出力の仲介を一切行いません。

リポジトリルートに `.subcli/memory/MEMORY.md` が存在する場合、起動前に各プロバイダ向け投影を自動同期して共有メモリを接続します（`--no-memory` で無効化、`--memory` で未初期化時に自動作成＋`.gitignore` 追記）。

```bash
# 基本的な起動（.subcli/memory/MEMORY.md があれば自動で共有メモリを接続）
subcli open claude
subcli open codex
subcli open agy

# 初回に共有メモリを自動初期化（.subcli/ 作成 + .gitignore 追記）しつつ起動
subcli open --memory claude

# 共有メモリを接続せずに起動
subcli open --no-memory agy

# 公式 CLI へ追加引数を渡す場合は `--` の後ろに記述
subcli open claude -- --model sonnet
subcli open codex -- -m o3
```

> **安全ガード**: `--dangerously-skip-permissions`、`bypassPermissions`、`danger-full-access` などの権限確認バイパス系フラグは `open` の追加引数に含めても拒否されます。

---

### 3.3.1 `subcli memory` — リポジトリローカル共有メモリの管理（チーム非共有）

リポジトリルートに `.subcli/memory/` を作成し、`claude` / `codex` / `agy` の対話セッション間で同じリポジトリの記憶を共有します。初期化時にリポジトリルートの `.gitignore` へ `.subcli/` を自動追記するため、個人の作業メモリがチームへ共有されることはありません。

#### ディレクトリ構成（リポジトリルート配下）

```text
<repo-root>/
├── .gitignore                          # `.subcli/` を自動追記（チーム非共有）
└── .subcli/
    ├── memory/                         # 正本（0700 / 0600・人間も直接編集可）
    │   ├── MEMORY.md                   # メイン共有メモリ & トピック索引
    │   └── <topic>.md                  # トピック別詳細ファイル（任意）
    └── projections/                    # subcli が自動生成する各 CLI 向け投影
        ├── claude/CLAUDE.md            # claude --add-dir 用
        └── agy/AGENTS.md               # agy --add-dir 用
```

#### 各プロバイダとの対話起動時連携

| プロバイダ | `subcli open` 時の接続方式 |
| --- | --- |
| **`claude`** | `--settings '{"autoMemoryDirectory":"<repo>/.subcli/memory"}'`（自動メモリの読み書き先を直接統一）＋ `--add-dir` と `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1` で投影 `CLAUDE.md` を読込 |
| **`codex`** | `--add-dir <repo>/.subcli/memory` ＋ `-c developer_instructions=...` で共有メモリと更新ルールを起動時注入 |
| **`agy`** | `--add-dir <repo>/.subcli/projections/agy` で投影 `AGENTS.md` をワークスペースルールとして自動読込（`agy` サブスクでも規約内で動作） |

#### サブコマンド一覧

```bash
# リポジトリルートに .subcli/memory/MEMORY.md を作成し .gitignore に .subcli/ を追記
subcli memory init

# 初期化状態・.gitignore 設定・ファイル数を確認（`subcli memory` 単体と同じ）
subcli memory status

# 共有メモリへ追記（API キーやトークンらしい文字列は自動でマスク）
subcli memory add "テストは go test -race ./... を実行する"
subcli memory add --topic architecture "policy.Check を必ず子プロセス起動前に呼ぶ"

# 内容表示・ファイル一覧・パス表示
subcli memory show
subcli memory show architecture
subcli memory list
subcli memory path

# MEMORY.md を手動編集したあとに各プロバイダ向け投影を再生成
subcli memory sync

# トピックファイルを削除（MEMORY.md の索引も更新）
subcli memory rm architecture

# 既存の Claude Code / Codex CLI のローカル Markdown メモリを読み取り専用で取り込む
subcli memory import claude
subcli memory import codex
subcli memory import claude --source /path/to/memory
```

---

### 3.4 `subcli run <provider> [flags] "<prompt>"` — ヘッドレス単発実行

`claude` または `codex` を非対話モードで 1 回実行し、正規化された結果を表示します。実行に成功するとローカルのセッション ID が自動保存され、標準エラー出力に `session: <id>` と表示されます。

```bash
# 読み取り専用（既定: --perm read-only）で要約を依頼
subcli run claude "README.md の内容を 3 行で要約して"

# 編集許可モード・モデル・作業ディレクトリを指定して実行
subcli run codex --perm edit --model o3 --cwd ./myproject "ユニットテストを追加して"

# 機械処理向けに正規化イベントを JSONL 形式で出力
subcli run claude --json "このプロジェクトの構成を説明して"

# セッション履歴に保存せず使い捨てで実行
subcli run claude --no-store "Go の context.WithTimeout の使い方を簡潔に教えて"
```

#### 主なフラグ

| フラグ | 既定値 | 説明 |
| --- | --- | --- |
| `--model <name>` | （CLI 既定） | 使用するモデル名 |
| `--cwd <dir>` | カレントディレクトリ | 実行時の作業ディレクトリ |
| `--perm <mode>` | `read-only` | 権限モード（`read-only` または `edit` のみ許可） |
| `--json` | `false` | 正規化イベントを 1 行 1 JSON（JSONL）で標準出力へ出力 |
| `--no-store` | `false` | ローカルセッション DB へ保存しない |
| `--timeout <duration>` | `30m` | 実行タイムアウト（例: `5m`, `90s`） |

> **補足**: サブスクリプションモード実行時のコスト表示には「参考値（実際の請求ではありません）」と明記されます。実行中に `Ctrl+C` を押すと、子プロセスグループへ `SIGINT` →（5 秒後）`SIGTERM` →（5 秒後）`SIGKILL` の順で安全に停止します。

---

### 3.5 `subcli sessions` — 保存済みセッションの管理

`subcli run` で保存されたローカルセッション履歴を確認・削除できます。DB にはローカル ID・プロバイダ・プロバイダ側セッション ID・作業ディレクトリ・日時・プロンプト先頭 80 文字（シークレット自動マスク済み）のみが保存され、認証情報は一切保存されません。

```bash
# セッション一覧を表示（`subcli sessions list` と同じ）
subcli sessions
subcli sessions list

# 特定セッションのメタデータ詳細を表示
subcli sessions show <local-session-id>

# 特定セッションを削除
subcli sessions rm <local-session-id>
```

---

### 3.6 `subcli resume <local-session-id> [flags] "<prompt>"` — セッションの継続実行

`subcli run` で保存されたローカルセッション ID を使って、前回の作業ディレクトリとプロバイダセッションを引き継いで続きを実行します。

```bash
subcli resume a1b2c3d4e5 "さきほどの要約を箇条書きにして"
subcli resume a1b2c3d4e5 --perm edit "提案してくれた修正を実装して"
```

#### 主なフラグ
- `--model <name>` / `--perm <read-only|edit>` / `--json` / `--timeout <duration>`（`run` と同様）

---

### 3.7 `subcli compare --providers <p1,p2,...> [flags] "<prompt>"` — 複数プロバイダの並列比較

同じプロンプトを複数プロバイダへ同時に投げ、結果を並べて比較します。端末幅が 100 桁以上のときは横並び、100 桁未満またはパイプ出力時は縦積みで表示されます。

```bash
# claude と codex の回答を並列取得して並べて比較
subcli compare --providers claude,codex "この設計のボトルネックと改善案を挙げて"

# JSONL 形式でイベントを出力（プロバイダ名・インデックス付き）
subcli compare --providers claude,codex --json "エラーハンドリングの方針を提案して"
```

#### 主なフラグ
- `--providers <p1,p2,...>`（必須）: 比較対象プロバイダのカンマ区切りリスト（例: `claude,codex`）
- `--model` / `--cwd` / `--perm` / `--json` / `--timeout`（`run` と同様）

#### 動作の特徴
- プロバイダごとに設定ファイル上の `max_concurrency`（既定 `2`）を厳守して並列数を制限します。
- 片方のプロバイダが失敗しても、他方の結果は最後まで取得して表示します（全体終了コードは非 0）。
- 参加者に `agy`（サブスク）など許可されないプロバイダが含まれている場合、**実行前に全体を拒否**し、他のプロバイダも一切起動しません。

---

### 3.8 `subcli chain` — 2〜3 段の直列連携（非循環パイプライン）

前段プロバイダの出力テキストを `{{.Prev}}` に展開して次段のプロバイダへ渡します。自動ループ防止のため、段数は **2〜3 段の固定・非循環** に制限されています。

```bash
# 2 段連携（claude で設計案を作成 → codex でレビュー）
subcli chain \
  --steps claude,codex \
  --prompt "認証ミドルウェアの実装案を出して" \
  --then "次の実装案をセキュリティ観点でレビューし、改善点を挙げて:\n\n{{.Prev}}"

# 3 段連携（段ごとに異なる --then テンプレートを指定）
subcli chain \
  --steps claude,codex,claude \
  --prompt "API 仕様案を作成して" \
  --then "次の仕様案の問題点を指摘して:\n{{.Prev}}" \
  --then "指摘を踏まえて最終版にまとめて:\n{{.Prev}}"

# JSONL 出力（step 番号付き）
subcli chain \
  --steps claude,codex \
  --prompt "テスト観点を列挙して" \
  --then "抜け漏れを補完して: {{.Prev}}" \
  --json
```

#### 主なフラグ
- `--steps <p1,p2[,p3]>`（必須）: 実行順プロバイダ（2〜3 段）
- `--prompt "<依頼>"`（必須）: 1 段目に渡すプロンプト
- `--then "<テンプレート>"`（必須・複数指定可）: 次段プロンプト用テンプレート（`{{.Prev}}` を利用可能。1 個指定すると全後続段で再利用、段数-1 個指定すると段ごとに適用）
- `--model` / `--cwd` / `--perm` / `--json` / `--timeout`

#### 動作の特徴
- テンプレート構文チェック・段数制限チェック・全参加プロバイダのポリシーチェックを 1 段目の起動前に完了させます。
- 途中段が失敗した場合や出力テキストが空の場合は即座に停止し、後続段を起動しません。

---

## 4. 設定ファイルと保存データ

設定ファイルを作成しなくても、デフォルト設定（全プロバイダ `auth = "subscription"`、`max_concurrency = 2`）でそのまま動作します。

### 設定ファイルの場所
- `$XDG_CONFIG_HOME/subcli/config.toml`
- `XDG_CONFIG_HOME` 未設定時は OS 標準の設定ディレクトリ配下の `subcli/config.toml`
  - macOS: `~/Library/Application Support/subcli/config.toml`
  - Linux: `~/.config/subcli/config.toml`

### 設定例（`config.toml`）

```toml
[providers.claude]
auth = "subscription"            # "subscription" | "api_key"
api_key_env = "ANTHROPIC_API_KEY" # auth = "api_key" のときのみ参照する環境変数名
max_concurrency = 2

[providers.codex]
auth = "subscription"
api_key_env = "CODEX_API_KEY"
max_concurrency = 2

[providers.agy]
auth = "subscription"            # サブスク利用時は login / open のみ許可
api_key_env = "GEMINI_API_KEY"
max_concurrency = 2
```

- **1 プロバイダ 1 アカウント制限**: 複数アカウント配列や別名プロバイダ定義、未知のキーは読み込み時にエラーになります。
- **API キー生値の記述禁止**: `api_key_env` に `sk-` や `AIza` で始まる生値を書くとエラーになります。

### ローカル保存ファイル一覧

| 用途 | 保存パス（既定） | パーミッション | 内容 |
| --- | --- | :---: | --- |
| 設定ファイル | `$XDG_CONFIG_HOME/subcli/config.toml` | 任意 | 認証モード・同時実行上限などの設定 |
| セッション DB | `$XDG_DATA_HOME/subcli/sessions.db`（既定 `~/.local/share/subcli/sessions.db`） | `0600` | セッションメタデータのみ（トークン非保存） |
| 動作ログ | `$XDG_STATE_HOME/subcli/subcli.log`（既定 `~/.local/state/subcli/subcli.log`） | `0600` | 実行ログ（API キー・トークン類は自動マスク） |

---

## 5. 規約遵守・安全ガードについて

`subcli` は以下の仕組みで規約違反や誤操作を防いでいます（詳細は [docs/COMPLIANCE.md](docs/COMPLIANCE.md) を参照）。

1. **実行前ポリシーガード**: 全コマンドが子プロセス起動前に許可マトリクスと実行コンテキスト（TTY / 他 AI エージェント配下検出）を検査し、禁止操作は理由と公式参照 URL を表示して終了します。
2. **認証情報への非接触**: 各公式 CLI の認証ファイルや OS キーチェーンには一切アクセスしません。
3. **意図しない API 課金の防止**: `auth = "subscription"` 時は、シェルに `ANTHROPIC_API_KEY`・`OPENAI_API_KEY`・`GEMINI_API_KEY` などが設定されていても子プロセスへ渡しません。
4. **危険フラグの遮断**: 権限確認をバイパスする危険フラグ（`--dangerously-skip-permissions` 等）はすべての経路で拒否します。
5. **規約確認日の監視**: `subcli doctor` が規約確認日（`docs/COMPLIANCE.md` の `Confirmed-Date`）から 30 日経過したことを検知して警告します。

---

## 6. よくある質問・トラブルシューティング

- **`拒否されました (open agy): 他の AI エージェント配下（検出理由: ...）からは agy（subscription）の open を実行できません`**
  - Claude Code や Codex などのエージェントのターミナルツール内から `subcli open agy` / `subcli login agy` を実行すると規約保護のため拒否されます。ご自身の通常ターミナルから直接実行してください。
- **`拒否されました (run agy): agy のサブスクリプション認証（Google AI Pro/Ultra）ではヘッドレス実行（run）は許可されていません`**
  - 仕様どおりの規約ガードです。Antigravity サブスクは `subcli open agy`（対話起動）でご利用ください。
- **公式 CLI が見つからないと表示される**
  - `subcli doctor` を実行し、対象の公式 CLI（`claude` / `codex` / `agy`）が `PATH` に通っているか確認してください。
- **テストを実行したい**
  - 実公式 CLI や認証情報を使わずに全ユニットテスト・静的遵守テストを実行できます：
    ```bash
    go vet ./...
    go test ./...
    ```
