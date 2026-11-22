# session-store Specification

## Purpose
ヘッドレス実行したセッションのメタデータをローカル SQLite データベースに安全に永続化し、`subcli resume` での継続および `subcli sessions` での履歴閲覧・削除を可能にする要件を定義する。

## Requirements

### Requirement: ローカルセッションデータベースの安全な保存
システムはセッション履歴を `$XDG_DATA_HOME/subcli/sessions.db`（未設定時は標準データディレクトリ）へパーミッション `0600` で保存しなければならない（SHALL）。保存する項目は、ローカル ID、プロバイダ ID、プロバイダ側セッション ID、作業ディレクトリ（`cwd`）、作成日時、最終実行日時、および最初のプロンプトの先頭 80 文字のみとし、認証情報やトークン・API キーを保存してはならない（MUST NOT）。

#### Scenario: 新規ヘッドレス実行時のセッション記録
- **WHEN** `--no-store` を指定せずにヘッドレス実行し `session_started` イベントからプロバイダ側セッション ID を取得する
- **THEN** `sessions.db` がパーミッション `0600` で作成・更新され、ローカル ID・プロバイダ・プロバイダ側セッション ID・cwd・作成日時・最終実行日時・プロンプト先頭 80 文字が保存される

#### Scenario: --no-store 指定時の非保存
- **WHEN** ユーザーが `--no-store` を指定して `subcli run` を実行する
- **THEN** セッションストアへ新規レコードは書き込まれない

### Requirement: セッション管理コマンド subcli sessions
システムは `subcli sessions list`、`subcli sessions show <local-id>`、および `subcli sessions rm <local-id>` を提供し、ローカルに保存されたセッション履歴の一覧表示・詳細表示・削除を行えなければならない（SHALL）。サブコマンド省略時は `list` と同等に振る舞わなければならない（SHALL）。

#### Scenario: セッション一覧・詳細表示・削除の操作
- **WHEN** ユーザーが `subcli sessions list`、`subcli sessions show <id>`、`subcli sessions rm <id>` を順次実行する
- **THEN** `list` で保存済みセッション一覧が表示され、`show` で対象レコードのメタデータ詳細が表示され、`rm` で対象レコードが削除される

#### Scenario: 存在しないセッション ID の指定
- **WHEN** 存在しない `<id>` に対して `subcli sessions show` または `subcli sessions rm` を実行する
- **THEN** 対象セッションが見つからない旨のエラーメッセージを表示し、非ゼロ終了コードで終了する
