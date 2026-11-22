# Spec Delta

## Purpose

ユーザーが 1 つの CLI から各公式 CLI の状態診断・公式ログイン・対話起動を規約の範囲内で安全に行うための基本コマンド（`doctor`, `login`, `open`）および CI・ドキュメント要件を定義する。

## ADDED Requirements

### Requirement: 診断コマンド subcli doctor
システムは `subcli doctor` コマンドを提供し、各公式 CLI（`claude`, `codex`, `agy`）の検出有無およびバージョンを表示しなければならない（SHALL）。ログイン状態は公式のステータス確認コマンドが存在する場合のみそれを呼び出して表示し、存在しない場合は「未確認（`subcli login <provider>` で確認してください）」と表示しなければならない（MUST）。ログイン状態の判定のために各 CLI の認証ファイルやキーチェーンを読んではならない（MUST NOT）。さらに、`docs/COMPLIANCE.md` の規約確認日から 30 日以上経過している場合は警告を表示しなければならない（SHALL）。

#### Scenario: 公式 CLI のインストール状況とバージョン表示
- **WHEN** ユーザーが `subcli doctor` を実行する
- **THEN** `claude`, `codex`, `agy` ごとに実行ファイルの検出結果、バージョン、および公式ステータス確認結果（または未確認メッセージ）が表示される

#### Scenario: 規約確認日から 30 日以上経過している場合の警告
- **WHEN** `docs/COMPLIANCE.md` に記録された確認日が現在日付より 30 日以上前である状態で `subcli doctor` を実行する
- **THEN** 出力に規約再確認を促す警告が表示される

### Requirement: 公式ログイン起動コマンド subcli login
システムは `subcli login <provider>` コマンドを提供し、事前にポリシー判定を通過した場合に限り、対象プロバイダの公式ログインコマンドを標準入力・標準出力・標準エラー出力を継承して起動しなければならない（SHALL）。システムはログイン結果やトークンを読み取らず、終了コードのみを報告しなければならない（MUST）。

#### Scenario: 許可されたプロバイダでのログイン起動
- **WHEN** ユーザーがポリシー条件を満たす環境で `subcli login codex`（または `claude`, `agy`）を実行する
- **THEN** 対象公式 CLI のログインフローが TTY/stdio 継承で起動され、終了後にその終了コードが表示される

#### Scenario: ポリシー不許可時のログイン拒否
- **WHEN** 他の AI エージェント配下から `subcli login agy`（`auth=subscription`）を実行する
- **THEN** 公式 CLI は起動されず、拒否理由と参照 URL が表示されて非ゼロ終了する

### Requirement: 対話 TUI 起動コマンド subcli open
システムは `subcli open <provider> [-- <extra-args>...]` コマンドを提供し、事前にポリシー判定を通過した場合に限り、公式 CLI の対話セッションを起動しなければならない（SHALL）。Unix 環境ではプロセス置換により `subcli` 自身を公式 CLI プロセスへ置き換え入出力を仲介してはならず（MUST）、Windows 環境では stdio を継承した子プロセス起動で終了を待機しなければならない（SHALL）。

#### Scenario: Unix ターミナルからの直接対話起動
- **WHEN** Unix 環境のターミナル（TTY かつ他 AI エージェント配下でない）から `subcli open agy` または `subcli open claude -- --model ...` を実行する
- **THEN** ポリシー判定を通過し、`subcli` プロセスが対象公式 CLI のプロセスへ置き換わり、追加引数がそのまま渡される

#### Scenario: 他 AI エージェント配下からの agy 対話起動の拒否
- **WHEN** Claude Code 等の AI エージェントのシェル実行ツール配下から `subcli open agy`（`auth=subscription`）を実行する
- **THEN** ポリシー判定が操作を拒否し、`agy` は起動されず拒否理由と参照 URL を表示して非ゼロ終了する

### Requirement: オフライン CI と README の規約明記
本リポジトリは、実公式 CLI やサブスク認証を使用せずにユニットテストおよび静的遵守テストを実行する CI ワークフローを備えなければならない（SHALL）。また、`README.md` には「各公式 CLI で各自がログインすること」「`subcli` は認証情報に触れないこと」「個人利用のローカルツールであること」を明記し、製品名・コマンド名に各社ブランド名称を使用してはならない（MUST NOT）。

#### Scenario: CI の認証なしテスト実行
- **WHEN** CI パイプラインを実行する
- **THEN** 実 CLI バイナリおよびアカウント認証なしで全ユニットテストおよび静的遵守テストが完走する
