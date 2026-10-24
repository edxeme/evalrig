// Package cli implements the evalrig command line interface.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edxeme/evalrig/internal/agent"
	"github.com/edxeme/evalrig/internal/config"
	"github.com/edxeme/evalrig/internal/harness"
	"github.com/edxeme/evalrig/internal/llm"
	"github.com/edxeme/evalrig/internal/report"
	"github.com/edxeme/evalrig/internal/runtime"
)

// Version is the evalrig release version.
const Version = "0.1.0"

const usageText = `evalrig — deterministic evals for LLM coding agents

Usage:
  evalrig run      --suite <file> [flags]   evaluate a suite
  evalrig validate --suite <file>           check a suite file
  evalrig list     --suite <file>           print the task inventory
  evalrig version                           print the version

Run flags:
  --suite string        path to the suite file (default "suite.yaml")
  --agents string       comma-separated: golden, openai, anthropic, ollama (default "golden")
  --concurrency int     parallel (task, agent) pairs (default 4)
  --model string        model override for chat agents
  --max-turns int       turn budget for chat agents (default 8)
  --out string          directory for reports and trajectories (default "runs")

Chat agents read OPENAI_API_KEY / ANTHROPIC_API_KEY from the environment.
`

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	switch cmd, rest := args[0], args[1:]; cmd {
	case "run":
		return runCmd(rest, stdout, stderr)
	case "validate":
		return validateCmd(rest, stdout, stderr)
	case "list":
		return listCmd(rest, stdout, stderr)
	case "version":
		fmt.Fprintf(stdout, "evalrig %s\n", Version)
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usageText)
		return 0
	default:
		fmt.Fprintf(stderr, "evalrig: unknown command %q\n\n%s", cmd, usageText)
		return 2
	}
}

func newSuiteFlags(name string, stderr io.Writer) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs, fs.String("suite", "suite.yaml", "path to the suite file")
}

func validateCmd(args []string, stdout, stderr io.Writer) int {
	fs, suitePath := newSuiteFlags("validate", stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	suite, err := config.Load(*suitePath)
	if err != nil {
		fmt.Fprintf(stderr, "evalrig: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s: %d tasks ok\n", suite.Name, len(suite.Tasks))
	return 0
}

func listCmd(args []string, stdout, stderr io.Writer) int {
	fs, suitePath := newSuiteFlags("list", stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	suite, err := config.Load(*suitePath)
	if err != nil {
		fmt.Fprintf(stderr, "evalrig: %v\n", err)
		return 1
	}
	for _, t := range suite.Tasks {
		timeout, _ := suite.Timeout(t)
		fmt.Fprintf(stdout, "%s\t%s\t%d verifier(s)\t%s\n", t.ID, t.Image, len(t.Verify), timeout)
	}
	return 0
}

func runCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	suitePath := fs.String("suite", "suite.yaml", "path to the suite file")
	agents := fs.String("agents", "golden", "comma-separated: golden, openai, anthropic, ollama")
	concurrency := fs.Int("concurrency", 4, "parallel (task, agent) pairs")
	model := fs.String("model", "", "model override for chat agents")
	maxTurns := fs.Int("max-turns", agent.DefaultMaxTurns, "turn budget for chat agents")
	out := fs.String("out", "runs", "directory for reports and trajectories")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	suite, err := config.Load(*suitePath)
	if err != nil {
		fmt.Fprintf(stderr, "evalrig: %v\n", err)
		return 1
	}
	agentList, err := buildAgents(strings.Split(*agents, ","), *model, *maxTurns)
	if err != nil {
		fmt.Fprintf(stderr, "evalrig: %v\n", err)
		return 1
	}

	ctx := context.Background()
	rt, err := runtime.NewDocker(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "evalrig: %v\n", err)
		return 1
	}
	defer rt.Close()

	outDir := filepath.Join(*out, time.Now().Format("20060102-150405"))
	h := &harness.Harness{Runtime: rt, Concurrency: *concurrency, OutDir: outDir}
	started := time.Now()
	results := h.Run(ctx, suite, agentList)

	rep := &report.Report{Suite: suite.Name, StartedAt: started, Results: results}
	mdPath, jsonPath, err := rep.Write(outDir)
	if err != nil {
		fmt.Fprintf(stderr, "evalrig: write report: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, rep.Markdown())
	fmt.Fprintf(stdout, "reports: %s · %s\n", mdPath, jsonPath)

	if rep.Passed() != len(results) {
		return 1
	}
	return 0
}

// buildAgents maps CLI agent names to configured agents.
func buildAgents(names []string, model string, maxTurns int) ([]agent.Agent, error) {
	var list []agent.Agent
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		switch name {
		case "":
			continue
		case "golden":
			list = append(list, agent.Golden{})
		case llm.ProviderOpenAI, llm.ProviderAnthropic, llm.ProviderOllama:
			if model == "" {
				model = llm.DefaultModel(name)
			}
			opts := llm.Options{Model: model}
			switch name {
			case llm.ProviderOpenAI:
				opts.APIKey = os.Getenv("OPENAI_API_KEY")
			case llm.ProviderAnthropic:
				opts.APIKey = os.Getenv("ANTHROPIC_API_KEY")
			}
			client, err := llm.New(name, opts)
			if err != nil {
				return nil, err
			}
			list = append(list, agent.NewChat(client, model, maxTurns))
		default:
			return nil, fmt.Errorf("unknown agent %q (want golden, openai, anthropic or ollama)", name)
		}
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no agents selected")
	}
	return list, nil
}
