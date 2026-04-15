package evaluations

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig_YAML(t *testing.T) {
	t.Parallel()
	assert := require.New(t)

	config, err := LoadConfig("testdata/mcp-test-evals.yaml")
	assert.NoError(err)

	// Verify basic fields
	assert.Equal("claude-sonnet-4-6", config.Model)
	assert.Equal("2m", config.Timeout)
	assert.EqualValues(10, config.MaxSteps)
	assert.EqualValues(4096, config.MaxTokens)

	// Verify MCP server config
	assert.Equal("go", config.MCPServer.Command)
	assert.Len(config.MCPServer.Args, 2)
	assert.Len(config.MCPServer.Env, 1)

	// Verify evals
	assert.Len(config.Evals, 4)

	firstEval := config.Evals[0]
	assert.Equal("add", firstEval.Name)
	assert.Equal("What is 5 plus 3?", firstEval.Prompt)
	assert.Equal("Should return 8", firstEval.ExpectedResult)

	// Verify the troubleshooting eval with grading rubric
	troubleshootEval := config.Evals[3]
	assert.Equal("troubleshoot_service_outage", troubleshootEval.Name)
	assert.NotNil(troubleshootEval.GradingRubric)
	assert.Len(troubleshootEval.GradingRubric.Dimensions, 3)
	assert.Contains(troubleshootEval.GradingRubric.Dimensions, "accuracy")
	assert.Contains(troubleshootEval.GradingRubric.Dimensions, "completeness")
	assert.Contains(troubleshootEval.GradingRubric.Dimensions, "reasoning")
	assert.NotNil(troubleshootEval.GradingRubric.Accuracy)
	assert.NotNil(troubleshootEval.GradingRubric.Completeness)
	assert.NotNil(troubleshootEval.GradingRubric.Reasoning)
	assert.Equal(4, troubleshootEval.GradingRubric.MinimumScores["accuracy"])
	assert.Equal(4, troubleshootEval.GradingRubric.MinimumScores["completeness"])
	assert.Equal(3, troubleshootEval.GradingRubric.MinimumScores["reasoning"])
}

func TestLoadConfig_InvalidFile(t *testing.T) {
	assert := require.New(t)

	_, err := LoadConfig("testdata/nonexistent.yaml")
	assert.Error(err)
}

func TestLoadConfig_InvalidExtension(t *testing.T) {
	assert := require.New(t)

	_, err := LoadConfig("testdata/test.txt")
	assert.Error(err)
	assert.Contains(err.Error(), "unsupported file extension")
}

func TestEvalClientConfig_Defaults(t *testing.T) {
	tests := []struct {
		name           string
		config         EvalClientConfig
		expectedSteps  int
		expectedTokens int
	}{
		{
			name: "applies defaults when not set",
			config: EvalClientConfig{
				Command: "echo",
				Model:   "test-model",
			},
			expectedSteps:  10,
			expectedTokens: 4096,
		},
		{
			name: "applies defaults when zero",
			config: EvalClientConfig{
				Command:   "echo",
				Model:     "test-model",
				MaxSteps:  0,
				MaxTokens: 0,
			},
			expectedSteps:  10,
			expectedTokens: 4096,
		},
		{
			name: "respects custom values",
			config: EvalClientConfig{
				Command:   "echo",
				Model:     "test-model",
				MaxSteps:  5,
				MaxTokens: 2048,
			},
			expectedSteps:  5,
			expectedTokens: 2048,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			assert := require.New(t)

			client := NewEvalClient(tt.config)

			assert.Equal(tt.expectedSteps, client.config.MaxSteps)
			assert.Equal(tt.expectedTokens, client.config.MaxTokens)
		})
	}
}

func TestSchemaForEvalConfig(t *testing.T) {
	assert := require.New(t)

	schema, err := SchemaForEvalConfig()
	assert.NoError(err)
	assert.NotEmpty(schema)

	t.Log("Generated JSON Schema:\n", schema)

	// Verify it's valid JSON
	var schemaMap map[string]any
	err = json.Unmarshal([]byte(schema), &schemaMap)
	assert.NoError(err)

	// Verify top-level metadata fields
	schemaURL, ok := schemaMap["$schema"].(string)
	assert.True(ok)
	assert.Equal("https://json-schema.org/draft/2020-12/schema", schemaURL)

	title, ok := schemaMap["title"].(string)
	assert.True(ok)
	assert.Equal("MCP Evaluation Configuration", title)

	desc, ok := schemaMap["description"].(string)
	assert.True(ok)
	assert.NotEmpty(desc)

	// Verify it has expected JSON schema fields
	_, ok = schemaMap["properties"]
	assert.True(ok)

	// Verify it contains expected EvalConfig properties
	properties, ok := schemaMap["properties"].(map[string]any)
	assert.True(ok)

	expectedProperties := []string{"model", "grading_model", "timeout", "max_steps", "max_tokens", "mcp_server", "evals"}
	for _, prop := range expectedProperties {
		assert.Contains(properties, prop)
	}

	// Verify descriptions are present on key fields
	testCases := []struct {
		path        []string
		description string
	}{
		{[]string{"model", "description"}, "Anthropic model ID"},
		{[]string{"timeout", "description"}, "Timeout duration"},
		{[]string{"mcp_server", "description"}, "Configuration for the MCP server"},
		{[]string{"evals", "description"}, "List of evaluation test cases"},
	}

	for _, tc := range testCases {
		var current any = properties
		for i, key := range tc.path {
			m, ok := current.(map[string]any)
			assert.True(ok, "path %v: expected map at level %d, got %T", tc.path, i, current)
			current, ok = m[key]
			assert.True(ok, "path %v: key %q not found", tc.path, key)
		}
		if desc, ok := current.(string); ok {
			assert.Contains(desc, tc.description)
		}
	}
}

func TestValidateConfigFile_Valid(t *testing.T) {
	assert := require.New(t)

	result, err := ValidateConfigFile("testdata/mcp-test-evals.yaml")
	assert.NoError(err)
	assert.True(result.Valid)
	assert.Empty(result.Errors)
}

func TestValidateConfigFile_MissingModel(t *testing.T) {
	assert := require.New(t)

	configContent := `
mcp_server:
  command: echo
  args: ["test"]

evals:
  - name: test
    prompt: "test prompt"
`

	tmpFile, err := os.CreateTemp("", "invalid-missing-model-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	result, err := ValidateConfigFile(tmpFile.Name())
	assert.NoError(err)
	assert.False(result.Valid)
	assert.NotEmpty(result.Errors)

	t.Logf("Got %d validation errors (expected)", len(result.Errors))
	for _, verr := range result.Errors {
		t.Logf("  - [%s] %s", verr.Path, verr.Message)
	}
}

func TestValidateConfigFile_MissingMCPServerCommand(t *testing.T) {
	assert := require.New(t)

	configContent := `
model: claude-sonnet-4-6

mcp_server:
  args: ["test"]

evals:
  - name: test
    prompt: "test prompt"
`

	tmpFile, err := os.CreateTemp("", "invalid-missing-command-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	result, err := ValidateConfigFile(tmpFile.Name())
	assert.NoError(err)
	assert.False(result.Valid)
	assert.NotEmpty(result.Errors)

	t.Logf("Got %d validation errors (expected)", len(result.Errors))
	for _, verr := range result.Errors {
		t.Logf("  - [%s] %s", verr.Path, verr.Message)
	}
}

func TestValidateConfigFile_MissingEvals(t *testing.T) {
	assert := require.New(t)

	configContent := `
model: claude-sonnet-4-6

mcp_server:
  command: echo
  args: ["test"]
`

	tmpFile, err := os.CreateTemp("", "invalid-missing-evals-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	result, err := ValidateConfigFile(tmpFile.Name())
	assert.NoError(err)
	assert.False(result.Valid)
	assert.NotEmpty(result.Errors)

	t.Logf("Got %d validation errors (expected)", len(result.Errors))
	for _, verr := range result.Errors {
		t.Logf("  - [%s] %s", verr.Path, verr.Message)
	}
}

func TestValidateConfigFile_InvalidEval(t *testing.T) {
	assert := require.New(t)

	configContent := `
model: claude-sonnet-4-6

mcp_server:
  command: echo
  args: ["test"]

evals:
  - name: test
    # missing prompt
`

	tmpFile, err := os.CreateTemp("", "invalid-eval-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	result, err := ValidateConfigFile(tmpFile.Name())
	assert.NoError(err)
	assert.False(result.Valid)
	assert.NotEmpty(result.Errors)

	t.Logf("Got %d validation errors (expected)", len(result.Errors))
	for _, verr := range result.Errors {
		t.Logf("  - [%s] %s", verr.Path, verr.Message)
	}
}

func TestValidateConfigFile_InvalidFileExtension(t *testing.T) {
	assert := require.New(t)

	tmpFile, err := os.CreateTemp("", "config-*.txt")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("test")
	assert.NoError(err)
	tmpFile.Close()

	_, err = ValidateConfigFile(tmpFile.Name())
	assert.Error(err)
	assert.Contains(err.Error(), "unsupported file extension")
}

func TestValidateConfigFile_NonExistentFile(t *testing.T) {
	assert := require.New(t)

	_, err := ValidateConfigFile("nonexistent-file.yaml")
	assert.Error(err)
}

func TestValidateConfigFile_InvalidYAML(t *testing.T) {
	assert := require.New(t)

	invalidYAML := `
model: claude
  invalid: indentation
    wrong: level
`

	tmpFile, err := os.CreateTemp("", "invalid-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(invalidYAML)
	assert.NoError(err)
	tmpFile.Close()

	_, err = ValidateConfigFile(tmpFile.Name())
	assert.Error(err)
}

func TestValidateConfigFile_JSONFormat(t *testing.T) {
	assert := require.New(t)

	configContent := `{
  "model": "claude-sonnet-4-6",
  "mcp_server": {
    "command": "echo",
    "args": ["test"]
  },
  "evals": [
    {
      "name": "test",
      "prompt": "test prompt"
    }
  ]
}`

	tmpFile, err := os.CreateTemp("", "valid-*.json")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	result, err := ValidateConfigFile(tmpFile.Name())
	assert.NoError(err)
	assert.True(result.Valid)
}

func TestLoadConfig_EnvironmentVariableExpansion(t *testing.T) {
	assert := require.New(t)

	// Set test environment variables
	t.Setenv("TEST_COMMAND", "/usr/bin/test-server")
	t.Setenv("TEST_PORT", "8080")
	t.Setenv("TEST_VERSION", "1.0.0")
	t.Setenv("TEST_TOKEN", "secret-token-123")

	// Create temporary config file with env vars
	configContent := `
model: claude-sonnet-4-6
mcp_server:
  command: ${TEST_COMMAND}
  args:
    - --port=${TEST_PORT}
  env:
    - VERSION=${TEST_VERSION}
    - TOKEN=${TEST_TOKEN}
evals:
  - name: test
    prompt: "Version is ${TEST_VERSION}"
    expected_result: "Should report ${TEST_VERSION}"
`

	tmpFile, err := os.CreateTemp("", "config-envvar-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	// Load and verify expansion
	config, err := LoadConfig(tmpFile.Name())
	assert.NoError(err)
	assert.Equal("/usr/bin/test-server", config.MCPServer.Command)
	assert.Equal([]string{"--port=8080"}, config.MCPServer.Args)
	assert.Contains(config.MCPServer.Env, "VERSION=1.0.0")
	assert.Contains(config.MCPServer.Env, "TOKEN=secret-token-123")
	assert.Contains(config.Evals[0].Prompt, "Version is 1.0.0")
	assert.Contains(config.Evals[0].ExpectedResult, "Should report 1.0.0")
}

func TestLoadConfig_UndefinedEnvironmentVariable(t *testing.T) {
	assert := require.New(t)

	configContent := `
model: claude-sonnet-4-6
mcp_server:
  command: ${UNDEFINED_VAR}server
  args: []
evals:
  - name: test
    prompt: "test ${UNDEFINED_VAR}value"
`

	tmpFile, err := os.CreateTemp("", "config-undefined-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	assert.NoError(err)
	assert.Equal("server", config.MCPServer.Command) // Undefined var expands to empty string
	assert.Equal("test value", config.Evals[0].Prompt)
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	assert := require.New(t)

	t.Setenv("CUSTOM_PORT", "9000")

	configContent := `
model: claude-sonnet-4-6
mcp_server:
  command: server
  args:
    - --port=${CUSTOM_PORT:-8080}
    - --host=${UNDEFINED_HOST:-localhost}
evals:
  - name: test
    prompt: "test"
`

	tmpFile, err := os.CreateTemp("", "config-defaults-*.yaml")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	assert.NoError(err)
	assert.Equal([]string{"--port=9000", "--host=localhost"}, config.MCPServer.Args)
}

func TestLoadConfig_EnvironmentVariableInJSON(t *testing.T) {
	assert := require.New(t)

	t.Setenv("TEST_MODEL", "claude-sonnet-4-6")
	t.Setenv("TEST_SERVER", "/path/to/server")

	configContent := `{
  "model": "${TEST_MODEL}",
  "mcp_server": {
    "command": "${TEST_SERVER}",
    "args": []
  },
  "evals": [
    {
      "name": "test",
      "prompt": "test prompt"
    }
  ]
}`

	tmpFile, err := os.CreateTemp("", "config-envvar-*.json")
	assert.NoError(err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configContent)
	assert.NoError(err)
	tmpFile.Close()

	config, err := LoadConfig(tmpFile.Name())
	assert.NoError(err)
	assert.Equal("claude-sonnet-4-6", config.Model)
	assert.Equal("/path/to/server", config.MCPServer.Command)
}
