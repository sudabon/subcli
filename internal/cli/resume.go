package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/session"
)

func newResumeCmd(deps *Deps) *cobra.Command {
	var (
		model   string
		permStr string
		jsonOut bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "resume <local-session-id> \"<prompt>\"",
		Short: "保存済みセッションを継続実行する",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localID := args[0]
			prompt := args[1]

			perm, err := parsePermFlag(permStr)
			if err != nil {
				return err
			}
			if timeout <= 0 {
				return fmt.Errorf("--timeout は正の時間を指定してください（現在: %v）", timeout)
			}

			store, err := deps.OpenStore()
			if err != nil {
				return err
			}
			defer store.Close()

			rec, err := store.Get(cmd.Context(), localID)
			if err != nil {
				if errors.Is(err, session.ErrNotFound) {
					return fmt.Errorf("セッション %q が見つかりません", localID)
				}
				return err
			}

			cfg, err := deps.LoadConfig()
			if err != nil {
				return err
			}
			pcfg, err := cfg.Provider(rec.Provider)
			if err != nil {
				return err
			}
			reg := buildRegistry(cfg)
			prov, ok := reg.Get(rec.Provider)
			if !ok {
				return fmt.Errorf("未対応のプロバイダです: %q", rec.Provider)
			}

			dec := policy.Check(prov.ID(), pcfg.Auth, policy.OpResume, deps.DetectContext())
			if !dec.Allow {
				return writePolicyDenied(deps, prov.ID(), policy.OpResume, dec)
			}

			req := provider.RunRequest{
				Prompt:            prompt,
				Model:             model,
				Cwd:               rec.Cwd,
				Perm:              perm,
				ProviderSessionID: rec.ProviderSessionID,
			}

			res, _, runErr := executeHeadless(cmd.Context(), deps, prov, pcfg, req, headlessExecOpts{
				JSON:    jsonOut,
				Timeout: timeout,
			})

			if res.Started {
				if touchErr := store.Touch(cmd.Context(), rec.ID, deps.Now().UTC()); touchErr != nil {
					deps.Logger.Warn("failed to update session last_run_at",
						logging.Field{Key: "id", Value: rec.ID},
						logging.Field{Key: "error", Value: touchErr.Error()},
					)
				}
			}

			return finalizeHeadlessExit(deps, prov.ID(), res, runErr)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "使用するモデル名")
	cmd.Flags().StringVar(&permStr, "perm", string(provider.PermReadOnly), "権限モード (read-only|edit)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "正規化イベントを JSONL 形式で標準出力へ出力")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultRunTimeout, "実行タイムアウト")
	return cmd
}
