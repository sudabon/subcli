package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/provider"
)

// CompareTarget describes a single provider invocation inside a parallel compare run.
type CompareTarget struct {
	Provider       provider.Provider
	BinPath        string
	Request        provider.RunRequest
	Auth           provider.AuthMode
	MaxConcurrency int
	BaseEnv        []string
	GracePeriod    time.Duration
	StderrCap      int
}

// CompareEvent tags a normalized Event with the 0-based target index in RunCompare.
type CompareEvent struct {
	Index int
	Event event.Event
}

// CompareOutcome captures the aggregated result of a single target in RunCompare.
type CompareOutcome struct {
	Index         int
	Provider      string
	Auth          provider.AuthMode
	Events        []event.Event
	Text          string
	ErrorMessages []string
	Usage         *event.UsageInfo
	Result        Result
	Err           error
}

// CompareOptions configures RunCompare execution.
type CompareOptions struct {
	Logger  *logging.Logger
	OnEvent func(CompareEvent)
}

// RunCompare executes all targets concurrently while strictly enforcing each
// provider's max_concurrency via a per-provider buffered-channel semaphore.
// All targets run to completion (one target failing does not cancel peers)
// unless ctx itself is canceled or times out.
// Events emitted by workers are funneled through a single aggregator goroutine
// so opts.OnEvent is never invoked concurrently.
func RunCompare(ctx context.Context, targets []CompareTarget, opts CompareOptions) []CompareOutcome {
	outcomes := make([]CompareOutcome, len(targets))
	if len(targets) == 0 {
		return outcomes
	}

	semaphores := make(map[string]chan struct{})
	limits := make(map[string]int)
	for _, t := range targets {
		if t.Provider == nil {
			continue
		}
		id := t.Provider.ID()
		limit := t.MaxConcurrency
		if limit <= 0 {
			limit = 1
		}
		if prev, exists := limits[id]; !exists || limit < prev {
			limits[id] = limit
		}
	}
	for id, limit := range limits {
		semaphores[id] = make(chan struct{}, limit)
	}

	eventsCh := make(chan CompareEvent, 64)
	aggDone := make(chan struct{})
	go func() {
		defer close(aggDone)
		for ce := range eventsCh {
			if opts.OnEvent != nil {
				opts.OnEvent(ce)
			}
		}
	}()

	var wg sync.WaitGroup
	for i, target := range targets {
		outcomes[i].Index = i
		outcomes[i].Auth = target.Auth
		if target.Provider != nil {
			outcomes[i].Provider = target.Provider.ID()
		}

		wg.Add(1)
		go func(idx int, t CompareTarget) {
			defer wg.Done()

			if t.Provider == nil {
				outcomes[idx].Result = Result{ExitCode: 1}
				outcomes[idx].Err = errors.New("runner: provider is required")
				return
			}

			sem := semaphores[t.Provider.ID()]
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				err := ctx.Err()
				code := 130
				if errors.Is(err, context.DeadlineExceeded) {
					code = 124
				}
				outcomes[idx].Result = Result{
					ExitCode: code,
					TimedOut: errors.Is(err, context.DeadlineExceeded),
					Canceled: errors.Is(err, context.Canceled),
				}
				outcomes[idx].Err = err
				return
			}

			var textBuf strings.Builder
			res, runErr := Run(ctx, Options{
				BinPath:     t.BinPath,
				Provider:    t.Provider,
				Request:     t.Request,
				Auth:        t.Auth,
				BaseEnv:     t.BaseEnv,
				Logger:      opts.Logger,
				GracePeriod: t.GracePeriod,
				StderrCap:   t.StderrCap,
				OnEvent: func(ev event.Event) {
					if ev.Provider == "" {
						ev.Provider = t.Provider.ID()
					}
					outcomes[idx].Events = append(outcomes[idx].Events, ev)
					switch ev.Kind {
					case event.Text:
						textBuf.WriteString(ev.Text)
					case event.Usage:
						if ev.Usage != nil {
							cp := *ev.Usage
							outcomes[idx].Usage = &cp
						}
					case event.Error:
						if strings.TrimSpace(ev.Text) != "" {
							outcomes[idx].ErrorMessages = append(outcomes[idx].ErrorMessages, ev.Text)
						}
					}
					eventsCh <- CompareEvent{Index: idx, Event: ev}
				},
			})
			outcomes[idx].Text = textBuf.String()
			outcomes[idx].Result = res
			outcomes[idx].Err = runErr
		}(i, target)
	}

	wg.Wait()
	close(eventsCh)
	<-aggDone

	return outcomes
}
