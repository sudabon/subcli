package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/render"
	"github.com/sudabon/subcli/internal/runner"
)

func newCompareCmd(deps *Deps) *cobra.Command {
	var (
		providersRaw string
		model        string
		cwd          string
		permStr      string
		jsonOut      bool
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "compare --providers <p1,p2,...> [flags] \"<prompt>\"",
		Short: "同じプロンプトを複数プロバイダへ並列実行して結果を比較する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := args[0]

			providerIDs, err := parseProviderList("--providers", providersRaw)
			if err != nil {
				return err
			}
			perm, err := parsePermFlag(permStr)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				return fmt.Errorf("--timeout は正の時間を指定してください（現在: %v）", timeout)
			}

			cfg, err := deps.LoadConfig()
			if err != nil {
				return err
			}
			reg := buildRegistry(cfg)

			type resolvedParticipant struct {
				prov provider.Provider
				pcfg config.ProviderConfig
			}
			participants := make([]resolvedParticipant, 0, len(providerIDs))
			for _, id := range providerIDs {
				pcfg, err := cfg.Provider(id)
				if err != nil {
					return err
				}
				prov, ok := reg.Get(id)
				if !ok {
					return fmt.Errorf("未対応のプロバイダです: %q", id)
				}
				participants = append(participants, resolvedParticipant{
					prov: prov,
					pcfg: pcfg,
				})
			}

			// Batch policy check across ALL participants before looking up binaries or launching any process.
			callCtx := deps.DetectContext()
			for _, p := range participants {
				dec := policy.Check(p.prov.ID(), p.pcfg.Auth, policy.OpCompare, callCtx)
				if !dec.Allow {
					return writePolicyDenied(deps, p.prov.ID(), policy.OpCompare, dec)
				}
			}

			workDir, err := resolveWorkDir(deps, cwd)
			if err != nil {
				return err
			}

			baseEnv := deps.Environ()
			targets := make([]runner.CompareTarget, 0, len(participants))
			for _, p := range participants {
				if err := validateAPIKey(p.pcfg, baseEnv); err != nil {
					return err
				}
				req := provider.RunRequest{
					Prompt: prompt,
					Model:  model,
					Cwd:    workDir,
					Perm:   perm,
				}
				if _, err := p.prov.HeadlessArgv(req); err != nil {
					if errors.Is(err, provider.ErrUnsupported) {
						return fmt.Errorf("%s はヘッドレス実行に対応していません", p.prov.ID())
					}
					return err
				}
				binPath, err := deps.LookPath(p.prov.Binary())
				if err != nil {
					return fmt.Errorf("%s CLI (%s) が見つかりません: %w", p.prov.ID(), p.prov.Binary(), err)
				}
				targets = append(targets, runner.CompareTarget{
					Provider:       p.prov,
					BinPath:        binPath,
					Request:        req,
					Auth:           p.pcfg.Auth,
					MaxConcurrency: p.pcfg.MaxConcurrency,
					BaseEnv:        baseEnv,
				})
			}

			sigCtx, stopSignals := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stopSignals()

			runCtx, cancelTimeout := context.WithTimeout(sigCtx, timeout)
			defer cancelTimeout()
			output := renderStatus{cancel: cancelTimeout}

			var jsonRenderer *render.CompareJSONLRenderer
			var onEvent func(runner.CompareEvent)
			if jsonOut {
				jsonRenderer = render.NewCompareJSONLRenderer(deps.Stdout)
				onEvent = func(ce runner.CompareEvent) {
					output.check(jsonRenderer.Render(ce.Index, ce.Event))
				}
			}

			outcomes := runner.RunCompare(runCtx, targets, runner.CompareOptions{
				Logger:  deps.Logger,
				OnEvent: onEvent,
			})

			if output.err != nil {
				return output.err
			}

			if !jsonOut {
				cols := make([]render.CompareColumn, 0, len(outcomes))
				for _, out := range outcomes {
					cols = append(cols, buildCompareColumn(out))
				}
				width := 1 // Piped output uses stacked sections.
				if deps.TermWidth != nil {
					width = deps.TermWidth()
				} else if render.IsTerminal(deps.Stdout) {
					width = render.TerminalWidth(deps.Stdout)
				}
				if err := render.RenderCompare(deps.Stdout, cols, render.CompareFormatOptions{
					Width: width,
					Color: render.IsTerminal(deps.Stdout),
				}); err != nil {
					return err
				}
			}

			return finalizeCompareExit(deps, outcomes)
		},
	}

	cmd.Flags().StringVar(&providersRaw, "providers", "", "比較対象プロバイダのカンマ区切りリスト（例: claude,codex）")
	cmd.Flags().StringVar(&model, "model", "", "使用するモデル名")
	cmd.Flags().StringVar(&cwd, "cwd", "", "作業ディレクトリ（既定: カレントディレクトリ）")
	cmd.Flags().StringVar(&permStr, "perm", string(provider.PermReadOnly), "権限モード (read-only|edit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "正規化イベントを JSONL 形式で標準出力へ出力")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultRunTimeout, "実行タイムアウト")
	return cmd
}

func parseProviderList(flagName, raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("%s を指定してください（例: claude,codex）", flagName)
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		id := strings.TrimSpace(p)
		if id == "" {
			return nil, fmt.Errorf("%s に空のプロバイダ指定が含まれています: %q", flagName, raw)
		}
		out = append(out, id)
	}
	return out, nil
}

func buildCompareColumn(out runner.CompareOutcome) render.CompareColumn {
	var tools []string
	for _, ev := range out.Events {
		if ev.Kind == event.ToolCall {
			tools = append(tools, render.SummarizeToolEvent(ev))
		}
	}

	var errParts []string
	for _, msg := range out.ErrorMessages {
		if s := strings.TrimSpace(msg); s != "" {
			errParts = append(errParts, s)
		}
	}
	if out.Result.TimedOut {
		errParts = append(errParts, "タイムアウトしました")
	} else if out.Result.Canceled {
		errParts = append(errParts, "キャンセルされました")
	} else if out.Err != nil {
		errParts = append(errParts, out.Err.Error())
	}
	if out.Result.ExitCode != 0 && strings.TrimSpace(out.Result.Stderr) != "" {
		errParts = append(errParts, strings.TrimSpace(out.Result.Stderr))
	}

	return render.CompareColumn{
		Index:    out.Index,
		Provider: out.Provider,
		Auth:     out.Auth,
		Text:     out.Text,
		Tools:    tools,
		Usage:    out.Usage,
		ExitCode: out.Result.ExitCode,
		ErrText:  strings.Join(errParts, "\n"),
	}
}

func finalizeCompareExit(deps *Deps, outcomes []runner.CompareOutcome) error {
	exitCode := 0
	for _, out := range outcomes {
		failed := out.Err != nil || out.Result.ExitCode != 0
		if !failed {
			continue
		}
		if out.Result.Stderr != "" {
			fmt.Fprintf(deps.Stderr, "[%s] %s", out.Provider, out.Result.Stderr)
			if !strings.HasSuffix(out.Result.Stderr, "\n") {
				fmt.Fprintln(deps.Stderr)
			}
		}
		if out.Result.TimedOut {
			fmt.Fprintf(deps.Stderr, "[%s] 実行がタイムアウトしました。\n", out.Provider)
			if exitCode == 0 {
				if out.Result.ExitCode != 0 {
					exitCode = out.Result.ExitCode
				} else {
					exitCode = 124
				}
			}
			continue
		}
		if out.Result.Canceled {
			if exitCode == 0 {
				if out.Result.ExitCode != 0 {
					exitCode = out.Result.ExitCode
				} else {
					exitCode = 130
				}
			}
			continue
		}
		if out.Err != nil {
			fmt.Fprintf(deps.Stderr, "[%s] エラー: %v\n", out.Provider, out.Err)
			if exitCode == 0 {
				exitCode = 1
			}
			continue
		}
		if out.Result.ExitCode != 0 && exitCode == 0 {
			exitCode = out.Result.ExitCode
		}
	}

	if exitCode != 0 {
		return &ExitError{Code: exitCode, Silent: true}
	}
	return nil
}
