package policy

import "github.com/sudabon/subcli/internal/provider"

const (
	urlAnthropicLegal   = "https://code.claude.com/docs/en/legal-and-compliance"
	urlOpenAIAuth       = "https://developers.openai.com/codex/auth"
	urlAntigravityFAQ   = "https://antigravity.google/docs/faq"
	urlAntigravityForum = "https://discuss.ai.google.dev/t/supported-gemini-subscription-access-for-a-third-party-vs-code-extension/189889/2"
)

type matrixKey struct {
	Provider string
	Auth     provider.AuthMode
	Op       Op
}

type matrixRule struct {
	Allow            bool
	RequireDirectTTY bool
	Reason           string
	DocURL           string
}

// policyMatrix encodes the permission matrix from the policy-guard specification as data.
var policyMatrix = map[matrixKey]matrixRule{
	// claude (subscription)
	{"claude", provider.AuthSubscription, OpLogin}:   {Allow: true},
	{"claude", provider.AuthSubscription, OpOpen}:    {Allow: true},
	{"claude", provider.AuthSubscription, OpRun}:     {Allow: true},
	{"claude", provider.AuthSubscription, OpResume}:  {Allow: true},
	{"claude", provider.AuthSubscription, OpCompare}: {Allow: true},
	{"claude", provider.AuthSubscription, OpChain}:   {Allow: true},

	// claude (api_key)
	{"claude", provider.AuthAPIKey, OpLogin}: {
		Allow:  false,
		Reason: "claude の API キーモード（auth=api_key）では公式サブスクリプションログインは不要です。環境変数で API キーを設定するか、auth=\"subscription\" に変更してください。",
		DocURL: urlAnthropicLegal,
	},
	{"claude", provider.AuthAPIKey, OpOpen}:    {Allow: true},
	{"claude", provider.AuthAPIKey, OpRun}:     {Allow: true},
	{"claude", provider.AuthAPIKey, OpResume}:  {Allow: true},
	{"claude", provider.AuthAPIKey, OpCompare}: {Allow: true},
	{"claude", provider.AuthAPIKey, OpChain}:   {Allow: true},

	// codex (subscription)
	{"codex", provider.AuthSubscription, OpLogin}:   {Allow: true},
	{"codex", provider.AuthSubscription, OpOpen}:    {Allow: true},
	{"codex", provider.AuthSubscription, OpRun}:     {Allow: true},
	{"codex", provider.AuthSubscription, OpResume}:  {Allow: true},
	{"codex", provider.AuthSubscription, OpCompare}: {Allow: true},
	{"codex", provider.AuthSubscription, OpChain}:   {Allow: true},

	// codex (api_key)
	{"codex", provider.AuthAPIKey, OpLogin}: {
		Allow:  false,
		Reason: "codex の API キーモード（auth=api_key）では公式サブスクリプションログインは不要です。環境変数で API キーを設定するか、auth=\"subscription\" に変更してください。",
		DocURL: urlOpenAIAuth,
	},
	{"codex", provider.AuthAPIKey, OpOpen}:    {Allow: true},
	{"codex", provider.AuthAPIKey, OpRun}:     {Allow: true},
	{"codex", provider.AuthAPIKey, OpResume}:  {Allow: true},
	{"codex", provider.AuthAPIKey, OpCompare}: {Allow: true},
	{"codex", provider.AuthAPIKey, OpChain}:   {Allow: true},

	// agy (subscription)
	{"agy", provider.AuthSubscription, OpLogin}: {
		Allow:            true,
		RequireDirectTTY: true,
		DocURL:           urlAntigravityFAQ,
	},
	{"agy", provider.AuthSubscription, OpOpen}: {
		Allow:            true,
		RequireDirectTTY: true,
		DocURL:           urlAntigravityFAQ,
	},
	{"agy", provider.AuthSubscription, OpRun}: {
		Allow:  false,
		Reason: "agy のサブスクリプション認証（Google AI Pro/Ultra）ではヘッドレス実行（run）は許可されていません。Google AI Studio の API キー（auth=\"api_key\"）を利用してください。",
		DocURL: urlAntigravityFAQ,
	},
	{"agy", provider.AuthSubscription, OpResume}: {
		Allow:  false,
		Reason: "agy のサブスクリプション認証（Google AI Pro/Ultra）ではセッション継続（resume）は許可されていません。Google AI Studio の API キー（auth=\"api_key\"）を利用してください。",
		DocURL: urlAntigravityFAQ,
	},
	{"agy", provider.AuthSubscription, OpCompare}: {
		Allow:  false,
		Reason: "agy のサブスクリプション認証（Google AI Pro/Ultra）は並列比較（compare）に参加できません。Google AI Studio の API キー（auth=\"api_key\"）を利用してください。",
		DocURL: urlAntigravityForum,
	},
	{"agy", provider.AuthSubscription, OpChain}: {
		Allow:  false,
		Reason: "agy のサブスクリプション認証（Google AI Pro/Ultra）は AI 間連携（chain）に参加できません。Google AI Studio の API キー（auth=\"api_key\"）を利用してください。",
		DocURL: urlAntigravityForum,
	},

	// agy (api_key)
	{"agy", provider.AuthAPIKey, OpLogin}: {
		Allow:  false,
		Reason: "agy の API キーモード（auth=api_key）では公式サブスクリプションログインは不要です。Google AI Studio の API キー環境変数を設定するか、auth=\"subscription\" に変更してください。",
		DocURL: urlAntigravityFAQ,
	},
	{"agy", provider.AuthAPIKey, OpOpen}:    {Allow: true},
	{"agy", provider.AuthAPIKey, OpRun}:     {Allow: true},
	{"agy", provider.AuthAPIKey, OpResume}:  {Allow: true},
	{"agy", provider.AuthAPIKey, OpCompare}: {Allow: true},
	{"agy", provider.AuthAPIKey, OpChain}:   {Allow: true},
}
