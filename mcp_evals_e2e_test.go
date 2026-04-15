//go:build e2e

package evaluations

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2E_BasicEvaluation(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping e2e test")
	}

	// Build test server
	serverPath := buildTestServer(t)

	// Configure eval client
	config := EvalClientConfig{
		APIKey:       apiKey,
		Command:      serverPath,
		Args:         []string{},
		Env:          []string{},
		Model:        "claude-sonnet-4-5-20250929", // claude-sonnet-4-0
		GradingModel: "claude-sonnet-4-6",
	}

	client := NewEvalClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Run evaluation
	evalRunResult, err := client.RunEval(ctx, Eval{
		Name:           "basic_addition",
		Description:    "Test basic addition",
		Prompt:         "What is 5 plus 3?",
		ExpectedResult: "Should return 8",
	})
	if err != nil {
		t.Fatalf("RunEval failed: %v", err)
	}

	// Check for errors in the result
	if evalRunResult.Error != nil {
		t.Fatalf("Eval execution error: %v", evalRunResult.Error)
	}

	// Verify result
	if evalRunResult.Result == nil || evalRunResult.Result.RawResponse == "" {
		t.Fatal("Expected non-empty result")
	}

	// Check if answer contains expected value
	if !strings.Contains(evalRunResult.Result.RawResponse, "8") {
		t.Errorf("Expected answer to contain '8', got: %s", evalRunResult.Result.RawResponse)
	}

	t.Logf("Evaluation result: %s", evalRunResult.Result.RawResponse)

	// Validate grade structure (auto-graded)
	if evalRunResult.Grade == nil {
		t.Fatal("Expected grade to be auto-generated")
	}
	validateGrade(t, evalRunResult.Grade)
	t.Logf("Grade: Accuracy=%d, Completeness=%d, Relevance=%d, Clarity=%d, Reasoning=%d",
		evalRunResult.Grade.Accuracy, evalRunResult.Grade.Completeness, evalRunResult.Grade.Relevance,
		evalRunResult.Grade.Clarity, evalRunResult.Grade.Reasoning)

	// Validate trace data
	validateTrace(t, evalRunResult.Trace)

	// Save trace for inspection
	saveTrace(t, evalRunResult.Trace, "basic_evaluation_trace.json")
}

func TestE2E_MultipleTools(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping e2e test")
	}

	// Build test server
	serverPath := buildTestServer(t)

	// Configure eval client
	config := EvalClientConfig{
		APIKey:  apiKey,
		Command: serverPath,
		Args:    []string{},
		Env:     []string{},
		Model:   "claude-sonnet-4-6",
	}

	client := NewEvalClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Run evaluation with multiple tools
	evalRunResult, err := client.RunEval(ctx, Eval{
		Name:           "multiple_tools",
		Description:    "Test using multiple tools in sequence",
		Prompt:         "Echo the message 'hello world' and tell me what time it is",
		ExpectedResult: "Should echo 'hello world' and provide current time",
	})
	if err != nil {
		t.Fatalf("RunEval failed: %v", err)
	}

	// Check for errors in the result
	if evalRunResult.Error != nil {
		t.Fatalf("Eval execution error: %v", evalRunResult.Error)
	}

	// Verify result
	if evalRunResult.Result == nil || evalRunResult.Result.RawResponse == "" {
		t.Fatal("Expected non-empty result")
	}

	// Check if answer contains expected content
	if !strings.Contains(strings.ToLower(evalRunResult.Result.RawResponse), "hello world") {
		t.Errorf("Expected answer to contain 'hello world', got: %s", evalRunResult.Result.RawResponse)
	}

	t.Logf("Evaluation result: %s", evalRunResult.Result.RawResponse)

	// Validate grade structure (auto-graded)
	if evalRunResult.Grade == nil {
		t.Fatal("Expected grade to be auto-generated")
	}
	validateGrade(t, evalRunResult.Grade)
	t.Logf("Grade: Accuracy=%d, Completeness=%d, Relevance=%d, Clarity=%d, Reasoning=%d",
		evalRunResult.Grade.Accuracy, evalRunResult.Grade.Completeness, evalRunResult.Grade.Relevance,
		evalRunResult.Grade.Clarity, evalRunResult.Grade.Reasoning)

	// Validate trace data
	validateTrace(t, evalRunResult.Trace)

	// Save trace for inspection
	saveTrace(t, evalRunResult.Trace, "multiple_tools_trace.json")
}

func TestE2E_EnvironmentVariables(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping e2e test")
	}

	// Build test server
	serverPath := buildTestServer(t)

	// Configure eval client with custom environment variable
	testToken := "test-secret-token-12345"
	config := EvalClientConfig{
		APIKey:  apiKey,
		Command: serverPath,
		Args:    []string{},
		Env:     []string{"TEST_API_TOKEN=" + testToken},
		Model:   "claude-sonnet-4-6",
	}

	client := NewEvalClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Run evaluation that requires checking an environment variable
	evalRunResult, err := client.RunEval(ctx, Eval{
		Name:           "environment_variables",
		Description:    "Test accessing custom environment variables",
		Prompt:         "What is the value of the TEST_API_TOKEN environment variable?",
		ExpectedResult: "Should return '" + testToken + "'",
	})
	if err != nil {
		t.Fatalf("RunEval failed: %v", err)
	}

	// Check for errors in the result
	if evalRunResult.Error != nil {
		t.Fatalf("Eval execution error: %v", evalRunResult.Error)
	}

	// Verify result contains the token value
	if evalRunResult.Result == nil || evalRunResult.Result.RawResponse == "" {
		t.Fatal("Expected non-empty result")
	}

	// Check if answer contains the test token
	if !strings.Contains(evalRunResult.Result.RawResponse, testToken) {
		t.Errorf("Expected answer to contain test token '%s', got: %s", testToken, evalRunResult.Result.RawResponse)
	}

	t.Logf("Evaluation result: %s", evalRunResult.Result.RawResponse)

	// Validate grade structure (auto-graded)
	if evalRunResult.Grade == nil {
		t.Fatal("Expected grade to be auto-generated")
	}
	validateGrade(t, evalRunResult.Grade)
	t.Logf("Grade: Accuracy=%d, Completeness=%d, Relevance=%d, Clarity=%d, Reasoning=%d",
		evalRunResult.Grade.Accuracy, evalRunResult.Grade.Completeness, evalRunResult.Grade.Relevance,
		evalRunResult.Grade.Clarity, evalRunResult.Grade.Reasoning)

	// Validate trace data
	validateTrace(t, evalRunResult.Trace)
}

func TestE2E_GradingScores(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping e2e test")
	}

	// Build test server
	serverPath := buildTestServer(t)

	// Configure eval client
	config := EvalClientConfig{
		APIKey:  apiKey,
		Command: serverPath,
		Args:    []string{},
		Env:     []string{},
		Model:   "claude-sonnet-4-6",
	}

	client := NewEvalClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Run evaluation
	evalRunResult, err := client.RunEval(ctx, Eval{
		Name:           "grading_test",
		Description:    "Test grading scores for correct answer",
		Prompt:         "What is 10 plus 20?",
		ExpectedResult: "Should return 30",
	})
	if err != nil {
		t.Fatalf("RunEval failed: %v", err)
	}

	// Check for errors in the result
	if evalRunResult.Error != nil {
		t.Fatalf("Eval execution error: %v", evalRunResult.Error)
	}

	// Validate grade structure and scores (auto-graded)
	if evalRunResult.Grade == nil {
		t.Fatal("Expected grade to be auto-generated")
	}
	validateGrade(t, evalRunResult.Grade)

	// Check that scores are reasonable for a correct answer
	// We expect high scores since the answer should be correct
	if evalRunResult.Grade.Accuracy < 3 {
		t.Errorf("Expected accuracy >= 3 for correct answer, got %d", evalRunResult.Grade.Accuracy)
	}

	t.Logf("Grade details:")
	t.Logf("  Accuracy: %d", evalRunResult.Grade.Accuracy)
	t.Logf("  Completeness: %d", evalRunResult.Grade.Completeness)
	t.Logf("  Relevance: %d", evalRunResult.Grade.Relevance)
	t.Logf("  Clarity: %d", evalRunResult.Grade.Clarity)
	t.Logf("  Reasoning: %d", evalRunResult.Grade.Reasoning)
	t.Logf("  Overall: %s", evalRunResult.Grade.OverallComment)

	// Validate trace data
	validateTrace(t, evalRunResult.Trace)
}

// buildTestServer builds the test MCP server and returns the path to the binary
func buildTestServer(t *testing.T) string {
	t.Helper()

	serverDir := filepath.Join("testdata", "mcp-test-server")
	outputPath := filepath.Join(t.TempDir(), "test-server")

	cmd := exec.Command("go", "build", "-o", outputPath, ".")
	cmd.Dir = serverDir

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build test server: %v\n%s", err, output)
	}

	return outputPath
}

func TestE2E_LoadConfigAndRunEvals(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping e2e test")
	}

	// Build test server
	serverPath := buildTestServer(t)

	// Load config from YAML
	config, err := LoadConfig("testdata/mcp-test-evals.yaml")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Override the command to use the built test server
	config.MCPServer.Command = serverPath
	config.MCPServer.Args = []string{}

	// Create eval client from config
	evalConfig := EvalClientConfig{
		APIKey:       apiKey,
		Command:      config.MCPServer.Command,
		Args:         config.MCPServer.Args,
		Env:          config.MCPServer.Env,
		Model:        config.Model,
		GradingModel: config.GradingModel,
	}
	client := NewEvalClient(evalConfig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run all evals from config
	results, err := client.RunEvals(ctx, config.Evals)
	if err != nil {
		t.Fatalf("RunEvals failed: %v", err)
	}

	// Verify we got results for all evals
	if len(results) != len(config.Evals) {
		t.Errorf("expected %d results, got %d", len(config.Evals), len(results))
	}

	// Check each result
	for i, result := range results {
		t.Logf("Eval %d: %s", i, result.Eval.Name)

		if result.Error != nil {
			t.Errorf("Eval %s failed: %v", result.Eval.Name, result.Error)
			continue
		}

		if result.Result == nil {
			t.Errorf("Eval %s has no result", result.Eval.Name)
			continue
		}

		if result.Result.RawResponse == "" {
			t.Errorf("Eval %s has empty response", result.Eval.Name)
			continue
		}

		if result.Grade == nil {
			t.Errorf("Eval %s has no grade", result.Eval.Name)
			continue
		}

		validateGrade(t, result.Grade)
		t.Logf("  Response: %s", result.Result.RawResponse)
		t.Logf("  Grade: Accuracy=%d, Completeness=%d, Relevance=%d, Clarity=%d, Reasoning=%d",
			result.Grade.Accuracy, result.Grade.Completeness, result.Grade.Relevance,
			result.Grade.Clarity, result.Grade.Reasoning)

		// Validate trace data
		validateTrace(t, result.Trace)

		// Save trace for inspection
		traceFilename := fmt.Sprintf("batch_%s_trace.json", result.Eval.Name)
		saveTrace(t, result.Trace, traceFilename)
	}

	// Verify specific results based on eval names
	evalsByName := make(map[string]EvalRunResult)
	for _, result := range results {
		evalsByName[result.Eval.Name] = result
	}

	// Check "add" eval
	if addResult, ok := evalsByName["add"]; ok {
		if !strings.Contains(addResult.Result.RawResponse, "8") {
			t.Errorf("Expected 'add' eval to contain '8', got: %s", addResult.Result.RawResponse)
		}
	} else {
		t.Error("Missing 'add' eval in results")
	}

	// Check "get_user_info" eval
	if getUserResult, ok := evalsByName["get_user_info"]; ok {
		response := strings.ToLower(getUserResult.Result.RawResponse)
		if !strings.Contains(response, "alice") || !strings.Contains(response, "user-123") {
			t.Errorf("Expected 'get_user_info' eval to contain 'Alice' and 'user-123', got: %s", getUserResult.Result.RawResponse)
		}
	} else {
		t.Error("Missing 'get_user_info' eval in results")
	}
}

// validateGrade validates that a GradeResult has all required fields and valid values
func validateGrade(t *testing.T, grade *GradeResult) {
	t.Helper()

	if grade == nil {
		t.Fatal("Expected non-nil grade")
	}

	dimensions := []struct {
		name  string
		score int
	}{
		{"accuracy", grade.Accuracy},
		{"completeness", grade.Completeness},
		{"relevance", grade.Relevance},
		{"clarity", grade.Clarity},
		{"reasoning", grade.Reasoning},
	}

	for _, dim := range dimensions {
		if dim.score < 0 || dim.score > 5 {
			t.Errorf("%s score %d out of valid range [0-5]", dim.name, dim.score)
		}
	}

	if grade.OverallComment == "" {
		t.Error("overall_comments is empty")
	}
}

// validateTrace validates that an EvalTrace has all required data
func validateTrace(t *testing.T, trace *EvalTrace) {
	t.Helper()

	if trace == nil {
		t.Fatal("Expected non-nil trace")
	}

	// Validate basic metrics
	if trace.StepCount == 0 {
		t.Error("Expected at least one step in trace")
	}

	if len(trace.Steps) != trace.StepCount {
		t.Errorf("Step count mismatch: StepCount=%d but len(Steps)=%d", trace.StepCount, len(trace.Steps))
	}

	if trace.TotalInputTokens == 0 {
		t.Error("Expected non-zero input tokens")
	}

	if trace.TotalOutputTokens == 0 {
		t.Error("Expected non-zero output tokens")
	}

	if trace.TotalDuration == 0 {
		t.Error("Expected non-zero duration")
	}

	// Validate each step
	for i, step := range trace.Steps {
		if step.StepNumber != i+1 {
			t.Errorf("Step %d has wrong step number: %d", i, step.StepNumber)
		}

		if step.StartTime.IsZero() {
			t.Errorf("Step %d has zero start time", i)
		}

		if step.EndTime.IsZero() {
			t.Errorf("Step %d has zero end time", i)
		}

		if step.Duration == 0 {
			t.Errorf("Step %d has zero duration", i)
		}

		if step.StopReason == "" {
			t.Errorf("Step %d has empty stop reason", i)
		}

		// Validate tool calls if present
		for j, tc := range step.ToolCalls {
			if tc.ToolID == "" {
				t.Errorf("Step %d, tool call %d has empty tool ID", i, j)
			}

			if tc.ToolName == "" {
				t.Errorf("Step %d, tool call %d has empty tool name", i, j)
			}

			if tc.Duration == 0 {
				t.Errorf("Step %d, tool call %d has zero duration", i, j)
			}

			if len(tc.Input) == 0 {
				t.Errorf("Step %d, tool call %d has empty input", i, j)
			}

			if len(tc.Output) == 0 {
				t.Errorf("Step %d, tool call %d has empty output", i, j)
			}
		}
	}

	// Validate grading trace
	if trace.Grading == nil {
		t.Error("Expected non-nil grading trace")
		return
	}

	if trace.Grading.UserPrompt == "" {
		t.Error("Grading trace has empty user prompt")
	}

	if trace.Grading.ModelResponse == "" {
		t.Error("Grading trace has empty model response")
	}

	if trace.Grading.GradingPrompt == "" {
		t.Error("Grading trace has empty grading prompt")
	}

	if trace.Grading.RawGradingOutput == "" {
		t.Error("Grading trace has empty raw grading output")
	}

	if trace.Grading.InputTokens == 0 {
		t.Error("Grading trace has zero input tokens")
	}

	if trace.Grading.OutputTokens == 0 {
		t.Error("Grading trace has zero output tokens")
	}

	if trace.Grading.Duration == 0 {
		t.Error("Grading trace has zero duration")
	}

	t.Logf("Trace summary: %d steps, %d tool calls, %d total tokens, duration: %v",
		trace.StepCount, trace.ToolCallCount, trace.TotalInputTokens+trace.TotalOutputTokens, trace.TotalDuration)
}

// saveTrace saves an evaluation trace to a JSON file for inspection
func saveTrace(t *testing.T, trace *EvalTrace, filename string) {
	t.Helper()

	// Create traces directory if it doesn't exist
	tracesDir := "traces"
	if err := os.MkdirAll(tracesDir, 0755); err != nil {
		t.Logf("Warning: failed to create traces directory: %v", err)
		return
	}

	// Write trace to JSON file
	traceJSON, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		t.Logf("Warning: failed to marshal trace: %v", err)
		return
	}

	outputPath := filepath.Join(tracesDir, filename)
	if err := os.WriteFile(outputPath, traceJSON, 0644); err != nil {
		t.Logf("Warning: failed to write trace file: %v", err)
		return
	}

	t.Logf("Trace saved to: %s", outputPath)
}
