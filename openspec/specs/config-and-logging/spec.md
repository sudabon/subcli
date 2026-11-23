# config-and-logging Specification

## Purpose
`subcli` の個人ローカル設定（プロバイダ別認証モード、API キー参照先環境変数名、並列上限）を安全に読み込み・検証し、シークレットを漏洩させないログ記録を実現するための仕様を定義する。

## Requirements

### Requirement: TOML 設定ファイルの読み込みと検証
システムは `$XDG_CONFIG_HOME/subcli/config.toml`（`$XDG_CONFIG_HOME` 未設定時は OS 標準の設定ディレクトリ）から設定を読み込まなければならない（SHALL）。各プロバイダ（`claude`, `codex`, `agy`）について設定可能な項目は、`auth`（`subscription` または `api_key`）、`api_key_env`（`auth=api_key` 時に読み取る環境変数名）、および `max_concurrency`（正の整数）とし、同一プロバイダに複数アカウントを定義する構成を許可してはならない（MUST NOT）。

#### Scenario: 正常な設定ファイルの読み込み
- **WHEN** 各プロバイダに有効な `auth`, `api_key_env`, `max_concurrency` が記述された `config.toml` が存在する
- **THEN** システムは各プロバイダの設定を読み込み、デフォルト値と合わせて利用可能にする

#### Scenario: 設定ファイルが存在しない場合のデフォルト動作
- **WHEN** `config.toml` が存在しない状態で `subcli` を実行する
- **THEN** システムは全プロバイダを `auth = "subscription"`、`max_concurrency = 2` の安全なデフォルト設定として扱う

#### Scenario: 不正な認証モードまたは未知の複数アカウント定義
- **WHEN** `auth` に `subscription` / `api_key` 以外の値が指定されている、または単一プロバイダに複数アカウント設定が記述されている
- **THEN** システムは明確な設定エラーメッセージを出力して非ゼロ終了する

### Requirement: API キー生値の非保存と環境変数名参照
設定ファイルには API キーの実体ではなく環境変数名（`api_key_env`）のみを記述する設計とし、システムは認証トークンや API キーを設定ファイルへ書き出してはならない（MUST NOT）。

#### Scenario: api_key モードでの環境変数参照
- **WHEN** プロバイダが `auth = "api_key"` と `api_key_env = "MY_KEY"` で設定されている
- **THEN** システムは実行時に環境変数 `MY_KEY` の値を参照し、設定ファイル自体は読み取り専用として扱う

### Requirement: シークレットマスク付きログ出力
システムは診断・実行ログを `$XDG_STATE_HOME/subcli/subcli.log`（未設定時は標準状態ディレクトリ）へ記録しなければならない（SHALL）。ログには環境変数一覧を丸ごと出力してはならず（MUST NOT）、API キーに該当するパターン（`sk-`、`AIza` で始まる文字列等）は記録前にマスクしなければならない（MUST）。

#### Scenario: API キー様文字列のマスク処理
- **WHEN** エラー出力やログメッセージに `sk-...` または `AIza...` 形式の文字列が含まれる
- **THEN** ログファイルに書き込まれる前に当該トークン文字列がマスク文字列へ置換され、平文のキーは記録されない

#### Scenario: 環境変数の非ダンプ
- **WHEN** サブプロセスの起動やエラー診断をログに記録する
- **THEN** ログにはプロセス環境変数全体が出力されない
