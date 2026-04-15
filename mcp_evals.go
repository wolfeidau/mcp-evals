package evaluations

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

const (
	AgentSystemPrompt = "You are an assistant responsible for evaluating the results of calling various tools. Given the user's query, use the tools available to you to answer the question."

	EvalSystemPrompt = `You are an expert evaluator assessing how well an LLM answers a given question. Review the provided answer and score it from 1 to 5 in each of the following categories:

- Accuracy: Does the answer contain factual errors or hallucinations?
- Completeness: Does the answer fully address all parts of the question?
- Relevance: Is the information directly related to the question?
- Clarity: Is the explanation easy to understand and well-structured?
- Reasoning: Does the answer show logical thinking or provide evidence or rationale?

If custom grading criteria are provided below, use those specific requirements to inform your scoring. The custom criteria define what "complete", "accurate", etc. mean for this particular evaluation.

CRITICAL: Return ONLY a valid JSON object with no markdown formatting, no code blocks, and no explanation. Your entire response must be valid JSON starting with { and ending with }.

Use this exact format:
{
    "accuracy": 1-5,
    "completeness": 1-5,
    "relevance": 1-5,
    "clarity": 1-5,
    "reasoning": 1-5,
    "overall_comments": "A short paragraph summarizing the strengths and weaknesses of the answer, specifically noting which rubric criteria were met or missed if custom criteria were provided."
}`
)

type EvalClientConfig struct {
	APIKey               string
	BaseURL              string // Optional: if set, override the default Anthropic API endpoint
	Command              string
	Args                 []string
	Env                  []string
	Model                string
	GradingModel         string // Optional: if set, use this model for grading instead of Model
	AgentSystemPrompt    string // Optional: custom system prompt for the agent being evaluated
	MaxSteps             int
	MaxTokens            int
	EnablePromptCaching  *bool             // Optional: enable Anthropic prompt caching for tool definitions and system prompts. Default: true
	CacheTTL             string            // Optional: cache time-to-live, either "5m" (default) or "1h". Requires EnablePromptCaching=true
	EnforceMinimumScores *bool             // Optional: enforce minimum scores from grading rubrics. Default: true
	StderrCallback       func(line string) // Optional: called for each line written to stderr by the MCP server subprocess
}

// ApplyDefaults sets default values for optional configuration fields.
// This method modifies the config in-place and returns a pointer to it for method chaining.
func (c *EvalClientConfig) ApplyDefaults() *EvalClientConfig {
	if c.MaxSteps <= 0 {
		c.MaxSteps = 10
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 4096
	}
	if c.CacheTTL == "" {
		c.CacheTTL = "5m" // Default to 5-minute cache (free)
	}
	if c.EnablePromptCaching == nil {
		c.EnablePromptCaching = toPtr(true) // Enable prompt caching by default for cost savings
	}
	if c.EnforceMinimumScores == nil {
		c.EnforceMinimumScores = toPtr(true) // Enable minimum score enforcement by default
	}
	return c
}

type EvalClient struct {
	client anthropic.Client
	config EvalClientConfig
}

func NewEvalClient(config EvalClientConfig) *EvalClient {
	// Apply defaults for optional fields
	config.ApplyDefaults()

	opts := []option.RequestOption{}
	if config.APIKey != "" {
		opts = append(opts, option.WithAPIKey(config.APIKey))
	}
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}

	// enable 1m tokens beta for sonnet models
	opts = append(opts, option.WithHeader("anthropic-beta", anthropic.AnthropicBetaContext1m2025_08_07))

	return &EvalClient{
		client: anthropic.NewClient(opts...), // uses ANTHROPIC_API_KEY from env
		config: config,
	}
}

// loadMCPSession creates an MCP client, connects to the server, and retrieves available tools
func (ec *EvalClient) loadMCPSession(ctx context.Context) (*mcp.ClientSession, *mcp.ListToolsResult, error) {
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "mcp-client", Version: "v1.0.0"}, nil)
	// #nosec G204 - Command and args are provided by the library caller as part of EvalClientConfig
	cmd := exec.Command(ec.config.Command, ec.config.Args...)

	// Handle stderr based on whether a callback is provided
	if ec.config.StderrCallback != nil {
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}

		// Spawn goroutine to read stderr line-by-line and invoke callback
		go func() {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				ec.config.StderrCallback(scanner.Text())
			}
			// Ignore scanner errors as they typically occur when the process exits
		}()
	} else {
		cmd.Stderr = os.Stderr // forward subprocess stderr for visibility (backward compatible)
	}

	// If custom env vars are provided, append them to the parent environment
	if len(ec.config.Env) > 0 {
		cmd.Env = append(os.Environ(), ec.config.Env...)
	}

	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	session, err := mcpClient.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	// get all the tools
	toolsResp, err := session.ListTools(ctx, nil)
	if err != nil {
		_ = session.Close()
		return nil, nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return session, toolsResp, nil
}

// executeAndTraceToolCall executes a single MCP tool call and captures complete trace data
func (ec *EvalClient) executeAndTraceToolCall(
	ctx context.Context,
	toolUseBlock anthropic.ToolUseBlock,
	session *mcp.ClientSession,
) ToolCall {
	toolCall := ToolCall{
		ToolID:    toolUseBlock.ID,
		ToolName:  toolUseBlock.Name,
		StartTime: time.Now(),
	}

	// Capture input
	if inputJSON, err := json.Marshal(toolUseBlock.Input); err == nil {
		toolCall.Input = inputJSON
	}

	// Execute MCP tool call
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolUseBlock.Name,
		Arguments: toolUseBlock.Input,
	})

	toolCall.EndTime = time.Now()
	toolCall.Duration = toolCall.EndTime.Sub(toolCall.StartTime)

	if err != nil {
		toolCall.Success = false
		toolCall.Error = err.Error()
		// Create error output in JSON format for consistency
		errorOutput := map[string]string{"error": err.Error()}
		if outputJSON, marshalErr := json.Marshal(errorOutput); marshalErr == nil {
			toolCall.Output = outputJSON
		}
	} else {
		toolCall.Success = true
		// Convert MCP result to structured output
		var contentParts []string
		for _, content := range result.Content {
			switch c := content.(type) {
			case *mcp.TextContent:
				contentParts = append(contentParts, c.Text)
			case *mcp.ImageContent:
				contentParts = append(contentParts, fmt.Sprintf("[Image: %s]", c.MIMEType))
			case *mcp.EmbeddedResource:
				contentParts = append(contentParts, fmt.Sprintf("[Resource: %s]", c.Resource.URI))
			}
		}
		resultContent := strings.Join(contentParts, "\n")

		// Store as JSON string for trace output
		outputData := map[string]string{"result": resultContent}
		if outputJSON, marshalErr := json.Marshal(outputData); marshalErr == nil {
			toolCall.Output = outputJSON
		}
	}

	return toolCall
}

func (ec *EvalClient) RunEval(ctx context.Context, eval Eval) (*EvalRunResult, error) {
	overallStart := time.Now()
	trace := &EvalTrace{
		Steps: make([]AgenticStep, 0, ec.config.MaxSteps),
	}

	result := &EvalRunResult{
		Eval:  eval,
		Trace: trace,
	}

	session, toolsResp, err := ec.loadMCPSession(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close() }()

	// convert the tools to the format expected by the anthropic model
	toolParams := make([]anthropic.ToolParam, 0, len(toolsResp.Tools))
	for _, tool := range toolsResp.Tools {
		// Convert the MCP tool input schema to Anthropic format
		var properties map[string]any
		if tool.InputSchema != nil {
			// MCP uses JSON Schema, convert to map
			schemaBytes, _ := json.Marshal(tool.InputSchema)
			var schema map[string]any
			if err := json.Unmarshal(schemaBytes, &schema); err == nil {
				if props, ok := schema["properties"].(map[string]any); ok {
					properties = props
				}
			}
		}

		toolParam := anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: properties,
			},
		}
		toolParams = append(toolParams, toolParam)
	}

	// Add cache control to the last tool definition if caching is enabled
	// This creates a cache breakpoint after all tools, maximizing cache reuse
	if ec.cachingEnabled() && len(toolParams) > 0 {
		lastIdx := len(toolParams) - 1
		toolParams[lastIdx].CacheControl = ec.newCacheControl()
	}

	tools := make([]anthropic.ToolUnionParam, len(toolParams))
	for i, toolParam := range toolParams {
		tools[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}

	// Initialize message history
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(eval.Prompt)),
	}

	var finalText strings.Builder

	// Agentic loop with tracing
	stepNumber := 0
	for range ec.config.MaxSteps {
		stepNumber++
		stepStart := time.Now()
		step := AgenticStep{
			StepNumber: stepNumber,
			StartTime:  stepStart,
			ToolCalls:  make([]ToolCall, 0),
		}

		// Build system prompt with optional cache control
		// Precedence: per-eval > client config > default constant
		promptText := AgentSystemPrompt
		if ec.config.AgentSystemPrompt != "" {
			promptText = ec.config.AgentSystemPrompt
		}
		if eval.AgentSystemPrompt != "" {
			promptText = eval.AgentSystemPrompt
		}

		systemPrompt := anthropic.TextBlockParam{
			Text: promptText,
		}
		if ec.cachingEnabled() {
			systemPrompt.CacheControl = ec.newCacheControl()
		}

		stream := ec.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(ec.config.Model),
			MaxTokens: int64(ec.config.MaxTokens),
			System: []anthropic.TextBlockParam{
				systemPrompt,
			},
			Messages: messages,
			Tools:    tools,
		})

		message := anthropic.Message{}

		// Process the stream
		for stream.Next() {
			event := stream.Current()
			if err = message.Accumulate(event); err != nil {
				step.Error = err.Error()
				trace.Steps = append(trace.Steps, step)
				return nil, fmt.Errorf("failed to accumulate event: %w", err)
			}

			if evt, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				finalText.WriteString(evt.Delta.Text)
			}
		}

		if err = stream.Err(); err != nil {
			step.Error = err.Error()
			trace.Steps = append(trace.Steps, step)
			return nil, fmt.Errorf("streaming error: %w", err)
		}

		// Record step data from message
		step.StopReason = string(message.StopReason)
		step.InputTokens = int(message.Usage.InputTokens)
		step.OutputTokens = int(message.Usage.OutputTokens)

		// Capture cache metrics from API response
		step.CacheCreationInputTokens = int(message.Usage.CacheCreationInputTokens)
		step.CacheReadInputTokens = int(message.Usage.CacheReadInputTokens)

		// Extract text content
		for _, block := range message.Content {
			if textBlock, ok := block.AsAny().(anthropic.TextBlock); ok {
				step.ModelResponse += textBlock.Text
			}
		}

		// Add assistant message to history
		messages = append(messages, message.ToParam())

		// Check stop reason
		if message.StopReason == anthropic.StopReasonEndTurn {
			finalizeStep(&step, trace)
			break
		}

		if message.StopReason != anthropic.StopReasonToolUse {
			finalizeStep(&step, trace)
			break
		}

		// Execute tools and collect results
		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range message.Content {
			if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				// Execute and trace tool call
				toolCall := ec.executeAndTraceToolCall(ctx, variant, session)
				step.ToolCalls = append(step.ToolCalls, toolCall)

				// Build result block for message history
				var resultContent string
				if toolCall.Success {
					resultContent = string(toolCall.Output)
				} else {
					resultContent = fmt.Sprintf("Error calling tool: %s", toolCall.Error)
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(
					block.ID,
					resultContent,
					!toolCall.Success,
				))
			}
		}

		finalizeStep(&step, trace)

		// If no tool results, we're done
		if len(toolResults) == 0 {
			break
		}

		// Add tool results to message history
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	// Calculate trace metrics
	trace.StepCount = len(trace.Steps)
	for _, step := range trace.Steps {
		trace.TotalInputTokens += step.InputTokens
		trace.TotalOutputTokens += step.OutputTokens
		trace.ToolCallCount += len(step.ToolCalls)

		// Aggregate cache metrics
		trace.TotalCacheCreationTokens += step.CacheCreationInputTokens
		trace.TotalCacheReadTokens += step.CacheReadInputTokens
	}

	evalResult := &EvalResult{
		Prompt:      eval.Prompt,
		RawResponse: finalText.String(),
	}
	result.Result = evalResult

	// Auto-grade the result with tracing
	grade, gradingTrace, err := ec.gradeWithTrace(ctx, eval, evalResult, trace)
	if err != nil {
		// Don't fail the entire eval if grading fails, just log it
		result.Error = fmt.Errorf("grading failed: %w", err)
		trace.Grading = gradingTrace // Still include partial trace if available
	} else {
		result.Grade = grade
		trace.Grading = gradingTrace

		// Check minimum scores if enforcement is enabled
		if ec.config.EnforceMinimumScores != nil && *ec.config.EnforceMinimumScores {
			if scoreErr := eval.GradingRubric.CheckMinimumScores(grade); scoreErr != nil {
				log.Warn().
					Str("eval", eval.Name).
					Err(scoreErr).
					Msg("Eval failed minimum score requirements")
				result.Error = scoreErr
			}
		}
	}

	// Include grading cache metrics in totals
	if trace.Grading != nil {
		trace.TotalCacheCreationTokens += trace.Grading.CacheCreationInputTokens
		trace.TotalCacheReadTokens += trace.Grading.CacheReadInputTokens
	}

	// Finalize trace timing
	trace.TotalDuration = time.Since(overallStart)

	return result, nil
}

// RunEvals executes multiple evaluations and returns all results.
// Individual eval failures are captured in EvalRunResult.Error and don't stop the batch.
func (ec *EvalClient) RunEvals(ctx context.Context, evals []Eval) ([]EvalRunResult, error) {
	results := make([]EvalRunResult, len(evals))

	for i, eval := range evals {
		result, err := ec.RunEval(ctx, eval)
		if err != nil {
			// Capture error but continue with other evals
			results[i] = EvalRunResult{
				Eval:  eval,
				Error: err,
			}
			continue
		}
		results[i] = *result
	}

	return results, nil
}

// formatDimensionCriteria formats a single dimension's criteria for the grading prompt
func (ec *EvalClient) formatDimensionCriteria(dimension string, criteria *DimensionCriteria) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "### %s\n", dimension)

	if criteria.Description != "" {
		fmt.Fprintf(&sb, "%s\n\n", criteria.Description)
	}

	writeBulletSection(&sb, "**Must have for high scores (4-5):**", criteria.MustHave)
	writeBulletSection(&sb, "**Nice to have:**", criteria.NiceToHave)
	writeBulletSection(&sb, "**Score reductions:**", criteria.Penalties)

	return sb.String()
}

// writeBulletSection writes a markdown bullet list under a header, or nothing if items is empty.
func writeBulletSection(sb *strings.Builder, header string, items []string) {
	if len(items) == 0 {
		return
	}
	sb.WriteString(header + "\n")
	for _, item := range items {
		fmt.Fprintf(sb, "- %s\n", item)
	}
	sb.WriteString("\n")
}

// buildGradingPrompt constructs the full grading prompt including rubric criteria
func (ec *EvalClient) buildGradingPrompt(eval Eval, evalResult *EvalResult, execTrace *EvalTrace) string {
	var prompt strings.Builder

	// Standard context
	fmt.Fprintf(&prompt, "Here is the user input: %s\n", evalResult.Prompt)
	fmt.Fprintf(&prompt, "Here is the LLM's answer: %s\n", evalResult.RawResponse)

	// Add tool execution context
	if execTrace != nil && execTrace.ToolCallCount > 0 {
		prompt.WriteString("\n\nTool Execution Context:\n")
		prompt.WriteString("The LLM had access to and successfully called the following tools to gather information:\n")
		for _, step := range execTrace.Steps {
			for _, toolCall := range step.ToolCalls {
				fmt.Fprintf(&prompt, "\n- Tool: '%s'\n", toolCall.ToolName)
				if toolCall.Success {
					prompt.WriteString("  Status: SUCCESS\n")
					if len(toolCall.Output) > 0 {
						// Include the actual tool output so grader can verify data accuracy
						fmt.Fprintf(&prompt, "  Returned data: %s\n", string(toolCall.Output))
					}
				} else {
					fmt.Fprintf(&prompt, "  Status: FAILED - %s\n", toolCall.Error)
				}
			}
		}
		prompt.WriteString("\nThe LLM's answer should be evaluated based on how well it used this tool-provided data.\n")
	}

	// Add rubric criteria if provided
	if eval.GradingRubric != nil {
		prompt.WriteString("\n\n## Custom Grading Criteria\n\n")
		prompt.WriteString("Use the following specific criteria when scoring this response:\n\n")

		if eval.GradingRubric.Accuracy != nil {
			prompt.WriteString(ec.formatDimensionCriteria("Accuracy", eval.GradingRubric.Accuracy))
		}
		if eval.GradingRubric.Completeness != nil {
			prompt.WriteString(ec.formatDimensionCriteria("Completeness", eval.GradingRubric.Completeness))
		}
		if eval.GradingRubric.Relevance != nil {
			prompt.WriteString(ec.formatDimensionCriteria("Relevance", eval.GradingRubric.Relevance))
		}
		if eval.GradingRubric.Clarity != nil {
			prompt.WriteString(ec.formatDimensionCriteria("Clarity", eval.GradingRubric.Clarity))
		}
		if eval.GradingRubric.Reasoning != nil {
			prompt.WriteString(ec.formatDimensionCriteria("Reasoning", eval.GradingRubric.Reasoning))
		}

		if len(eval.GradingRubric.MinimumScores) > 0 {
			prompt.WriteString("\n### Minimum Acceptable Scores:\n")
			for dim, score := range eval.GradingRubric.MinimumScores {
				fmt.Fprintf(&prompt, "- %s: %d/5\n", dim, score)
			}
		}
	}

	return prompt.String()
}

// gradeWithTrace grades an evaluation result and returns complete trace data
func (ec *EvalClient) gradeWithTrace(ctx context.Context, eval Eval, evalResult *EvalResult, execTrace *EvalTrace) (*GradeResult, *GradingTrace, error) {
	trace := &GradingTrace{
		UserPrompt:     eval.Prompt,
		ModelResponse:  evalResult.RawResponse,
		ExpectedResult: eval.ExpectedResult,
		StartTime:      time.Now(),
	}

	// Build grading prompt with rubric guidance
	gradingPrompt := ec.buildGradingPrompt(eval, evalResult, execTrace)
	trace.GradingPrompt = gradingPrompt

	// Determine which model to use for grading
	gradingModel := ec.config.Model
	if ec.config.GradingModel != "" {
		gradingModel = ec.config.GradingModel
	}

	// Build grading system prompt with optional cache control
	gradingSystemPrompt := anthropic.TextBlockParam{
		Text: EvalSystemPrompt,
	}
	if ec.cachingEnabled() {
		gradingSystemPrompt.CacheControl = ec.newCacheControl()
	}

	// Execute grading
	resp, err := ec.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(gradingModel),
		MaxTokens: 1000,
		System: []anthropic.TextBlockParam{
			gradingSystemPrompt,
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(gradingPrompt)),
		},
	})

	trace.EndTime = time.Now()
	trace.Duration = trace.EndTime.Sub(trace.StartTime)

	if err != nil {
		trace.Error = err.Error()
		return nil, trace, fmt.Errorf("failed to get grading response: %w", err)
	}

	// Capture raw response and token usage
	if len(resp.Content) == 0 {
		trace.Error = "empty grading response"
		return nil, trace, fmt.Errorf("grading response contained no content")
	}
	textBlock, ok := resp.Content[0].AsAny().(anthropic.TextBlock)
	if !ok {
		trace.Error = "unexpected grading response content type"
		return nil, trace, fmt.Errorf("grading response was not a text block")
	}
	rawResponse := textBlock.Text
	trace.RawGradingOutput = rawResponse
	trace.InputTokens = int(resp.Usage.InputTokens)
	trace.OutputTokens = int(resp.Usage.OutputTokens)

	// Capture cache metrics from API response
	trace.CacheCreationInputTokens = int(resp.Usage.CacheCreationInputTokens)
	trace.CacheReadInputTokens = int(resp.Usage.CacheReadInputTokens)

	// Parse grade result
	cleanedResponse, err := extractJSONFromResponse(rawResponse)
	if err != nil {
		trace.Error = err.Error()
		return nil, trace, fmt.Errorf("failed to extract JSON from grading response: %w", err)
	}

	var gradeResult GradeResult
	if err := json.Unmarshal([]byte(cleanedResponse), &gradeResult); err != nil {
		trace.Error = err.Error()
		return nil, trace, fmt.Errorf("failed to parse grading response: %w", err)
	}

	return &gradeResult, trace, nil
}

type EvalResult struct {
	Prompt      string
	RawResponse string
}

type GradeResult struct {
	Accuracy       int    `json:"accuracy"`
	Completeness   int    `json:"completeness"`
	Relevance      int    `json:"relevance"`
	Clarity        int    `json:"clarity"`
	Reasoning      int    `json:"reasoning"`
	OverallComment string `json:"overall_comments"`
}

// Eval represents a single evaluation test case
type Eval struct {
	Name              string         `yaml:"name" json:"name" jsonschema:"Unique identifier for this evaluation"`
	Description       string         `yaml:"description,omitempty" json:"description,omitempty" jsonschema:"Human-readable description of what this eval tests"`
	Prompt            string         `yaml:"prompt" json:"prompt" jsonschema:"The input prompt to send to the LLM"`
	ExpectedResult    string         `yaml:"expected_result,omitempty" json:"expected_result,omitempty" jsonschema:"Expected behavior or result (used for documentation and grading context)"`
	AgentSystemPrompt string         `yaml:"agent_system_prompt,omitempty" json:"agent_system_prompt,omitempty" jsonschema:"Optional custom system prompt for the agent (overrides global default)"`
	GradingRubric     *GradingRubric `yaml:"grading_rubric,omitempty" json:"grading_rubric,omitempty" jsonschema:"Optional custom grading criteria for this evaluation"`
}

// GradingRubric defines specific evaluation criteria for grading
type GradingRubric struct {
	// Optional: Override which dimensions to grade (defaults to all 5 standard dimensions)
	Dimensions []string `yaml:"dimensions,omitempty" json:"dimensions,omitempty" jsonschema:"Which dimensions to grade: accuracy, completeness, relevance, clarity, reasoning"`

	// Criteria for each dimension - what to look for when grading
	Accuracy     *DimensionCriteria `yaml:"accuracy,omitempty" json:"accuracy,omitempty" jsonschema:"Specific criteria for accuracy scoring"`
	Completeness *DimensionCriteria `yaml:"completeness,omitempty" json:"completeness,omitempty" jsonschema:"Specific criteria for completeness scoring"`
	Relevance    *DimensionCriteria `yaml:"relevance,omitempty" json:"relevance,omitempty" jsonschema:"Specific criteria for relevance scoring"`
	Clarity      *DimensionCriteria `yaml:"clarity,omitempty" json:"clarity,omitempty" jsonschema:"Specific criteria for clarity scoring"`
	Reasoning    *DimensionCriteria `yaml:"reasoning,omitempty" json:"reasoning,omitempty" jsonschema:"Specific criteria for reasoning scoring"`

	// Optional: Minimum acceptable scores for pass/fail
	MinimumScores map[string]int `yaml:"minimum_scores,omitempty" json:"minimum_scores,omitempty" jsonschema:"Minimum acceptable score for each dimension (1-5)"`
}

// DimensionCriteria provides specific guidance for grading a dimension
type DimensionCriteria struct {
	Description string   `yaml:"description,omitempty" json:"description,omitempty" jsonschema:"What this dimension means for this specific eval"`
	MustHave    []string `yaml:"must_have,omitempty" json:"must_have,omitempty" jsonschema:"Required elements for high scores (4-5)"`
	NiceToHave  []string `yaml:"nice_to_have,omitempty" json:"nice_to_have,omitempty" jsonschema:"Optional elements that improve scores"`
	Penalties   []string `yaml:"penalties,omitempty" json:"penalties,omitempty" jsonschema:"Elements that reduce scores (errors, omissions, inaccuracies)"`
}

// Validate checks that the rubric is well-formed
func (r *GradingRubric) Validate() error {
	if r == nil {
		return nil // nil rubric is valid (optional field)
	}

	validDimensions := map[string]bool{
		"accuracy": true, "completeness": true,
		"relevance": true, "clarity": true, "reasoning": true,
	}

	// Validate dimensions list if provided
	for _, dim := range r.Dimensions {
		if !validDimensions[dim] {
			return fmt.Errorf("invalid dimension '%s': must be one of: accuracy, completeness, relevance, clarity, reasoning", dim)
		}
	}

	// Validate minimum scores
	for dim, score := range r.MinimumScores {
		if !validDimensions[dim] {
			return fmt.Errorf("invalid dimension in minimum_scores '%s': must be one of: accuracy, completeness, relevance, clarity, reasoning", dim)
		}
		if score < 1 || score > 5 {
			return fmt.Errorf("minimum score for '%s' must be between 1 and 5, got %d", dim, score)
		}
	}

	return nil
}

// CheckMinimumScores verifies that graded scores meet minimum thresholds
func (r *GradingRubric) CheckMinimumScores(grade *GradeResult) error {
	if r == nil || len(r.MinimumScores) == 0 {
		return nil // No minimum scores to enforce
	}

	var failures []string

	for dim, minScore := range r.MinimumScores {
		var actualScore int
		switch dim {
		case "accuracy":
			actualScore = grade.Accuracy
		case "completeness":
			actualScore = grade.Completeness
		case "relevance":
			actualScore = grade.Relevance
		case "clarity":
			actualScore = grade.Clarity
		case "reasoning":
			actualScore = grade.Reasoning
		}

		if actualScore < minScore {
			failures = append(failures, fmt.Sprintf("%s: got %d, required %d", dim, actualScore, minScore))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("eval failed minimum score requirements: %s. Review grading criteria or adjust rubric thresholds", strings.Join(failures, "; "))
	}

	return nil
}

// EvalRunResult combines the eval configuration with its execution results
type EvalRunResult struct {
	Eval   Eval
	Result *EvalResult
	Grade  *GradeResult
	Error  error
	Trace  *EvalTrace // Complete execution trace for debugging and analysis
}

// EvalTrace captures complete execution history of an evaluation run
type EvalTrace struct {
	Steps                    []AgenticStep `json:"steps"`                       // Each step in the agentic loop
	Grading                  *GradingTrace `json:"grading,omitempty"`           // Grading interaction details
	TotalDuration            time.Duration `json:"total_duration"`              // Total execution time
	TotalInputTokens         int           `json:"total_input_tokens"`          // Sum of input tokens across all steps
	TotalOutputTokens        int           `json:"total_output_tokens"`         // Sum of output tokens across all steps
	StepCount                int           `json:"step_count"`                  // Number of agentic steps executed
	ToolCallCount            int           `json:"tool_call_count"`             // Total number of tool calls made
	TotalCacheCreationTokens int           `json:"total_cache_creation_tokens"` // Sum of cache creation tokens across all steps
	TotalCacheReadTokens     int           `json:"total_cache_read_tokens"`     // Sum of cache read tokens across all steps
}

// AgenticStep records a single iteration of the agentic loop
type AgenticStep struct {
	StepNumber               int           `json:"step_number"`                 // 1-indexed step number
	StartTime                time.Time     `json:"start_time"`                  // When this step started
	EndTime                  time.Time     `json:"end_time"`                    // When this step completed
	Duration                 time.Duration `json:"duration"`                    // Step execution duration
	ModelResponse            string        `json:"model_response"`              // Text content from assistant
	StopReason               string        `json:"stop_reason"`                 // end_turn, tool_use, max_tokens, etc.
	ToolCalls                []ToolCall    `json:"tool_calls"`                  // Tools executed in this step
	InputTokens              int           `json:"input_tokens"`                // Input tokens for this step
	OutputTokens             int           `json:"output_tokens"`               // Output tokens for this step
	CacheCreationInputTokens int           `json:"cache_creation_input_tokens"` // Tokens used to create cache
	CacheReadInputTokens     int           `json:"cache_read_input_tokens"`     // Tokens read from cache
	Error                    string        `json:"error,omitempty"`             // Error message if step failed
}

// ToolCall captures details of a single tool invocation
type ToolCall struct {
	ToolID    string          `json:"tool_id"`         // Unique ID from content block
	ToolName  string          `json:"tool_name"`       // MCP tool name
	StartTime time.Time       `json:"start_time"`      // When tool execution started
	EndTime   time.Time       `json:"end_time"`        // When tool execution completed
	Duration  time.Duration   `json:"duration"`        // Tool execution duration
	Input     json.RawMessage `json:"input"`           // Tool arguments as JSON
	Output    json.RawMessage `json:"output"`          // Tool result as JSON
	Success   bool            `json:"success"`         // Whether tool executed successfully
	Error     string          `json:"error,omitempty"` // Error message if tool failed
}

// GradingTrace records the grading interaction with the LLM
type GradingTrace struct {
	UserPrompt               string        `json:"user_prompt"`                 // Original eval prompt
	ModelResponse            string        `json:"model_response"`              // Model's answer being graded
	ExpectedResult           string        `json:"expected_result"`             // Expected result description
	GradingPrompt            string        `json:"grading_prompt"`              // Full prompt sent to grader
	RawGradingOutput         string        `json:"raw_grading_output"`          // Complete LLM response before parsing
	StartTime                time.Time     `json:"start_time"`                  // When grading started
	EndTime                  time.Time     `json:"end_time"`                    // When grading completed
	Duration                 time.Duration `json:"duration"`                    // Grading duration
	InputTokens              int           `json:"input_tokens"`                // Input tokens for grading
	OutputTokens             int           `json:"output_tokens"`               // Output tokens for grading
	CacheCreationInputTokens int           `json:"cache_creation_input_tokens"` // Tokens used to create cache
	CacheReadInputTokens     int           `json:"cache_read_input_tokens"`     // Tokens read from cache
	Error                    string        `json:"error,omitempty"`             // Error message if grading failed
}

// toPtr returns a pointer to the provided value.
// This generic helper simplifies creating pointers to literals or values.
func toPtr[T any](v T) *T {
	return &v
}

// cachingEnabled returns true if prompt caching is enabled in the config.
func (ec *EvalClient) cachingEnabled() bool {
	return ec.config.EnablePromptCaching != nil && *ec.config.EnablePromptCaching
}

// newCacheControl builds an ephemeral cache control param with the configured TTL.
func (ec *EvalClient) newCacheControl() anthropic.CacheControlEphemeralParam {
	cc := anthropic.NewCacheControlEphemeralParam()
	if ec.config.CacheTTL == "1h" {
		cc.TTL = "1h"
	}
	return cc
}

// finalizeStep records the end time, duration, and appends the step to the trace.
func finalizeStep(step *AgenticStep, trace *EvalTrace) {
	step.EndTime = time.Now()
	step.Duration = step.EndTime.Sub(step.StartTime)
	trace.Steps = append(trace.Steps, *step)
}
