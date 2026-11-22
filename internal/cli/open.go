package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/memory"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
)

type openOpts struct {
	initMemory    bool
	noMemory      bool
	skipGitignore bool
}

func newOpenCmd(deps *Deps) *cobra.Command {
	var opts openOpts
	cmd := &cobra.Command{
		Use:   "open <provider> [-- <extra-args>...]",
		Short: "対象プロバイダの対話 TUI を起動する（Unix はプロセス置換）",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runOpen(deps, args[0], args[1:], opts)
		},
	}
	cmd.Flags().BoolVar(&opts.initMemory, "memory", false, "リポジトリ共有メモリ（.subcli/memory）が未初期化なら作成して読み込む")
	cmd.Flags().BoolVar(&opts.noMemory, "no-memory", false, "リポジトリ共有メモリを接続せずに起動する")
	cmd.Flags().BoolVar(&opts.skipGitignore, "no-gitignore", false, "--memory で初期化時に .gitignore へ .subcli/ を追記しない")
	return cmd
}

func runOpen(deps *Deps, providerID string, extra []string, opts openOpts) error {
	if opts.initMemory && opts.noMemory {
		return fmt.Errorf("--memory と --no-memory は同時に指定できません")
	}

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

	if err := provider.ValidateExtraArgs(extra); err != nil {
		return err
	}

	cc := deps.DetectContext()
	dec := policy.Check(p.ID(), pc.Auth, policy.OpOpen, cc)
	if !dec.Allow {
		return writePolicyDenied(deps, p.ID(), policy.OpOpen, dec)
	}

	binPath, err := deps.LookPath(p.Binary())
	if err != nil {
		return fmt.Errorf("公式 CLI %q が PATH に見つかりません: %w", p.Binary(), err)
	}

	extraArgs := p.InteractiveArgv(extra)
	env := p.ChildEnv(deps.Environ(), pc.Auth)

	var memArgs []string
	memoryAttached := false
	if !opts.noMemory {
		if memAdapter, supportsMem := p.(provider.MemoryAdapter); supportsMem {
			if cwd, wdErr := deps.Getwd(); wdErr == nil && cwd != "" {
				if store, openErr := memory.Open(cwd); openErr == nil {
					if !store.Exists() && opts.initMemory {
						initRes, initErr := store.Init(memory.InitOptions{SkipGitignore: opts.skipGitignore})
						if initErr != nil {
							return fmt.Errorf("共有メモリの初期化に失敗しました: %w", initErr)
						}
						if initRes.CreatedMemory {
							fmt.Fprintf(deps.Stderr, "共有メモリを初期化しました: %s\n", initRes.MemoryFile)
						}
						if initRes.UpdatedGitignore {
							fmt.Fprintf(deps.Stderr, ".gitignore に .subcli/ を追記しました\n")
						}
					}
					if store.Exists() {
						memCtx, ctxErr := store.BuildProviderContext(p.ID())
						if ctxErr != nil {
							return fmt.Errorf("共有メモリの同期に失敗しました: %w", ctxErr)
						}
						memArgs = memAdapter.InteractiveMemoryArgs(memCtx, extraArgs)
						env = memAdapter.InteractiveMemoryEnv(env, memCtx)
						memoryAttached = true
					}
				}
			}
		}
	}

	if err := provider.ValidateExtraArgs(memArgs); err != nil {
		return err
	}

	argv := make([]string, 0, 1+len(memArgs)+len(extraArgs))
	argv = append(argv, p.Binary())
	argv = append(argv, memArgs...)
	argv = append(argv, extraArgs...)

	deps.Logger.Info("opening interactive CLI",
		logging.Field{Key: "provider", Value: p.ID()},
		logging.Field{Key: "auth", Value: string(pc.Auth)},
		logging.Field{Key: "shared_memory", Value: fmt.Sprintf("%t", memoryAttached)},
	)
	return deps.ExecReplace(binPath, argv, env)
}
