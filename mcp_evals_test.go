package evaluations

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const testMCPServerInstructions = "Use the test MCP server tools to answer evaluation prompts. Prefer get_user for user profile questions and get_system_logs for log troubleshooting questions."

func TestEvalClient_loadMCPSession(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		args          []string
		expectedTools []string
		expectError   bool
	}{
		{
			name:    "successfully loads test MCP server",
			command: "go",
			args:    []string{"run", "testdata/mcp-test-server/main.go"},
			expectedTools: []string{
				"add",
				"echo",
				"get_current_time",
				"get_env",
				"get_user",
				"get_system_logs",
			},
			expectError: false,
		},
		{
			name:        "invalid command",
			command:     "nonexistent-command",
			args:        []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := require.New(t)

			client := NewEvalClient(EvalClientConfig{
				Command: tt.command,
				Args:    tt.args,
			})

			ctx := context.Background()
			session, toolsResp, serverInstructions, err := client.loadMCPSession(ctx)

			if tt.expectError {
				assert.Error(err)
				return
			}

			assert.NoError(err)
			defer func() { _ = session.Close() }()

			assert.NotNil(toolsResp)
			assert.Equal(testMCPServerInstructions, serverInstructions)

			// Verify expected tools are present
			toolMap := make(map[string]bool)
			for _, tool := range toolsResp.Tools {
				toolMap[tool.Name] = true
			}

			for _, expectedTool := range tt.expectedTools {
				assert.True(toolMap[expectedTool], "expected tool %q not found in response", expectedTool)
			}

			// Verify we got the correct number of tools
			assert.Len(toolsResp.Tools, len(tt.expectedTools))
		})
	}
}

func TestEvalClient_loadMCPSession_ToolExecution(t *testing.T) {
	assert := require.New(t)

	// Set environment variable for the test
	t.Setenv("TEST_VAR", "test_value")

	client := NewEvalClient(EvalClientConfig{
		Command: "go",
		Args:    []string{"run", "testdata/mcp-test-server/main.go"},
	})

	ctx := context.Background()
	session, _, _, err := client.loadMCPSession(ctx)
	assert.NoError(err)
	defer func() { _ = session.Close() }()

	// Test calling the get_env tool
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_env",
		Arguments: map[string]any{
			"name": "TEST_VAR",
		},
	})
	assert.NoError(err)
	assert.NotEmpty(result.Content)

	// Parse the response
	textContent, ok := result.Content[0].(*mcp.TextContent)
	assert.True(ok, "expected text content but got %T", result.Content[0])

	var output struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Set   bool   `json:"set"`
	}
	err = json.Unmarshal([]byte(textContent.Text), &output)
	assert.NoError(err)

	// Verify the response
	assert.Equal("TEST_VAR", output.Name)
	assert.Equal("test_value", output.Value)
	assert.True(output.Set)
}

func TestEvalClient_loadMCPSession_CustomEnv(t *testing.T) {
	assert := require.New(t)

	// Test that custom environment variables are added while preserving parent env
	client := NewEvalClient(EvalClientConfig{
		Command: "go",
		Args:    []string{"run", "testdata/mcp-test-server/main.go"},
		Env:     []string{"CUSTOM_TEST_VAR=custom_value"},
	})

	ctx := context.Background()
	session, _, _, err := client.loadMCPSession(ctx)
	assert.NoError(err)
	defer func() { _ = session.Close() }()

	// Test that custom env var works
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_env",
		Arguments: map[string]any{
			"name": "CUSTOM_TEST_VAR",
		},
	})
	assert.NoError(err)

	textContent, ok := result.Content[0].(*mcp.TextContent)
	assert.True(ok, "expected text content but got %T", result.Content[0])

	var output struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Set   bool   `json:"set"`
	}
	err = json.Unmarshal([]byte(textContent.Text), &output)
	assert.NoError(err)

	assert.Equal("custom_value", output.Value)
	assert.True(output.Set)

	// Test that parent env vars are still available (e.g., PATH)
	result2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_env",
		Arguments: map[string]any{
			"name": "PATH",
		},
	})
	assert.NoError(err)

	textContent2, ok := result2.Content[0].(*mcp.TextContent)
	assert.True(ok, "expected text content but got %T", result2.Content[0])

	var output2 struct {
		Name  string `json:"name"`
		Value string `json:"value"`
		Set   bool   `json:"set"`
	}
	err = json.Unmarshal([]byte(textContent2.Text), &output2)
	assert.NoError(err)

	// PATH should be inherited from parent environment
	assert.True(output2.Set)
	assert.NotEmpty(output2.Value)
}

func TestEvalClient_buildAgentSystemPrompt(t *testing.T) {
	tests := []struct {
		name                 string
		configPrompt         string
		evalPrompt           string
		serverInstructions   string
		expectedBlocks       int
		expectedBasePrompt   string
		expectedInstructions string
	}{
		{
			name:               "default prompt without server instructions",
			expectedBlocks:     1,
			expectedBasePrompt: AgentSystemPrompt,
		},
		{
			name:                 "adds server instructions as second block",
			serverInstructions:   "Prefer the search tool first.",
			expectedBlocks:       2,
			expectedBasePrompt:   AgentSystemPrompt,
			expectedInstructions: "MCP server instructions:\nPrefer the search tool first.",
		},
		{
			name:               "uses client config prompt",
			configPrompt:       "Use the configured prompt.",
			expectedBlocks:     1,
			expectedBasePrompt: "Use the configured prompt.",
		},
		{
			name:               "eval prompt overrides client config prompt",
			configPrompt:       "Use the configured prompt.",
			evalPrompt:         "Use the eval prompt.",
			expectedBlocks:     1,
			expectedBasePrompt: "Use the eval prompt.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := require.New(t)

			client := NewEvalClient(EvalClientConfig{
				Model:             "test",
				AgentSystemPrompt: tt.configPrompt,
			})

			systemPrompt := client.buildAgentSystemPrompt(Eval{
				AgentSystemPrompt: tt.evalPrompt,
			}, tt.serverInstructions)

			assert.Len(systemPrompt, tt.expectedBlocks)
			assert.Equal(tt.expectedBasePrompt, systemPrompt[0].Text)

			lastIdx := len(systemPrompt) - 1
			assert.NotZero(systemPrompt[lastIdx].CacheControl)
			if len(systemPrompt) > 1 {
				assert.Zero(systemPrompt[0].CacheControl)
				assert.Equal(tt.expectedInstructions, systemPrompt[1].Text)
			}
		})
	}
}

func TestGradingRubricParsing(t *testing.T) {
	assert := require.New(t)

	yamlData := `
name: test_eval
prompt: test prompt
grading_rubric:
  dimensions:
    - accuracy
    - completeness
  accuracy:
    description: "Test accuracy description"
    must_have:
      - "item 1"
      - "item 2"
    nice_to_have:
      - "nice item 1"
    penalties:
      - "penalty 1"
  completeness:
    must_have:
      - "complete item"
  minimum_scores:
    accuracy: 4
    completeness: 3
`

	var eval Eval
	err := yaml.Unmarshal([]byte(yamlData), &eval)
	assert.NoError(err)
	assert.NotNil(eval.GradingRubric)

	// Test dimensions
	assert.Len(eval.GradingRubric.Dimensions, 2)
	assert.Equal("accuracy", eval.GradingRubric.Dimensions[0])
	assert.Equal("completeness", eval.GradingRubric.Dimensions[1])

	// Test accuracy criteria
	assert.NotNil(eval.GradingRubric.Accuracy)
	assert.Equal("Test accuracy description", eval.GradingRubric.Accuracy.Description)
	assert.Len(eval.GradingRubric.Accuracy.MustHave, 2)
	assert.Equal("item 1", eval.GradingRubric.Accuracy.MustHave[0])
	assert.Equal("item 2", eval.GradingRubric.Accuracy.MustHave[1])
	assert.Len(eval.GradingRubric.Accuracy.NiceToHave, 1)
	assert.Equal("nice item 1", eval.GradingRubric.Accuracy.NiceToHave[0])
	assert.Len(eval.GradingRubric.Accuracy.Penalties, 1)
	assert.Equal("penalty 1", eval.GradingRubric.Accuracy.Penalties[0])

	// Test completeness criteria
	assert.NotNil(eval.GradingRubric.Completeness)
	assert.Len(eval.GradingRubric.Completeness.MustHave, 1)

	// Test minimum scores
	assert.Equal(4, eval.GradingRubric.MinimumScores["accuracy"])
	assert.Equal(3, eval.GradingRubric.MinimumScores["completeness"])
}

func TestGradingRubricParsingWithoutRubric(t *testing.T) {
	assert := require.New(t)

	yamlData := `
name: test_eval
prompt: test prompt
expected_result: test expected result
`

	var eval Eval
	err := yaml.Unmarshal([]byte(yamlData), &eval)
	assert.NoError(err)
	assert.Nil(eval.GradingRubric)
	assert.Equal("test_eval", eval.Name)
	assert.Equal("test prompt", eval.Prompt)
	assert.Equal("test expected result", eval.ExpectedResult)
}

func TestGradingRubricJSONMarshal(t *testing.T) {
	assert := require.New(t)

	eval := Eval{
		Name:   "test",
		Prompt: "test prompt",
		GradingRubric: &GradingRubric{
			Dimensions: []string{"accuracy"},
			Accuracy: &DimensionCriteria{
				Description: "Test description",
				MustHave:    []string{"item 1"},
			},
			MinimumScores: map[string]int{"accuracy": 5},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(eval)
	assert.NoError(err)

	// Unmarshal back
	var decoded Eval
	err = json.Unmarshal(jsonData, &decoded)
	assert.NoError(err)

	// Verify fields
	assert.Equal("test", decoded.Name)
	assert.NotNil(decoded.GradingRubric)
	assert.Len(decoded.GradingRubric.Dimensions, 1)
	assert.Equal("accuracy", decoded.GradingRubric.Dimensions[0])
	assert.NotNil(decoded.GradingRubric.Accuracy)
	assert.Equal("Test description", decoded.GradingRubric.Accuracy.Description)
	assert.Equal(5, decoded.GradingRubric.MinimumScores["accuracy"])
}

func TestFormatDimensionCriteria(t *testing.T) {
	assert := require.New(t)

	client := NewEvalClient(EvalClientConfig{Model: "test"})

	criteria := &DimensionCriteria{
		Description: "Test description",
		MustHave:    []string{"must 1", "must 2"},
		NiceToHave:  []string{"nice 1"},
		Penalties:   []string{"penalty 1"},
	}

	result := client.formatDimensionCriteria("Accuracy", criteria)

	assert.Contains(result, "### Accuracy")
	assert.Contains(result, "Test description")
	assert.Contains(result, "must 1")
	assert.Contains(result, "must 2")
	assert.Contains(result, "nice 1")
	assert.Contains(result, "penalty 1")
	assert.Contains(result, "Must have for high scores")
	assert.Contains(result, "Nice to have")
	assert.Contains(result, "Score reductions")
}

func TestBuildGradingPromptWithRubric(t *testing.T) {
	assert := require.New(t)

	client := NewEvalClient(EvalClientConfig{Model: "test"})

	eval := Eval{
		Prompt: "test prompt",
		GradingRubric: &GradingRubric{
			Accuracy: &DimensionCriteria{
				MustHave: []string{"criterion 1", "criterion 2"},
			},
			MinimumScores: map[string]int{"accuracy": 4},
		},
	}

	evalResult := &EvalResult{
		Prompt:      "test prompt",
		RawResponse: "test response",
	}

	prompt := client.buildGradingPrompt(eval, evalResult, nil)

	assert.Contains(prompt, "Custom Grading Criteria")
	assert.Contains(prompt, "criterion 1")
	assert.Contains(prompt, "criterion 2")
	assert.Contains(prompt, "Minimum Acceptable Scores")
	assert.Contains(prompt, "accuracy: 4/5")
}

func TestBuildGradingPromptWithoutRubric(t *testing.T) {
	assert := require.New(t)

	client := NewEvalClient(EvalClientConfig{Model: "test"})

	eval := Eval{
		Prompt:        "test prompt",
		GradingRubric: nil, // No rubric
	}

	evalResult := &EvalResult{
		Prompt:      "test prompt",
		RawResponse: "test response",
	}

	prompt := client.buildGradingPrompt(eval, evalResult, nil)

	assert.Contains(prompt, "test prompt")
	assert.Contains(prompt, "test response")
	assert.NotContains(prompt, "Custom Grading Criteria")
}

func TestBuildGradingPromptWithToolContext(t *testing.T) {
	assert := require.New(t)

	client := NewEvalClient(EvalClientConfig{Model: "test"})

	eval := Eval{
		Prompt: "test prompt",
	}

	evalResult := &EvalResult{
		Prompt:      "test prompt",
		RawResponse: "test response",
	}

	execTrace := &EvalTrace{
		ToolCallCount: 2,
		Steps: []AgenticStep{
			{
				ToolCalls: []ToolCall{
					{
						ToolName: "test_tool",
						Success:  true,
						Output:   []byte(`{"result":"test output"}`),
					},
				},
			},
		},
	}

	prompt := client.buildGradingPrompt(eval, evalResult, execTrace)

	assert.Contains(prompt, "Tool Execution Context")
	assert.Contains(prompt, "test_tool")
	assert.Contains(prompt, "SUCCESS")
	assert.Contains(prompt, "test output")
}

func TestGradingRubricValidate(t *testing.T) {
	tests := []struct {
		name      string
		rubric    *GradingRubric
		wantError bool
		errorMsg  string
	}{
		{
			name:      "nil rubric is valid",
			rubric:    nil,
			wantError: false,
		},
		{
			name: "empty rubric is valid",
			rubric: &GradingRubric{
				Dimensions:    []string{},
				MinimumScores: map[string]int{},
			},
			wantError: false,
		},
		{
			name: "valid dimensions",
			rubric: &GradingRubric{
				Dimensions: []string{"accuracy", "completeness", "relevance", "clarity", "reasoning"},
			},
			wantError: false,
		},
		{
			name: "invalid dimension in list",
			rubric: &GradingRubric{
				Dimensions: []string{"accuracy", "invalid_dimension"},
			},
			wantError: true,
			errorMsg:  "invalid dimension 'invalid_dimension'",
		},
		{
			name: "valid minimum scores",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy":     5,
					"completeness": 4,
					"relevance":    3,
					"clarity":      2,
					"reasoning":    1,
				},
			},
			wantError: false,
		},
		{
			name: "invalid dimension in minimum scores",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy":          5,
					"invalid_dimension": 4,
				},
			},
			wantError: true,
			errorMsg:  "invalid dimension in minimum_scores 'invalid_dimension'",
		},
		{
			name: "minimum score too low",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy": 0,
				},
			},
			wantError: true,
			errorMsg:  "minimum score for 'accuracy' must be between 1 and 5, got 0",
		},
		{
			name: "minimum score too high",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy": 6,
				},
			},
			wantError: true,
			errorMsg:  "minimum score for 'accuracy' must be between 1 and 5, got 6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := require.New(t)

			err := tt.rubric.Validate()

			if tt.wantError {
				assert.Error(err)
				assert.Contains(err.Error(), tt.errorMsg)
			} else {
				assert.NoError(err)
			}
		})
	}
}

func TestGradingRubricCheckMinimumScores(t *testing.T) {
	tests := []struct {
		name      string
		rubric    *GradingRubric
		grade     *GradeResult
		wantError bool
		errorMsg  string
	}{
		{
			name:      "nil rubric passes",
			rubric:    nil,
			grade:     &GradeResult{Accuracy: 1, Completeness: 1},
			wantError: false,
		},
		{
			name:      "empty minimum scores passes",
			rubric:    &GradingRubric{MinimumScores: map[string]int{}},
			grade:     &GradeResult{Accuracy: 1, Completeness: 1},
			wantError: false,
		},
		{
			name: "all scores meet minimum",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy":     3,
					"completeness": 3,
					"relevance":    3,
				},
			},
			grade: &GradeResult{
				Accuracy:     5,
				Completeness: 4,
				Relevance:    3,
			},
			wantError: false,
		},
		{
			name: "accuracy below minimum",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy": 4,
				},
			},
			grade: &GradeResult{
				Accuracy: 3,
			},
			wantError: true,
			errorMsg:  "accuracy: got 3, required 4",
		},
		{
			name: "multiple scores below minimum",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy":     4,
					"completeness": 3,
					"clarity":      5,
				},
			},
			grade: &GradeResult{
				Accuracy:     2,
				Completeness: 3,
				Clarity:      4,
			},
			wantError: true,
			errorMsg:  "accuracy: got 2, required 4",
		},
		{
			name: "edge case - exactly at minimum",
			rubric: &GradingRubric{
				MinimumScores: map[string]int{
					"accuracy": 3,
				},
			},
			grade: &GradeResult{
				Accuracy: 3,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := require.New(t)

			err := tt.rubric.CheckMinimumScores(tt.grade)

			if tt.wantError {
				assert.Error(err)
				assert.Contains(err.Error(), tt.errorMsg)
			} else {
				assert.NoError(err)
			}
		})
	}
}
