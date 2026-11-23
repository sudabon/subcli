package runner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/sudabon/subcli/internal/event"
	"github.com/sudabon/subcli/internal/logging"
	"github.com/sudabon/subcli/internal/provider"
)

const (
	// MinChainSteps is the minimum allowed number of steps in subcli chain.
	MinChainSteps = 2
	// MaxChainSteps is the hard upper bound of steps in subcli chain (acyclic, no loops).
	MaxChainSteps = 3
)

// ChainTemplateData holds the template variables available to --then templates.
type ChainTemplateData struct {
	Prev string
}

// ChainStep configures a single step in a fixed 2-to-3 step acyclic chain.
type ChainStep struct {
	Provider    provider.Provider
	BinPath     string
	Request     provider.RunRequest // Prompt is populated by RunChain.
	Auth        provider.AuthMode
	BaseEnv     []string
	GracePeriod time.Duration
	StderrCap   int
}

// ChainStepResult holds the output and status of a completed chain step.
type ChainStepResult struct {
	StepIndex int
	Provider  string
	Auth      provider.AuthMode
	Prompt    string
	Text      string
	Usage     *event.UsageInfo
	Events    []event.Event
	Result    Result
	Err       error
}

// ChainOptions provides hooks and logging for RunChain.
type ChainOptions struct {
	Logger         *logging.Logger
	OnStepStart    func(stepIndex, totalSteps int, provID string, auth provider.AuthMode, prompt string)
	OnEvent        func(stepIndex int, ev event.Event)
	OnStepComplete func(stepIndex, totalSteps int, res ChainStepResult)
}

// ChainError describes a failure during a specific step of RunChain.
type ChainError struct {
	StepIndex int
	Provider  string
	Result    Result
	Err       error
}

func (e *ChainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("ステップ %d (%s) で失敗しました: %v", e.StepIndex+1, e.Provider, e.Err)
	}
	return fmt.Sprintf("ステップ %d (%s) が終了コード %d で失敗しました", e.StepIndex+1, e.Provider, e.Result.ExitCode)
}

func (e *ChainError) Unwrap() error { return e.Err }

// ValidateChainStepCount enforces the fixed 2-to-3 step boundary before execution.
func ValidateChainStepCount(stepCount int) error {
	if stepCount < MinChainSteps || stepCount > MaxChainSteps {
		return fmt.Errorf("chain の段数は %d〜%d 段に制限されています（指定: %d 段）", MinChainSteps, MaxChainSteps, stepCount)
	}
	return nil
}

// CompileChainTemplates validates and compiles --then templates before any child process starts.
// Either 1 template (reused for every subsequent step) or stepCount-1 templates are accepted.
func CompileChainTemplates(stepCount int, thenTemplates []string) ([]*template.Template, error) {
	if err := ValidateChainStepCount(stepCount); err != nil {
		return nil, err
	}
	needed := stepCount - 1
	if len(thenTemplates) != 1 && len(thenTemplates) != needed {
		return nil, fmt.Errorf("--then テンプレートは 1 個または %d 個指定してください（指定: %d 個）", needed, len(thenTemplates))
	}

	compiled := make([]*template.Template, len(thenTemplates))
	for i, raw := range thenTemplates {
		if strings.TrimSpace(raw) == "" {
			return nil, fmt.Errorf("--then テンプレート #%d が空です", i+1)
		}
		tmpl, err := template.New(fmt.Sprintf("then_%d", i+1)).Option("missingkey=error").Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("--then テンプレート #%d の構文が不正です: %w", i+1, err)
		}
		// Dry-run against ChainTemplateData to catch invalid field references before step 1 launches.
		var checkBuf bytes.Buffer
		if err := tmpl.Execute(&checkBuf, ChainTemplateData{Prev: "preview"}); err != nil {
			return nil, fmt.Errorf("--then テンプレート #%d の展開検証に失敗しました（利用可能な変数は {{.Prev}} のみです）: %w", i+1, err)
		}
		compiled[i] = tmpl
	}
	return compiled, nil
}

// RenderChainPrompt renders tmpl with Prev set to prevText.
func RenderChainPrompt(tmpl *template.Template, prevText string) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ChainTemplateData{Prev: prevText}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RunChain executes a strictly acyclic 2-to-3 step pipeline.
// Each step's concatenated event.Text output is injected into {{.Prev}} for the
// next step. Any step failure or empty intermediate output aborts immediately
// without launching any subsequent step.
func RunChain(
	ctx context.Context,
	steps []ChainStep,
	initialPrompt string,
	thenTemplates []string,
	opts ChainOptions,
) ([]ChainStepResult, error) {
	if err := ValidateChainStepCount(len(steps)); err != nil {
		return nil, err
	}
	if strings.TrimSpace(initialPrompt) == "" {
		return nil, fmt.Errorf("--prompt は空にできません")
	}
	compiled, err := CompileChainTemplates(len(steps), thenTemplates)
	if err != nil {
		return nil, err
	}

	// Pre-validate every step's provider and arguments before launching step 1.
	for i, s := range steps {
		if s.Provider == nil {
			return nil, fmt.Errorf("ステップ %d のプロバイダが未設定です", i+1)
		}
		checkReq := s.Request
		checkReq.Prompt = "validate"
		if _, err := s.Provider.HeadlessArgv(checkReq); err != nil {
			return nil, fmt.Errorf("ステップ %d (%s) の引数が不正です: %w", i+1, s.Provider.ID(), err)
		}
	}

	results := make([]ChainStepResult, 0, len(steps))
	var prevText string

	for i, step := range steps {
		var prompt string
		if i == 0 {
			prompt = initialPrompt
		} else {
			tmplIdx := i - 1
			if len(compiled) == 1 {
				tmplIdx = 0
			}
			rendered, renderErr := RenderChainPrompt(compiled[tmplIdx], prevText)
			if renderErr != nil {
				return results, &ChainError{
					StepIndex: i,
					Provider:  step.Provider.ID(),
					Result:    Result{ExitCode: 1},
					Err:       fmt.Errorf("テンプレート展開に失敗しました: %w", renderErr),
				}
			}
			prompt = rendered
		}

		if opts.OnStepStart != nil {
			opts.OnStepStart(i, len(steps), step.Provider.ID(), step.Auth, prompt)
		}

		req := step.Request
		req.Prompt = prompt

		var stepEvents []event.Event
		var textBuf strings.Builder
		var usage *event.UsageInfo

		res, runErr := Run(ctx, Options{
			BinPath:     step.BinPath,
			Provider:    step.Provider,
			Request:     req,
			Auth:        step.Auth,
			BaseEnv:     step.BaseEnv,
			Logger:      opts.Logger,
			GracePeriod: step.GracePeriod,
			StderrCap:   step.StderrCap,
			OnEvent: func(ev event.Event) {
				if ev.Provider == "" {
					ev.Provider = step.Provider.ID()
				}
				stepEvents = append(stepEvents, ev)
				switch ev.Kind {
				case event.Text:
					textBuf.WriteString(ev.Text)
				case event.Usage:
					if ev.Usage != nil {
						cp := *ev.Usage
						usage = &cp
					}
				}
				if opts.OnEvent != nil {
					opts.OnEvent(i, ev)
				}
			},
		})

		stepRes := ChainStepResult{
			StepIndex: i,
			Provider:  step.Provider.ID(),
			Auth:      step.Auth,
			Prompt:    prompt,
			Text:      textBuf.String(),
			Usage:     usage,
			Events:    stepEvents,
			Result:    res,
			Err:       runErr,
		}
		results = append(results, stepRes)

		if opts.OnStepComplete != nil {
			opts.OnStepComplete(i, len(steps), stepRes)
		}

		if runErr != nil || res.ExitCode != 0 {
			return results, &ChainError{
				StepIndex: i,
				Provider:  step.Provider.ID(),
				Result:    res,
				Err:       runErr,
			}
		}

		if i < len(steps)-1 && strings.TrimSpace(stepRes.Text) == "" {
			return results, &ChainError{
				StepIndex: i,
				Provider:  step.Provider.ID(),
				Result:    Result{ExitCode: 1},
				Err:       fmt.Errorf("出力テキストが空のため、次段へ引き継げません"),
			}
		}

		prevText = stepRes.Text
	}

	return results, nil
}
