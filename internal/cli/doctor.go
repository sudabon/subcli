package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/provider"
)

const complianceStaleDays = 30

func newDoctorCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "公式 CLI の検出・バージョン・ログイン状態および規約確認日を診断する",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), deps)
		},
	}
}

func runDoctor(ctx context.Context, deps *Deps) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return err
	}
	reg := buildRegistry(cfg)
	baseEnv := deps.Environ()

	fmt.Fprintln(deps.Stdout, "=== Providers ===")
	for _, p := range reg.List() {
		pc, _ := cfg.Provider(p.ID())
		if err := validateAPIKey(pc, baseEnv); err != nil {
			fmt.Fprintf(deps.Stdout, "- %s: WARNING: %v\n", p.ID(), err)
			continue
		}
		binPath, lookErr := deps.LookPath(p.Binary())
		if lookErr != nil {
			fmt.Fprintf(deps.Stdout, "- %s (%s): not found (auth=%s)\n", p.ID(), p.Binary(), pc.Auth)
			continue
		}

		childEnv := p.ChildEnv(baseEnv, pc.Auth)
		verCtx, cancelVer := context.WithTimeout(ctx, 5*time.Second)
		verOut, verErr := deps.RunCapture(verCtx, binPath, []string{"--version"}, childEnv)
		cancelVer()
		verSummary := firstNonEmptyLine(logging.Redact(verOut))
		if verErr != nil && verSummary == "" {
			verSummary = fmt.Sprintf("error: %v", verErr)
		}
		if verSummary == "" {
			verSummary = "unknown"
		}

		statusText := fmt.Sprintf("未確認（subcli login %s で確認してください）", p.ID())
		if sc, ok := p.(provider.StatusChecker); ok && len(sc.StatusArgv()) > 0 {
			stCtx, cancelSt := context.WithTimeout(ctx, 5*time.Second)
			stOut, stErr := deps.RunCapture(stCtx, binPath, sc.StatusArgv(), childEnv)
			cancelSt()
			cleanOut, loggedIn := summarizeStatusOutput(logging.Redact(stOut))
			switch {
			case loggedIn != nil && !*loggedIn:
				statusText = fmt.Sprintf("要確認 (%s)", cleanOut)
			case stErr == nil && cleanOut != "":
				statusText = cleanOut
			case stErr == nil:
				statusText = "ok"
			case cleanOut != "":
				statusText = fmt.Sprintf("要確認 (%s)", cleanOut)
			default:
				statusText = fmt.Sprintf("要確認 (%v)", stErr)
			}
		}

		fmt.Fprintf(deps.Stdout, "- %s (%s): found at %s\n", p.ID(), p.Binary(), binPath)
		fmt.Fprintf(deps.Stdout, "  auth:    %s\n", pc.Auth)
		fmt.Fprintf(deps.Stdout, "  version: %s\n", verSummary)
		fmt.Fprintf(deps.Stdout, "  login:   %s\n", statusText)
	}

	fmt.Fprintln(deps.Stdout, "\n=== Compliance ===")
	md, source := deps.ComplianceText()
	confirmedDate, parseErr := parseComplianceConfirmedDate(md)
	if parseErr != nil {
		fmt.Fprintf(deps.Stdout, "WARNING: 規約確認日を %s から読み取れませんでした (%v)\n", source, parseErr)
		deps.Logger.Warn("doctor compliance parse error", logging.Field{Key: "source", Value: source}, logging.Field{Key: "err", Value: parseErr.Error()})
		return nil
	}

	nowUTC := deps.Now().UTC()
	ageDays := int(nowUTC.Sub(confirmedDate.UTC()).Hours() / 24)
	fmt.Fprintf(deps.Stdout, "- source:         %s\n", source)
	fmt.Fprintf(deps.Stdout, "- confirmed_date: %s (%d 日前)\n", confirmedDate.Format("2006-01-02"), ageDays)
	if ageDays >= complianceStaleDays {
		fmt.Fprintf(deps.Stdout, "WARNING: docs/COMPLIANCE.md の規約確認日 (%s) から %d 日経過しています（%d 日以上）。各社の最新規約と参照 URL を再確認し、Confirmed-Date を更新してください。\n",
			confirmedDate.Format("2006-01-02"), ageDays, complianceStaleDays)
	}

	deps.Logger.Info("doctor completed", logging.Field{Key: "compliance_age_days", Value: fmt.Sprintf("%d", ageDays)})
	return nil
}

func parseComplianceConfirmedDate(markdown string) (time.Time, error) {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Confirmed-Date:") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "Confirmed-Date:"))
			t, err := time.Parse("2006-01-02", raw)
			if err != nil {
				return time.Time{}, fmt.Errorf("日付形式が不正です (%q): %w", raw, err)
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("Confirmed-Date 行が見つかりません")
}

type statusJSONPayload struct {
	LoggedIn         *bool  `json:"loggedIn"`
	AuthMethod       string `json:"authMethod"`
	SubscriptionType string `json:"subscriptionType"`
	APIProvider      string `json:"apiProvider"`
}

func summarizeStatusOutput(s string) (string, *bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", nil
	}

	if summary, loggedIn, ok := parseStatusJSON(trimmed); ok {
		return summary, loggedIn
	}

	if start := strings.IndexByte(trimmed, '{'); start >= 0 {
		if end := strings.LastIndexByte(trimmed, '}'); end > start {
			if summary, loggedIn, ok := parseStatusJSON(trimmed[start : end+1]); ok && loggedIn != nil {
				return summary, loggedIn
			}
		}
	}

	return firstNonEmptyLine(trimmed), nil
}

func parseStatusJSON(raw string) (string, *bool, bool) {
	if !strings.HasPrefix(raw, "{") {
		return "", nil, false
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return "", nil, false
	}

	var payload statusJSONPayload
	if err := json.Unmarshal([]byte(raw), &payload); err == nil && payload.LoggedIn != nil {
		if !*payload.LoggedIn {
			return "Not logged in", payload.LoggedIn, true
		}
		var details []string
		if m := strings.TrimSpace(payload.AuthMethod); m != "" {
			details = append(details, m)
		}
		if sub := strings.TrimSpace(payload.SubscriptionType); sub != "" {
			details = append(details, sub)
		}
		if len(details) == 0 {
			if ap := strings.TrimSpace(payload.APIProvider); ap != "" {
				details = append(details, ap)
			}
		}
		if len(details) > 0 {
			return fmt.Sprintf("Logged in (%s)", strings.Join(details, ", ")), payload.LoggedIn, true
		}
		return "Logged in", payload.LoggedIn, true
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err == nil {
		return compact.String(), nil, true
	}
	return firstNonEmptyLine(raw), nil, true
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
