# Design

## Context

Phase 0 の実機契約・fixture と Phase 1 の CLI 骨格・`policy`・`config`・ログ基盤の上に、`claude` と `codex` のヘッドレス実行・セッション管理を構築する。モチベーションは `proposal.md`、要求仕様は `specs/` 配下の 5 ファイルを参照。

## Goals / Non-Goals

**Goals:**
- `internal/event`, `internal/runner`, `internal/render`, `internal/session` の各基盤パッケージを責務分離して実装する。
- `claude` / `codex` アダプタを Phase 0 fixture に完全準拠させて実装し、未知イベント耐性・`ChildEnv` 分離・危険フラグ禁止をユニットテストで検証する。
- `testdata/bin/` に偽 CLI を置き、CI 上で認証なしにストリーミング・タイムアウト・シグナル転送・セッション往復を検証可能にする。

**Non-Goals:**
- `compare` / `chain` コマンド（Phase 3）。
- `agy` の API キーモード・`internal/geminiapi`・PTY 実行（Phase 4）。
- セッション DB への会話本文フルログ保存や認証情報の保存。

## Decisions

### 1. ストリーム読み取りに `bufio.Reader.ReadBytes('\n')` を採用
- **決定**: `internal/runner` の stdout ループでは `bufio.Scanner` を使わず、`bufio.NewReader(stdout).ReadBytes('\n')` を用いる。
- **理由**: `bufio.Scanner` はデフォルト 64KB 制限があり、ファイル差分やツール結果を含む長大な NDJSON 行で `bufio.ErrTooLong` を起こして停止するため（`spce.md` §8.1）。
- **代替案**: `Scanner.Buffer` の上限引き上げは依然として固定上限が残るため却下。

### 2. プロセスグループ単位の段階的シグナル転送（`Setpgid`）
- **決定**: Unix では `SysProcAttr{Setpgid: true}` で子プロセスグループを作成し、親コンテキストのキャンセル（Ctrl+C や `--timeout`）時に `-pgid` へ `SIGINT` → 5 秒後 `SIGTERM` → さらに 5 秒後 `SIGKILL` を送信する。stderr は 64KB 固定長のリングバッファ構造体で保持する。
- **理由**: 公式 CLI が起動した孫プロセス（サブシェル・テストコマンド等）が孤児として残るのを防ぐため。Windows 向けは `cmd.Process.Kill()` 等のフォールバックをビルドタグで分離する。

### 3. SQLite ドライバとスキーマ設計（`modernc.org/sqlite`）
- **決定**: cgo 不要の `modernc.org/sqlite` を使用し、`$XDG_DATA_HOME/subcli/sessions.db` のディレクトリを `0700`、DB ファイルを `0600` で作成・維持する。
- **スキーマ**:
  ```sql
  CREATE TABLE IF NOT EXISTS sessions (
      id TEXT PRIMARY KEY,
      provider TEXT NOT NULL,
      provider_session_id TEXT NOT NULL,
      cwd TEXT NOT NULL,
      created_at TEXT NOT NULL,
      last_run_at TEXT NOT NULL,
      prompt_preview TEXT NOT NULL
  );
  ```
- **ローカル ID 形式**: 短い衝突しにくい ID（例: 8〜12 文字のランダム英数字またはタイムスタンプ+乱数）を採用し、人間が `subcli resume <id>` で扱いやすくする。
- **プロンプト先頭 80 文字**: ルーン（Unicode 文字）単位で 80 文字に丸め、マルチバイト日本語文字列が途中で壊れないようにする。さらに保存前に API キー様文字列のマスクを適用する。

### 4. `claude` / `codex` パーサのイベント正規化と未知型フォールバック
- **決定**: 両アダプタの `Parse(line []byte) ([]event.Event, error)` は 1 行から 0 個以上のイベントを返す設計とする（`type=result` や `turn.completed` のように 1 行から `Usage` + `Completed` の 2 イベントを生成するケース、および `type=assistant` の `content` 配列に複数ブロックが含まれるケースに対応するため）。
- JSON 構造の不一致や未知の `type` はエラーにせず `event.Unknown`（`Raw: line`）として返す。JSON としてそもそも壊れた行（非 JSON のノイズ出力）はエラーにせず警告ログ＋`Unknown`（または JSON 文字列としてラップした `Raw`）にする方針を Phase 0 fixture で確認した挙動に合わせて確定する。

### 5. `render` パッケージの責務分離
- **決定**: `Renderer` インターフェースを `TTYRenderer` と `JSONLRenderer` に分離する。`TTYRenderer` はプロバイダ別の ANSI カラー（非 TTY 出力時はカラー無効化）、`Text` の逐次出力、`ToolCall` の 1 行要約、`Usage` 出力時の `auth == subscription` 判定による「参考値（実際の請求ではありません）」注記を担当する。

## Risks / Trade-offs

- [公式 CLI のバージョンアップで NDJSON スキーマが変更される] → 未知フィールド・未知型を `Unknown` として透過させ、`Raw` を保持することで即時破綻を防ぐ。
- [`codex exec resume` の引数順序がバージョンで変わり得る] → Phase 0 で採取した `docs/cli-contracts/codex-<version>.txt` に厳密に合わせ、ユニットテストで引数列を固定検証する。
- [64KB リングバッファで非常に長いエラーメッセージの先頭が落ちる] → 最新のエラー末尾が原因究明に最も重要であり、メモリ枯渇防止とのバランスとして仕様（§8.1）どおり 64KB 末尾保持を採用する。
