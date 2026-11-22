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
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/render"
	"github.com/sudabon/subcli/internal/runner"
	"github.com/sudabon/subcli/internal/session"
)

const defaultRunTimeout = 30 * time.Minute

func newRunCmd(deps *Deps) *cobra.Command {
	var (
		model   string
		cwd     string
		permStr string
		jsonOut bool
		noStore bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run <provider> [flags] \"<prompt>\"",
		Short: "指定プロバイダをヘッドレスで単発実行する",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provID := args[0]
			prompt := args[1]

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
			pcfg, err := cfg.Provider(provID)
			if err != nil {
				return err
			}
			reg := buildRegistry(cfg)
			prov, ok := reg.Get(provID)
			if !ok {
				return fmt.Errorf("未対応のプロバイダです: %q", provID)
			}

			dec := policy.Check(prov.ID(), pcfg.Auth, policy.OpRun, deps.DetectContext())
			if !dec.Allow {
				return writePolicyDenied(deps, prov.ID(), policy.OpRun, dec)
			}

			workDir := strings.TrimSpace(cwd)
			if workDir == "" {
				workDir, err = deps.Getwd()
				if err != nil {
					return fmt.Errorf("カレントディレクトリの取得に失敗しました: %w", err)
				}
			}

			req := provider.RunRequest{
				Prompt: prompt,
				Model:  model,
				Cwd:    workDir,
				Perm:   perm,
			}

			res, providerSessionID, runErr := executeHeadless(cmd.Context(), deps, prov, pcfg, req, headlessExecOpts{
				JSON:    jsonOut,
				Timeout: timeout,
			})

			if !noStore && providerSessionID != "" {
				store, storeErr := deps.OpenStore()
				if storeErr != nil {
					deps.Logger.Warn("failed to open session store", logging.Field{Key: "error", Value: storeErr.Error()})
					fmt.Fprintf(deps.Stderr, "警告: セッションストアを開けませんでした: %v\n", storeErr)
				} else {
					now := deps.Now().UTC()
					rec, createErr := store.Create(cmd.Context(), session.Record{
						Provider:          prov.ID(),
						ProviderSessionID: providerSessionID,
						Cwd:               workDir,
						CreatedAt:         now,
						LastRunAt:         now,
						PromptPreview:     prompt,
					})
					_ = store.Close()
					if createErr != nil {
						deps.Logger.Warn("failed to save session", logging.Field{Key: "error", Value: createErr.Error()})
						fmt.Fprintf(deps.Stderr, "警告: セッションの保存に失敗しました: %v\n", createErr)
					} else {
						fmt.Fprintf(deps.Stderr, "session: %s\n", rec.ID)
					}
				}
			}

			return finalizeHeadlessExit(deps, prov.ID(), res, runErr)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "使用するモデル名")
	cmd.Flags().StringVar(&cwd, "cwd", "", "作業ディレクトリ（既定: カレントディレクトリ）")
	cmd.Flags().StringVar(&permStr, "perm", string(provider.PermReadOnly), "権限モード (read-only|edit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "正規化イベントを JSONL 形式で標準出力へ出力")
	cmd.Flags().BoolVar(&noStore, "no-store", false, "セッションストアへ保存しない")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultRunTimeout, "実行タイムアウト")
	return cmd
}

type headlessExecOpts struct {
	JSON    bool
	Timeout time.Duration
}

func parsePermFlag(raw string) (provider.Perm, error) {
	switch provider.Perm(raw) {
	case provider.PermReadOnly:
		return provider.PermReadOnly, nil
	case provider.PermEdit:
		return provider.PermEdit, nil
	default:
		return "", fmt.Errorf("不正な --perm=%q です（%q または %q を指定してください）", raw, provider.PermReadOnly, provider.PermEdit)
	}
}

func executeHeadless(
	parentCtx context.Context,
	deps *Deps,
	prov provider.Provider,
	pcfg config.ProviderConfig,
	req provider.RunRequest,
	opts headlessExecOpts,
) (runner.Result, string, error) {
	// Validate arguments before resolving or launching the binary.
	if _, err := prov.HeadlessArgv(req); err != nil {
		return runner.Result{ExitCode: 1}, "", err
	}

	binPath, err := deps.LookPath(prov.Binary())
	if err != nil {
		return runner.Result{ExitCode: 1}, "", fmt.Errorf("%s CLI (%s) が見つかりません: %w", prov.ID(), prov.Binary(), err)
	}

	var renderer render.Renderer
	if opts.JSON {
		renderer = render.NewJSONLRenderer(deps.Stdout)
	} else {
		renderer = render.NewTTYRenderer(deps.Stdout, pcfg.Auth, render.IsTerminal(deps.Stdout))
	}

	sigCtx, stopSignals := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	runCtx, cancelTimeout := context.WithTimeout(sigCtx, opts.Timeout)
	defer cancelTimeout()

	var providerSessionID string
	res, runErr := runner.Run(runCtx, runner.Options{
		BinPath:  binPath,
		Provider: prov,
		Request:  req,
		Auth:     pcfg.Auth,
		BaseEnv:  deps.Environ(),
		Logger:   deps.Logger,
		OnEvent: func(ev event.Event) {
			if providerSessionID == "" && ev.SessionID != "" {
				providerSessionID = ev.SessionID
			}
			_ = renderer.Render(ev)
		},
	})
	_ = renderer.Flush()
	return res, providerSessionID, runErr
}

func finalizeHeadlessExit(deps *Deps, provID string, res runner.Result, runErr error) error {
	if res.Stderr != "" && (res.ExitCode != 0 || runErr != nil) {
		fmt.Fprint(deps.Stderr, res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			fmt.Fprintln(deps.Stderr)
		}
	}
	if res.TimedOut {
		fmt.Fprintf(deps.Stderr, "%s の実行がタイムアウトしました。\n", provID)
		code := res.ExitCode
		if code == 0 {
			code = 124
		}
		return &ExitError{Code: code, Silent: true}
	}
	if res.Canceled {
		code := res.ExitCode
		if code == 0 {
			code = 130
		}
		return &ExitError{Code: code, Silent: true}
	}
	if runErr != nil {
		if errors.Is(runErr, provider.ErrUnsupported) {
			return fmt.Errorf("%s はヘッドレス実行に対応していません", provID)
		}
		return runErr
	}
	if res.ExitCode != 0 {
		return &ExitError{Code: res.ExitCode, Silent: true}
	}
	return nil
}
