package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"
	evaluations "github.com/wolfeidau/mcp-evals"
	"github.com/wolfeidau/mcp-evals/internal/help"
	"github.com/wolfeidau/mcp-evals/internal/reporting"
)

// RunCmd handles the run command
type RunCmd struct {
	Quiet    bool   `help:"Suppress progress output, only show summary" short:"q"`
	TraceDir string `help:"Directory to write trace files" type:"path"`
	Config   string `help:"Path to evaluation configuration file (YAML or JSON)" required:"" type:"path"`
	APIKey   string `help:"Anthropic API key (overrides ANTHROPIC_API_KEY env var)"`
	BaseURL  string `help:"Base URL for Anthropic API (overrides ANTHROPIC_BASE_URL env var)"`
	Verbose  bool   `help:"Show detailed per-eval breakdown" short:"v"`
	Filter   string `help:"Regex pattern to filter which evals to run (matches against eval name)" short:"f"`

	// MCP Server overrides
	MCPCommand string   `help:"Override MCP server command from config"`
	MCPArgs    []string `help:"Override MCP server args from config"`
	MCPEnv     []string `help:"Override MCP server env vars from config"`
}

// Run executes the run command
func (r *RunCmd) Run(globals *Globals) error {
	// Load configuration
	config, err := evaluations.LoadConfig(r.Config)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Apply MCP server overrides from command-line flags
	if r.MCPCommand != "" {
		config.MCPServer.Command = r.MCPCommand
	}
	if len(r.MCPArgs) > 0 {
		config.MCPServer.Args = r.MCPArgs
	}
	if len(r.MCPEnv) > 0 {
		config.MCPServer.Env = r.MCPEnv
	}

	// Filter evals if pattern provided
	evalsToRun := config.Evals
	if r.Filter != "" {
		filtered, err := filterEvals(config.Evals, r.Filter)
		if err != nil {
			return fmt.Errorf("invalid filter pattern: %w", err)
		}
		if len(filtered) == 0 {
			return fmt.Errorf("no evals matched filter pattern: %s", r.Filter)
		}
		evalsToRun = filtered

		if !r.Quiet {
			fmt.Printf("Filter '%s' matched %d of %d eval(s)\n", r.Filter, len(filtered), len(config.Evals))
		}
	}

	// Parse timeout if specified
	var timeout time.Duration
	if config.Timeout != "" {
		timeout, err = time.ParseDuration(config.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}

	// Create context with timeout
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Resolve base URL: flag takes precedence, then env var
	resolvedBaseURL := r.BaseURL
	if resolvedBaseURL == "" {
		resolvedBaseURL = os.Getenv("ANTHROPIC_BASE_URL")
	}

	// Create client
	client := createClient(config, r.APIKey, resolvedBaseURL, r.Quiet)

	// Run evaluations
	if !r.Quiet {
		fmt.Printf("Running %d evaluation(s)...\n\n", len(evalsToRun))
	}

	results, err := runEvals(ctx, client, evalsToRun, r.Quiet)
	if err != nil {
		return err
	}

	// Write traces if directory specified
	if r.TraceDir != "" {
		if err := writeTraces(results, r.TraceDir); err != nil {
			log.Error().Err(err).Msg("failed to write traces")
			return fmt.Errorf("failed to write traces: %w", err)
		}
	}

	// Print summary using new reporting system
	if err := reporting.PrintStyledReport(results, r.Verbose); err != nil {
		return fmt.Errorf("failed to print report: %w", err)
	}

	// Check for failures
	if hasFailures(results) {
		return fmt.Errorf("evaluations failed")
	}

	return nil
}

func runEvals(ctx context.Context, client *evaluations.EvalClient, evals []evaluations.Eval, quiet bool) ([]evaluations.EvalRunResult, error) {
	styles := help.DefaultStyles()
	results := make([]evaluations.EvalRunResult, len(evals))

	// Style for indented content (description, status)
	indentStyle := lipgloss.NewStyle().Padding(0, 0, 0, 8)

	for i, eval := range evals {
		if !quiet {
			// Print eval header with index
			header := fmt.Sprintf("[%d/%d] Running eval: %s", i+1, len(evals), eval.Name)
			fmt.Println(styles.Heading.Render(header))

			if eval.Description != "" {
				desc := indentStyle.Render(styles.Muted.Render(eval.Description))
				fmt.Println(desc)
			}
		}

		result, err := client.RunEval(ctx, eval)
		if err != nil {
			results[i] = evaluations.EvalRunResult{
				Eval:  eval,
				Error: err,
			}
			if !quiet {
				errMsg := fmt.Sprintf("❌ Error: %v", err)
				fmt.Println(indentStyle.Render(styles.Error.Render(errMsg)))
				fmt.Println()
			}
			continue
		}

		results[i] = *result

		if !quiet {
			if result.Grade != nil {
				msg := fmt.Sprintf("✓ Completed (avg score: %.1f/5)", avgScore(result.Grade))
				fmt.Println(indentStyle.Render(styles.Success.Render(msg)))
			} else {
				fmt.Println(indentStyle.Render(styles.Success.Render("✓ Completed")))
			}
			fmt.Println()
		}
	}

	return results, nil
}

func writeTraces(results []evaluations.EvalRunResult, traceDir string) error {
	// Create trace directory if it doesn't exist
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return fmt.Errorf("failed to create trace directory: %w", err)
	}

	for _, result := range results {
		if result.Trace == nil {
			continue
		}

		filename := filepath.Join(traceDir, fmt.Sprintf("%s.json", result.Eval.Name))

		// Save full result (eval + grade + trace) for better report generation
		traceData := struct {
			Eval  evaluations.Eval         `json:"eval"`
			Grade *evaluations.GradeResult `json:"grade,omitempty"`
			Trace *evaluations.EvalTrace   `json:"trace"`
		}{
			Eval:  result.Eval,
			Grade: result.Grade,
			Trace: result.Trace,
		}

		data, err := json.MarshalIndent(traceData, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal trace for %s: %w", result.Eval.Name, err)
		}

		if err := os.WriteFile(filename, data, 0600); err != nil {
			return fmt.Errorf("failed to write trace for %s: %w", result.Eval.Name, err)
		}
	}

	return nil
}

func hasFailures(results []evaluations.EvalRunResult) bool {
	for _, result := range results {
		if result.Error != nil {
			return true
		}
		if result.Grade != nil {
			sum := result.Grade.Accuracy + result.Grade.Completeness + result.Grade.Relevance + result.Grade.Clarity + result.Grade.Reasoning
			avg := float64(sum) / 5.0
			if avg < 3.0 {
				return true
			}
		}
	}
	return false
}

func avgScore(grade *evaluations.GradeResult) float64 {
	sum := grade.Accuracy + grade.Completeness + grade.Relevance + grade.Clarity + grade.Reasoning
	return float64(sum) / 5.0
}

// filterEvals filters evaluations by regex pattern matching against eval names
func filterEvals(evals []evaluations.Eval, pattern string) ([]evaluations.Eval, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var filtered []evaluations.Eval
	for _, eval := range evals {
		if regex.MatchString(eval.Name) {
			filtered = append(filtered, eval)
		}
	}
	return filtered, nil
}
