# subcli Compliance & Terms Summary

Confirmed-Date: 2026-11-22

> **注意**: 本書は各社の公開ドキュメントおよび公式フォーラムでの回答をもとに整理した設計上の遵守方針であり、法的助言ではありません。各社の方針は随時更新されるため、月 1 回または各公式 CLI のメジャー更新時に必ず参照 URL を再確認し、本書の `Confirmed-Date` と `internal/policy/matrix.go` を更新してください。

---

## 1. 基本原則

1. **公式 CLI ランチャー兼ビューアに徹する**: 各社のモデル API やバックエンドを直接呼ばない（例外は Gemini API キーの従量課金モードのみ）。
2. **認証情報に一切触れない**: 各公式 CLI の認証ファイル・キーチェーン・OAuth トークンを読まない、コピーしない、環境変数へ注入しない。ログインは公式 CLI の正規フローに完全に委ねる。
3. **個人・単一ユーザー・ローカル専用**: サーバ化、MCP/HTTP 公開、他人への提供、複数アカウントの自動ローテーションは実装しない。
4. **コードで規約ガードを強制する**: 禁止操作は実行前に `policy.Check()` で拒否し、静的テストで認証ファイルアクセス・危険フラグ・意図しない HTTP 通信を CI で防止する。

---

## 2. 各社の公式見解（要約・2026-11-22 再確認）

### 2.1 Anthropic（Claude Code）

- コンシューマー向けプラン（Free / Pro / Max 等）の OAuth 資格情報は、Claude.ai や公式 Claude Code CLI などのネイティブアプリケーションの通常利用を前提としている。
- 第三者開発者が Claude.ai ログインを独自アプリに組み込むこと、ユーザーに代わってプラン資格情報でリクエストを中継すること、資格情報やセッショントークンを収集・保存・仲介することは禁止されている。
- 一方で、エンドユーザーが**未改変の公式 Claude Code バイナリ**に自身のサブスクリプションで直接サインインして利用することは妨げられない。
- サードパーティ製ツール・カスタムハーネス・外部フレームワークがコンシューマー OAuth トークンを流用することは禁止されており、独自統合では API キー等の正規開発者チャネルが必要。
- `claude -p`（非対話モード）および Agent SDK とプラン利用枠の扱いについては最新サポート記事を継続監視する必要がある（2026-10-07 時点でプランログイン時はサブスク利用枠消費）。

### 2.2 OpenAI（Codex CLI）

- Codex CLI は「Sign in with ChatGPT（サブスクリプション）」と「OpenAI API キー（従量課金）」の双方を公式にサポートしている。
- `codex exec` は保存済みの CLI 認証を再利用でき、スクリプトや CI からの非対話実行が公式ドキュメントで案内されている。
- 自動化・CI 環境では API キー（`CODEX_API_KEY` / `OPENAI_API_KEY`）の利用が推奨既定値とされ、ChatGPT サブスク認証での自動運用は上級扱い。
- `~/.codex/auth.json` にはアクセストークンが含まれるため、パスワード同等に厳重に取り扱うよう公式に注意喚起されている（`subcli` はこのファイルを一切読まない）。

### 2.3 Google（Antigravity CLI）

- **Antigravity サブスクリプション（Google AI Pro / Ultra）**と **Google AI Studio API キー（従量課金）**は別個の利用枠・契約体系である。
- 公式 FAQ（2026-11-22 再確認）:
  - サードパーティ製ソフトウェアやツールから Antigravity ログインを利用してアクセスする行為は利用規約違反であり、アカウント停止・終了の対象となり得る。
  - `agy -p`（ヘッドレスモード）は直接のスクリプトおよび CI ワークフロー向けであり、サードパーティ製コーディングエージェントやアプリケーション内で `agy` をラップすること、アカウントローテーションを自動化することはコンシューマープランではサポート外。
  - サードパーティ製エージェント等から Gemini を利用する場合は、**Google AI Studio API キー**または **Gemini Enterprise** の利用が公式に推奨されている。
- 公式フォーラム回答（2026-10-19 / 2026-10-26）:
  - コンシューマープランのパススルーやパートナー OAuth は提供されていない。
  - 外部 AI が自律的に `agy` を起動し出力を読んで次の実行を指示するマルチエージェント自動ループやサードパーティアプリへの組み込みは不可。
- 既知の不具合（`google-antigravity/antigravity-cli#76`）: 非 TTY の stdout で `agy -p` の出力が欠落する問題が報告されている。

---

## 3. 許可マトリクス方針（`subcli` の実装境界）

`agy`（subscription）を自作 CLI からヘッドレス呼び出しする構成は「個人のスクリプト」と「`agy` ラッパーアプリ」のグレーゾーンに該当するため、**v1 では `agy`（subscription）は `login` / `open`（TTY かつ他 AI エージェント配下でない場合のみ）に厳格に限定する**。API キーモードでは Gemini バックエンドでの対話起動に対応する。ヘッドレス操作はアルファ版では未実装。

凡例: `✅` 許可（実装する） / `—` 未対応 / `❌` 拒否（`policy.Check()` で実行前エラー）

| 操作 | `claude` (subscription) | `codex` (subscription) | `agy` (subscription) | `agy` (api_key) |
| --- | :---: | :---: | :---: | :---: |
| `login`（公式ログイン起動） | ✅ | ✅ | ✅ ※1 | ❌ ※3 |
| `open`（対話 TUI へ `exec` 置換） | ✅ | ✅ | ✅ ※1 | ✅ |
| `run`（ヘッドレス単発） | ✅ | ✅ | ❌ ※2 | — |
| `resume`（セッション継続） | ✅ | ✅ | ❌ ※2 | — |
| `compare`（並列比較） | ✅ | ✅ | ❌ | — |
| `chain`（最大 3 段の直列連携・ループなし） | ✅ | ✅ | ❌ | — |
| 複数アカウントの自動切替 | ❌ | ❌ | ❌ | ❌ |
| サーバ / MCP / HTTP 外部公開 | ❌ | ❌ | ❌ | ❌ |

- **※1**: 標準入力・標準出力がともに TTY で、他の AI エージェント配下でないことを確認できた場合のみ許可。
- **※2**: v1 では未実装・コードで拒否。
- **※3**: API キーモードは公式ログインを使わないため拒否。

---

## 4. 参照 URL 一覧（2026-11-22 HTTP 200 および内容再確認済み）

### Anthropic
- Claude Code Legal and compliance（認証と資格情報の利用）:
  https://code.claude.com/docs/en/legal-and-compliance
- Agent SDK / `claude -p` とプランの利用枠:
  https://support.claude.com/en/articles/15036540

### OpenAI
- Codex Authentication:
  https://developers.openai.com/codex/auth
- Codex Non-interactive mode（`codex exec`）:
  https://developers.openai.com/codex/noninteractive

### Google
- Antigravity FAQ:
  https://antigravity.google/docs/faq
- Antigravity CLI Headless mode:
  https://antigravity.google/docs/cli/headless
- Antigravity Terms:
  https://antigravity.google/terms
- Google AI Developers Forum（サードパーティ拡張とコンシューマープラン、2026-10-19）:
  https://discuss.ai.google.dev/t/supported-gemini-subscription-access-for-a-third-party-vs-code-extension/189889/2
- Google AI Developers Forum（CI と外部 AI、Case A / Case B、2026-10-26）:
  https://discuss.ai.google.dev/t/consumer-plan-does-my-own-deterministic-ci-remain-permitted-if-its-test-input-is-prepared-by-another-ai/194998/2
- 既知の不具合（非 TTY での出力欠落）:
  https://github.com/google-antigravity/antigravity-cli/issues/76

---

## 5. 定期再確認手順

1. 月 1 回、または `claude` / `codex` / `agy` のメジャー更新時に第 4 節の全 URL を確認する。
2. 規約や方針に変更があった場合は、まず `internal/policy/matrix.go` とそのテストを更新し、その後各プロバイダアダプタへ反映する。
3. 本書先頭の `Confirmed-Date: YYYY-MM-DD` を更新する（`subcli doctor` はこの日付から 30 日以上経過した場合に警告を表示する）。
