package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sudabon/subcli/docs"
	"github.com/sudabon/subcli/internal/childcmd"
	"github.com/sudabon/subcli/internal/config"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/policy"
	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/agy"
	"github.com/sudabon/subcli/internal/provider/claude"
	"github.com/sudabon/subcli/internal/provider/codex"
	"github.com/sudabon/subcli/internal/runner"
	"github.com/sudabon/subcli/internal/session"
)

// ExitError carries an explicit process exit status without calling os.Exit directly.
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// Deps bundles external dependencies so commands can be tested without real CLIs or credentials.
type Deps struct {
	LoadConfig     func() (config.Config, error)
	DetectContext  func() policy.CallContext
	LookPath       func(file string) (string, error)
	ExecReplace    func(binPath string, argv []string, env []string) error
	RunInteractive func(ctx context.Context, binPath string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
	RunCapture     func(ctx context.Context, binPath string, args []string, env []string) (string, error)
	Environ        func() []string
	Getwd          func() (string, error)
	OpenStore      func() (*session.Store, error)
	Now            func() time.Time
	ComplianceText func() (string, string)
	TermWidth      func() int
	Logger         *logging.Logger
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
}

// DefaultDeps returns production CLI dependencies.
func DefaultDeps() Deps {
	logger, err := logging.OpenDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: ログファイルを開けませんでした: %v\n", err)
	}
	return Deps{
		LoadConfig:     config.Load,
		DetectContext:  policy.DetectCallContext,
		LookPath:       exec.LookPath,
		ExecReplace:    execInteractive,
		RunInteractive: defaultRunInteractive,
		RunCapture:     defaultRunCapture,
		Environ:        os.Environ,
		Getwd:          os.Getwd,
		OpenStore:      session.OpenDefault,
		Now:            time.Now,
		ComplianceText: defaultComplianceText,
		Logger:         logger,
		Stdin:          os.Stdin,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
	}
}

func (d *Deps) normalize() {
	if d.LoadConfig == nil {
		d.LoadConfig = config.Load
	}
	if d.DetectContext == nil {
		d.DetectContext = policy.DetectCallContext
	}
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	if d.ExecReplace == nil {
		d.ExecReplace = execInteractive
	}
	if d.RunInteractive == nil {
		d.RunInteractive = defaultRunInteractive
	}
	if d.RunCapture == nil {
		d.RunCapture = defaultRunCapture
	}
	if d.Environ == nil {
		d.Environ = os.Environ
	}
	if d.Getwd == nil {
		d.Getwd = os.Getwd
	}
	if d.OpenStore == nil {
		d.OpenStore = session.OpenDefault
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.ComplianceText == nil {
		d.ComplianceText = defaultComplianceText
	}
	if d.Stdin == nil {
		d.Stdin = os.Stdin
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
}

// Execute runs the root subcli command using default production dependencies.
func Execute() error {
	deps := DefaultDeps()
	if deps.Logger != nil {
		defer deps.Logger.Close()
	}
	return NewRootCmd(deps).Execute()
}

// NewRootCmd constructs the cobra command tree with injected dependencies.
func NewRootCmd(deps Deps) *cobra.Command {
	deps.normalize()

	root := &cobra.Command{
		Use:           "subcli",
		Short:         "利用規約の範囲内で公式 CLI を安全に起動する個人用ローカルランチャー",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)

	root.AddCommand(
		newDoctorCmd(&deps),
		newLoginCmd(&deps),
		newOpenCmd(&deps),
		newMemoryCmd(&deps),
		newRunCmd(&deps),
		newResumeCmd(&deps),
		newSessionsCmd(&deps),
		newCompareCmd(&deps),
		newChainCmd(&deps),
	)
	return root
}

func buildRegistry(cfg config.Config) *provider.Registry {
	return provider.NewRegistry(
		claude.New(cfg.Providers.Claude.APIKeyEnv),
		codex.New(cfg.Providers.Codex.APIKeyEnv),
		agy.New(cfg.Providers.Agy.APIKeyEnv),
	)
}

func defaultRunInteractive(ctx context.Context, binPath string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd, err := childcmd.CommandContext(ctx, binPath, args...)
	if err != nil {
		return 1, err
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = env
	return runForeground(cmd)
}

// runForeground runs an interactive child that shares subcli's terminal. The
// terminal delivers Ctrl+C to the child directly, so subcli ignores it while
// waiting instead of exiting and leaving the child behind, then propagates the
// child's exit status.
func runForeground(cmd *exec.Cmd) (int, error) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return runner.ExitCodeFromError(err), nil
	}
	return 1, err
}

func defaultRunCapture(ctx context.Context, binPath string, args []string, env []string) (string, error) {
	cmd, err := childcmd.CommandContext(ctx, binPath, args...)
	if err != nil {
		return "", err
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func defaultComplianceText() (string, string) {
	return docs.ComplianceMarkdown, "embedded docs/COMPLIANCE.md"
}

func validateAPIKey(pc config.ProviderConfig, env []string) error {
	if pc.Auth == provider.AuthAPIKey {
		value, _ := provider.LookupEnvSlice(env, pc.APIKeyEnv)
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("API キーモードには環境変数 %s の設定が必要です", pc.APIKeyEnv)
		}
	}
	return nil
}

func resolveWorkDir(deps *Deps, cwd string) (string, error) {
	if !filepath.IsAbs(cwd) {
		base, err := deps.Getwd()
		if err != nil {
			return "", fmt.Errorf("カレントディレクトリの取得に失敗しました: %w", err)
		}
		cwd = filepath.Join(base, cwd)
	}
	return filepath.Abs(cwd)
}

func writePolicyDenied(deps *Deps, prov string, op policy.Op, dec policy.Decision) error {
	fmt.Fprintf(deps.Stderr, "拒否されました (%s %s): %s\n", op, prov, dec.Reason)
	if dec.DocURL != "" {
		fmt.Fprintf(deps.Stderr, "参照 URL: %s\n", dec.DocURL)
	}
	deps.Logger.Warn("policy denied",
		logging.Field{Key: "provider", Value: prov},
		logging.Field{Key: "op", Value: string(op)},
		logging.Field{Key: "reason", Value: dec.Reason},
		logging.Field{Key: "doc_url", Value: dec.DocURL},
	)
	return &ExitError{Code: 1, Silent: true}
}
