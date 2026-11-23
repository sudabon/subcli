package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sudabon/subcli/internal/childcmd"
	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/provider"
)

// DefaultGracePeriod is the wait between SIGINT -> SIGTERM -> SIGKILL escalations.
const DefaultGracePeriod = 5 * time.Second

// Options configures a single headless child process execution.
type Options struct {
	BinPath     string
	Provider    provider.Provider
	Request     provider.RunRequest
	Auth        provider.AuthMode
	BaseEnv     []string
	OnEvent     func(event.Event)
	Logger      *logging.Logger
	GracePeriod time.Duration
	StderrCap   int
}

// Result summarizes the outcome of a completed headless execution.
type Result struct {
	Started         bool
	ExitCode        int
	Stderr          string
	StderrTruncated bool
	TimedOut        bool
	Canceled        bool
}

// Run executes the provider CLI headlessly, streaming stdout line-by-line via
// bufio.Reader.ReadBytes('\n') (handling arbitrarily long lines), buffering the
// trailing 64KB of stderr, forwarding cancellation/timeouts across the child
// process group, and returning the child's exit code.
func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.Provider == nil {
		return Result{ExitCode: 1}, errors.New("runner: provider is required")
	}
	if err := ctx.Err(); err != nil {
		return Result{
			ExitCode: 1,
			TimedOut: errors.Is(err, context.DeadlineExceeded),
			Canceled: errors.Is(err, context.Canceled),
		}, err
	}

	argv, err := opts.Provider.HeadlessArgv(opts.Request)
	if err != nil {
		return Result{ExitCode: 1}, err
	}

	binPath := opts.BinPath
	if binPath == "" {
		binPath, err = exec.LookPath(opts.Provider.Binary())
		if err != nil {
			return Result{ExitCode: 1}, fmt.Errorf("%s CLI (%s) が見つかりません: %w", opts.Provider.ID(), opts.Provider.Binary(), err)
		}
	}

	baseEnv := opts.BaseEnv
	if baseEnv == nil {
		baseEnv = os.Environ()
	}

	gracePeriod := opts.GracePeriod
	if gracePeriod <= 0 {
		gracePeriod = DefaultGracePeriod
	}

	stderrCap := opts.StderrCap
	if stderrCap <= 0 {
		stderrCap = DefaultStderrCap
	}
	stderrBuf := NewRingBuffer(stderrCap)

	// exec.CommandContext is anchored on an internal context so our watcher controls
	// graceful SIGINT -> SIGTERM -> SIGKILL escalation across the child process group.
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancelExec()

	cmd, err := childcmd.CommandContext(execCtx, binPath, argv...)
	if err != nil {
		return Result{ExitCode: 1}, err
	}
	// Codex treats a lone dash as a request to read the prompt from stdin.
	if opts.Provider.ID() == "codex" && opts.Request.Prompt == "-" {
		cmd.Stdin = strings.NewReader(opts.Request.Prompt)
	}
	cmd.Env = opts.Provider.ChildEnv(baseEnv, opts.Auth)
	if opts.Request.Cwd != "" {
		cmd.Dir = opts.Request.Cwd
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{ExitCode: 1}, fmt.Errorf("stdout パイプの作成に失敗しました: %w", err)
	}
	defer stdoutPipe.Close()
	cmd.Stderr = stderrBuf

	group, err := startProcessGroup(cmd)
	if err != nil {
		return Result{ExitCode: 1}, fmt.Errorf("%s CLI の起動に失敗しました: %w", opts.Provider.ID(), err)
	}

	defer group.close()
	procDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			group.terminate(procDone, gracePeriod)
		case <-procDone:
		}
	}()

	var streamErr error
	terminalFailure := false
	reader := bufio.NewReader(stdoutPipe)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				events, parseErr := opts.Provider.Parse(trimmed)
				if parseErr != nil {
					// Ordinary diagnostic lines can coexist with JSON events. A malformed
					// structured record can conceal the result of a turn.
					if trimmed[0] == '{' && streamErr == nil {
						streamErr = fmt.Errorf("%s のイベントを解析できませんでした: %w", opts.Provider.ID(), parseErr)
					}
					opts.Logger.Warn("provider parse error",
						logging.Field{Key: "provider", Value: opts.Provider.ID()},
						logging.Field{Key: "error", Value: parseErr.Error()},
					)
					if len(events) == 0 {
						events = []event.Event{event.NewUnknown(opts.Provider.ID(), trimmed, time.Now())}
					}
				}
				for _, ev := range events {
					terminalFailure = terminalFailure || ev.TerminalError
					if opts.OnEvent != nil {
						opts.OnEvent(ev)
					}
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			// Pipe may close or error on signal termination; continue to Wait().
			break
		}
	}

	waitErr := cmd.Wait()
	close(procDone)
	<-watcherDone

	ctxErr := ctx.Err()
	res := Result{
		Started:         true,
		ExitCode:        ExitCodeFromError(waitErr),
		Stderr:          stderrBuf.String(),
		StderrTruncated: stderrBuf.Truncated(),
		TimedOut:        errors.Is(ctxErr, context.DeadlineExceeded),
		Canceled:        errors.Is(ctxErr, context.Canceled),
	}

	if ctxErr != nil && res.ExitCode == 0 {
		if res.TimedOut {
			res.ExitCode = 124
		} else {
			res.ExitCode = 130
		}
	}

	if (terminalFailure || streamErr != nil) && res.ExitCode == 0 {
		res.ExitCode = 1
	}

	if (res.ExitCode != 0 || ctxErr != nil) && opts.Logger != nil {
		opts.Logger.Error("headless command failed",
			logging.Field{Key: "provider", Value: opts.Provider.ID()},
			logging.Field{Key: "exit_code", Value: fmt.Sprintf("%d", res.ExitCode)},
			logging.Field{Key: "timed_out", Value: fmt.Sprintf("%t", res.TimedOut)},
			logging.Field{Key: "stderr", Value: res.Stderr},
		)
	}

	if ctxErr != nil {
		return res, ctxErr
	}
	if waitErr == nil && streamErr != nil {
		return res, streamErr
	}
	return res, nil
}
