# multi-provider-workflows Specification

## Purpose
複数の許可済みプロバイダに対して同一プロンプトを並列比較する `subcli compare` と、最大 3 段の非循環パイプラインで出力を次段へ渡す `subcli chain` の動作・制約・規約ガード要件を定義する。

## Requirements

### Requirement: 並列比較コマンド subcli compare
システムは `subcli compare --providers <p1,p2,...> "<prompt>"` コマンドを提供し、指定された複数プロバイダへ同一プロンプトを並列実行しなければならない（SHALL）。並列実行数はプロバイダ設定の `max_concurrency` を超えてはならず（MUST NOT）、結果は TTY では横並び表示、JSON 指定時は正規化イベントの JSONL として出力しなければならない（SHALL）。

#### Scenario: claude と codex の並列比較実行
- **WHEN** ユーザーが `subcli compare --providers claude,codex "要約して"` を実行する
- **THEN** 両プロバイダが事前ポリシー判定を通過したうえで並列実行され、結果がプロバイダ別に横並び（または JSONL）で表示される

#### Scenario: max_concurrency の遵守
- **WHEN** 同一プロバイダに対する同時実行要求が `max_concurrency` を上回る条件で比較を実行する
- **THEN** 当該プロバイダへの同時起動プロセス数は常に `max_concurrency` 以下に制限される

### Requirement: 直列連携コマンド subcli chain
システムは `subcli chain --steps <p1,p2[,p3]> --prompt "<最初の依頼>" --then "<テンプレート>"` コマンドを提供しなければならない（SHALL）。前段プロバイダの最終テキスト出力をテンプレート変数 `{{.Prev}}` へ埋め込んで次段プロバイダのプロンプトを生成し、順次実行しなければならない（SHALL）。段数は 2 段以上かつ最大 3 段までに制限し、出力内容に応じて自動的に次の実行や反復を決定するループ機構を実装してはならない（MUST NOT）。

#### Scenario: 2 段の直列連携実行
- **WHEN** ユーザーが `subcli chain --steps claude,codex --prompt "実装案を出して" --then "次の案をレビューして: {{.Prev}}"` を実行する
- **THEN** まず `claude` が実行され、その最終テキストが `{{.Prev}}` に展開されたプロンプトで `codex` が 1 回だけ実行される

#### Scenario: 3 段を超えるステップ指定の拒否
- **WHEN** ユーザーが 4 段以上（または 1 段以下）の `--steps` を指定して `subcli chain` を実行する
- **THEN** システムは 1 プロセスも起動せず、段数制限（2〜3 段）エラーを表示して非ゼロ終了する

#### Scenario: 前段失敗時の中断
- **WHEN** `chain` の途中の段でプロバイダ実行がエラー終了する
- **THEN** システムは後続の段を起動せず、エラー詳細と非ゼロ終了コードを返して終了する

### Requirement: 複数プロバイダ操作における事前一括ポリシー拒否
`subcli compare` および `subcli chain` は、実行開始前に参加する全プロバイダに対して `policy.Check` を実行しなければならない（SHALL）。参加プロバイダのうち 1 つでも拒否される場合（例: `agy` の `subscription` モードが含まれる場合）、システムは他の許可プロバイダを含めて一切プロセスを起動せず、全体の実行を拒否しなければならない（MUST）。

#### Scenario: agy サブスクを含む compare の事前全体拒否
- **WHEN** `agy` が `auth=subscription` の状態で `subcli compare --providers claude,agy "比較して"` を実行する
- **THEN** `claude` も `agy` も起動されず、`agy` サブスクが `compare` で禁止されている理由と公式参照 URL を表示して非ゼロ終了する

#### Scenario: agy サブスクを含む chain の事前全体拒否
- **WHEN** `agy` が `auth=subscription` の状態で `subcli chain --steps claude,agy --prompt "案を出して" --then "確認: {{.Prev}}"` を実行する
- **THEN** `claude` も `agy` も起動されず、`agy` サブスクが `chain` で禁止されている理由と公式参照 URL を表示して非ゼロ終了する
