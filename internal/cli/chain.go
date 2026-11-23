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

func newChainCmd(deps *Deps) *cobra.Command {
	var (
		stepsRaw      string
		initialPrompt string
		thenTemplates []string
		model         string
		cwd           string
		permStr       string
		jsonOut       bool
		timeout       time.Duration
	)

	cmd := &cobra.Command{
		Use:   "chain --steps <p1,p2[,p3]> --prompt \"<最初の依頼>\" --then \"<テンプレート>\"",
		Short: "前段の出力を {{.Prev}} に展開して次段へ渡す 2〜3 段の非循環直列実行を行う",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stepIDs, err := parseProviderList("--steps", stepsRaw)
			if err != nil {
				return err
			}
			if err := runner.ValidateChainStepCount(len(stepIDs)); err != nil {
				return err
			}
			if strings.TrimSpace(initialPrompt) == "" {
				return fmt.Errorf("--prompt を指定してください")
			}
			if _, err := runner.CompileChainTemplates(len(stepIDs), thenTemplates); err != nil {
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

			type resolvedStep struct {
				prov provider.Provider
				pcfg config.ProviderConfig
			}
			resolved := make([]resolvedStep, 0, len(stepIDs))
			for _, id := range stepIDs {
				pcfg, err := cfg.Provider(id)
				if err != nil {
					return err
				}
				prov, ok := reg.Get(id)
				if !ok {
					return fmt.Errorf("未対応のプロバイダです: %q", id)
				}
				resolved = append(resolved, resolvedStep{
					prov: prov,
					pcfg: pcfg,
				})
			}

			// Batch policy check across ALL step participants before looking up binaries or launching any process.
			callCtx := deps.DetectContext()
			for _, s := range resolved {
				dec := policy.Check(s.prov.ID(), s.pcfg.Auth, policy.OpChain, callCtx)
				if !dec.Allow {
					return writePolicyDenied(deps, s.prov.ID(), policy.OpChain, dec)
				}
			}

			workDir, err := resolveWorkDir(deps, cwd)
			if err != nil {
				return err
			}

			baseEnv := deps.Environ()
			chainSteps := make([]runner.ChainStep, 0, len(resolved))
			for _, s := range resolved {
				if err := validateAPIKey(s.pcfg, baseEnv); err != nil {
					return err
				}
				req := provider.RunRequest{
					Prompt: "validate",
					Model:  model,
					Cwd:    workDir,
					Perm:   perm,
				}
				if _, err := s.prov.HeadlessArgv(req); err != nil {
					if errors.Is(err, provider.ErrUnsupported) {
						return fmt.Errorf("%s はヘッドレス実行に対応していません", s.prov.ID())
					}
					return err
				}
				binPath, err := deps.LookPath(s.prov.Binary())
				if err != nil {
					return fmt.Errorf("%s CLI (%s) が見つかりません: %w", s.prov.ID(), s.prov.Binary(), err)
				}
				req.Prompt = ""
				chainSteps = append(chainSteps, runner.ChainStep{
					Provider: s.prov,
					BinPath:  binPath,
					Request:  req,
					Auth:     s.pcfg.Auth,
					BaseEnv:  baseEnv,
				})
			}

			sigCtx, stopSignals := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stopSignals()

			runCtx, cancelTimeout := context.WithTimeout(sigCtx, timeout)
			defer cancelTimeout()
			output := renderStatus{cancel: cancelTimeout}

			isTTY := render.IsTerminal(deps.Stdout)
			var jsonRenderer *render.ChainJSONLRenderer
			var ttyRenderer *render.TTYRenderer
			if jsonOut {
				jsonRenderer = render.NewChainJSONLRenderer(deps.Stdout)
			}

			_, runErr := runner.RunChain(runCtx, chainSteps, initialPrompt, thenTemplates, runner.ChainOptions{
				Logger: deps.Logger,
				OnStepStart: func(stepIndex, totalSteps int, provID string, auth provider.AuthMode, _ string) {
					if !jsonOut {
						output.check(render.WriteChainStepHeader(deps.Stdout, stepIndex, totalSteps, provID, isTTY))
						ttyRenderer = render.NewTTYRenderer(deps.Stdout, auth, isTTY)
					}
				},
				OnEvent: func(stepIndex int, ev event.Event) {
					if jsonOut {
						output.check(jsonRenderer.Render(stepIndex+1, ev))
						return
					}
					if ttyRenderer != nil {
						output.check(ttyRenderer.Render(ev))
					}
				},
				OnStepComplete: func(_, _ int, _ runner.ChainStepResult) {
					if ttyRenderer != nil {
						output.check(ttyRenderer.Flush())
					}
				},
			})

			if output.err != nil {
				return output.err
			}
			return finalizeChainExit(deps, runErr)
		},
	}

	cmd.Flags().StringVar(&stepsRaw, "steps", "", "実行順プロバイダのカンマ区切りリスト（2〜3 段、例: claude,codex）")
	cmd.Flags().StringVar(&initialPrompt, "prompt", "", "1 段目に渡す最初の依頼")
	cmd.Flags().StringArrayVar(&thenTemplates, "then", nil, "次段へ渡すテンプレート（{{.Prev}} に前段の出力を展開、1 個または段数-1 個指定可）")
	cmd.Flags().StringVar(&model, "model", "", "使用するモデル名")
	cmd.Flags().StringVar(&cwd, "cwd", "", "作業ディレクトリ（既定: カレントディレクトリ）")
	cmd.Flags().StringVar(&permStr, "perm", string(provider.PermReadOnly), "権限モード (read-only|edit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "正規化イベントを JSONL 形式で標準出力へ出力")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultRunTimeout, "実行タイムアウト")
	return cmd
}

func finalizeChainExit(deps *Deps, err error) error {
	if err == nil {
		return nil
	}
	var chainErr *runner.ChainError
	if !errors.As(err, &chainErr) {
		return err
	}
	if chainErr.Result.Stderr != "" {
		fmt.Fprint(deps.Stderr, chainErr.Result.Stderr)
		if !strings.HasSuffix(chainErr.Result.Stderr, "\n") {
			fmt.Fprintln(deps.Stderr)
		}
	}
	if chainErr.Result.TimedOut {
		fmt.Fprintf(deps.Stderr, "ステップ %d (%s) の実行がタイムアウトしました。\n", chainErr.StepIndex+1, chainErr.Provider)
		code := chainErr.Result.ExitCode
		if code == 0 {
			code = 124
		}
		return &ExitError{Code: code, Silent: true}
	}
	if chainErr.Result.Canceled {
		code := chainErr.Result.ExitCode
		if code == 0 {
			code = 130
		}
		return &ExitError{Code: code, Silent: true}
	}
	fmt.Fprintf(deps.Stderr, "%s\n", chainErr.Error())
	code := chainErr.Result.ExitCode
	if code == 0 {
		code = 1
	}
	return &ExitError{Code: code, Silent: true}
}
