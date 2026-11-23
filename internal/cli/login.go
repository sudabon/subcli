package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
)

func newLoginCmd(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "login <provider>",
		Short: "対象プロバイダの公式ログインコマンドを TTY/stdio 継承で起動する",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd.Context(), deps, args[0])
		},
	}
}

func runLogin(ctx context.Context, deps *Deps, providerID string) error {
	cfg, err := deps.LoadConfig()
	if err != nil {
		return err
	}
	pc, err := cfg.Provider(providerID)
	if err != nil {
		return err
	}
	reg := buildRegistry(cfg)
	p, ok := reg.Get(providerID)
	if !ok {
		return fmt.Errorf("未対応のプロバイダです: %q", providerID)
	}

	cc := deps.DetectContext()
	dec := policy.Check(p.ID(), pc.Auth, policy.OpLogin, cc)
	if !dec.Allow {
		return writePolicyDenied(deps, p.ID(), policy.OpLogin, dec)
	}

	binPath, err := deps.LookPath(p.Binary())
	if err != nil {
		return fmt.Errorf("公式 CLI %q が PATH に見つかりません: %w", p.Binary(), err)
	}

	args := p.LoginArgv()
	env := p.ChildEnv(deps.Environ(), pc.Auth)
	deps.Logger.Info("launching official login",
		logging.Field{Key: "provider", Value: p.ID()},
		logging.Field{Key: "auth", Value: string(pc.Auth)},
	)

	exitCode, runErr := deps.RunInteractive(ctx, binPath, args, env, deps.Stdin, deps.Stdout, deps.Stderr)
	if runErr != nil {
		return fmt.Errorf("%s のログイン起動に失敗しました: %w", p.ID(), runErr)
	}

	fmt.Fprintf(deps.Stdout, "%s login exited with code %d\n", p.ID(), exitCode)
	deps.Logger.Info("official login finished",
		logging.Field{Key: "provider", Value: p.ID()},
		logging.Field{Key: "exit_code", Value: fmt.Sprintf("%d", exitCode)},
	)
	if exitCode != 0 {
		return &ExitError{Code: exitCode, Silent: true}
	}
	return nil
}
