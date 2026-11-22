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
	"time"

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

	cmd := exec.CommandContext(execCtx, binPath, argv...)
	cmd.Env = opts.Provider.ChildEnv(baseEnv, opts.Auth)
	if opts.Request.Cwd != "" {
		cmd.Dir = opts.Request.Cwd
	}
	configureProcAttr(cmd)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{ExitCode: 1}, fmt.Errorf("stdout パイプの作成に失敗しました: %w", err)
	}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return Result{ExitCode: 1}, fmt.Errorf("%s CLI の起動に失敗しました: %w", opts.Provider.ID(), err)
	}

	procDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			terminateProcessGroup(cmd.Process, procDone, gracePeriod)
		case <-procDone:
		}
	}()

	reader := bufio.NewReader(stdoutPipe)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimRight(line, "\r\n")
			if len(bytes.TrimSpace(trimmed)) > 0 {
				events, parseErr := opts.Provider.Parse(trimmed)
				if parseErr != nil {
					if opts.Logger != nil {
						opts.Logger.Warn("provider parse error",
							logging.Field{Key: "provider", Value: opts.Provider.ID()},
							logging.Field{Key: "error", Value: parseErr.Error()},
						)
					}
					if opts.OnEvent != nil {
						opts.OnEvent(event.NewUnknown(opts.Provider.ID(), trimmed, time.Now()))
					}
				} else if opts.OnEvent != nil {
					for _, ev := range events {
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
		ExitCode:        exitCodeFromError(waitErr),
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
	return res, nil
}
