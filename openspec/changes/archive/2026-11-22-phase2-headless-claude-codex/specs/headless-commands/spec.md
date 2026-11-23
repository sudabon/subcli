# Spec Delta

## Purpose

ユーザーが単一プロバイダに対して統一的なフラグと出力形式でヘッドレス実行（`subcli run`）およびセッション継続（`subcli resume`）を行うためのコマンドインターフェースと描画仕様を定義する。

## ADDED Requirements

### Requirement: 単発ヘッドレス実行コマンド subcli run
システムは `subcli run <provider> [flags] "<prompt>"` を提供し、事前ポリシー判定（`OpRun`）を通過した場合に限り対象プロバイダをヘッドレス実行しなければならない（SHALL）。対応するフラグは以下のとおりとする。
- `--model <name>`: モデル指定
- `--cwd <dir>`: 作業ディレクトリ（既定: カレントディレクトリ）
- `--perm read-only|edit`: 権限（既定: `read-only`）
- `--json`: 正規化イベントを JSONL で標準出力へ出力
- `--no-store`: セッションストアへ保存しない
- `--timeout <dur>`: 実行タイムアウト（既定: `30m`）

#### Scenario: 既定オプションでの run 実行
- **WHEN** ユーザーが `subcli run claude "README を要約して"` を実行する
- **THEN** ポリシー判定（`OpRun`）を通過し、`--perm read-only`・カレントディレクトリ・タイムアウト 30 分で実行され、セッション情報がストアへ保存される

#### Scenario: 不正な --perm 値の拒否
- **WHEN** ユーザーが `read-only` と `edit` 以外の値（例: `full` や `bypass`）を `--perm` に指定する
- **THEN** 子プロセスを起動せず不正な権限指定エラーを返して非ゼロ終了する

### Requirement: セッション継続コマンド subcli resume
システムは `subcli resume <local-session-id> "<prompt>"` を提供し、セッションストアから対象レコードの `provider` とプロバイダ側セッション ID を取得して事前ポリシー判定（`OpResume`）を通過したうえで継続実行し、最終実行日時を更新しなければならない（SHALL）。

#### Scenario: 既存セッションの継続実行
- **WHEN** ユーザーが `subcli run` で保存された `<local-session-id>` を指定して `subcli resume <local-session-id> "続きを書いて"` を実行する
- **THEN** ストアから対応するプロバイダとプロバイダ側セッション ID が解決され、継続フラグ付きで実行され、最終実行日時が更新される

#### Scenario: 未登録のローカルセッション ID での resume 失敗
- **WHEN** ストアに存在しない `<local-session-id>` で `subcli resume` を実行する
- **THEN** 対象セッションが存在しない旨のエラーメッセージを表示して非ゼロ終了する

### Requirement: TTY ストリーミング表示と JSONL 出力レンダリング
システムは、通常（TTY）モードではプロバイダごとの色付きプレフィックス、テキストのストリーミング表示、ツール呼び出しの 1 行要約、および末尾の使用量（usage）表示を行わなければならない（SHALL）。`auth = "subscription"` でコスト値が表示される場合は「参考値（実際の請求ではありません）」と明記しなければならない（MUST）。`--json` 指定時は、全正規化イベントを 1 行 1 JSON の JSONL 形式で標準出力へ出力しなければならない（SHALL）。

#### Scenario: サブスクモードでの TTY 出力とコスト注記
- **WHEN** `auth = "subscription"` のプロバイダで `subcli run` を TTY モードで実行し、`usage` イベントに `cost_usd` が含まれる
- **THEN** プロバイダ色付きプレフィックス、ストリーミングテキスト、ツール 1 行要約に続き、コストが「参考値（実際の請求ではありません）」の注記付きで表示される

#### Scenario: --json フラグでの JSONL 出力
- **WHEN** ユーザーが `subcli run --json codex "テスト"` を実行する
- **THEN** 標準出力の各行が正規化イベントの有効な JSON オブジェクトとして出力される
