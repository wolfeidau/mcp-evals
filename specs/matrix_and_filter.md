# Matrix Testing and Eval Filtering

## Overview

This specification describes two new features for mcp-evals that enable testing across multiple MCP server versions and selective eval execution:

1. **Environment Variable Interpolation**: Replace placeholders in configuration files with environment variable values
2. **Eval Pattern Filtering**: Run a subset of evals using regex pattern matching

## Customer Problem

**Matrix Testing**: Customers need to validate their MCP server implementations across multiple releases to ensure backward compatibility and catch regressions. Currently, they must maintain separate configuration files for each version, creating maintenance overhead and risk of configuration drift.

**Selective Testing**: During development, customers want to run specific evals without executing the entire suite. This accelerates iteration cycles and reduces API costs when debugging or developing new evaluation scenarios.

## Environment Variable Interpolation

### Requirements

- Support `${VAR_NAME}` and `$VAR_NAME` syntax in YAML/JSON configuration files
- Apply interpolation to all string fields including:
  - `mcp_server.command`
  - `mcp_server.args` (array elements)
  - `mcp_server.env` (environment variable values)
  - `evals[].prompt`
  - `evals[].expected_result`
  - Any other string fields
- Use standard Go `os.ExpandEnv()` behavior for consistency with shell environments
- Fail gracefully if required environment variables are unset (expand to empty string per Go convention)

### Implementation Details

**File**: `mcp_eval_config.go`

Modify the `LoadConfig()` function to expand environment variables before unmarshaling using `mvdan.cc/sh/v3/shell`:

```go
import (
    "mvdan.cc/sh/v3/shell"
    // ... other imports
)

func LoadConfig(filePath string) (*EvalConfig, error) {
    data, err := os.ReadFile(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    // Expand environment variables using shell expansion
    // Supports ${VAR}, $VAR, and ${VAR:-default} syntax
    expandedStr, err := shell.Expand(string(data), nil)
    if err != nil {
        return nil, fmt.Errorf("failed to expand environment variables: %w", err)
    }
    expandedData := []byte(expandedStr)

    var config EvalConfig
    ext := strings.ToLower(filepath.Ext(filePath))

    switch ext {
    case ".yaml", ".yml":
        if err := yaml.Unmarshal(expandedData, &config); err != nil {
            return nil, fmt.Errorf("failed to parse YAML config: %w", err)
        }
    // ... rest of parsing logic
}
```

**Dependencies**: Requires `go get mvdan.cc/sh/v3/shell`

### Usage Examples

**Configuration file** (`matrix.yaml`):
```yaml
model: claude-sonnet-4-6

mcp_server:
  command: ${MCP_SERVER_PATH}
  args:
    - --port=${SERVER_PORT:-8080}
  env:
    - API_TOKEN=${TEST_API_TOKEN}
    - VERSION=${SERVER_VERSION}

evals:
  - name: version_check
    prompt: "What version are you running?"
    expected_result: "Should report ${SERVER_VERSION}"
```

**Running against multiple versions**:
```bash
# Test v1.0.0
export MCP_SERVER_PATH=/releases/v1.0.0/mcp-server
export SERVER_VERSION=1.0.0
mcp-evals run --config matrix.yaml --trace-dir traces/v1.0.0

# Test v2.0.0
export MCP_SERVER_PATH=/releases/v2.0.0/mcp-server
export SERVER_VERSION=2.0.0
mcp-evals run --config matrix.yaml --trace-dir traces/v2.0.0
```

**CI/CD matrix strategy**:
```yaml
# GitHub Actions example
strategy:
  matrix:
    version: [1.0.0, 1.1.0, 2.0.0]
steps:
  - name: Run evals for ${{ matrix.version }}
    env:
      MCP_SERVER_PATH: /releases/${{ matrix.version }}/server
      SERVER_VERSION: ${{ matrix.version }}
    run: mcp-evals run --config matrix.yaml --trace-dir traces/${{ matrix.version }}
```

**Buildkite Pipeline matrix strategy**:
```yaml
# .buildkite/pipeline.yml
steps:
  - label: ":test_tube: MCP Evals - {{matrix.version}}"
    command: |
      echo "Testing MCP server version {{matrix.version}}"
      export MCP_SERVER_PATH=/releases/{{matrix.version}}/mcp-server
      export SERVER_VERSION={{matrix.version}}
      mcp-evals run --config matrix.yaml --trace-dir traces/{{matrix.version}}
    matrix:
      setup:
        version:
          - "1.0.0"
          - "1.1.0"
          - "2.0.0"
    artifact_paths:
      - "traces/{{matrix.version}}/*.json"
```

**Advanced Buildkite example with multiple dimensions**:
```yaml
# Test across versions, models, and configurations
steps:
  - label: ":test_tube: {{matrix.version}} / {{matrix.model}}"
    command: |
      export MCP_SERVER_PATH=/releases/{{matrix.version}}/mcp-server
      export SERVER_VERSION={{matrix.version}}
      export TEST_MODEL={{matrix.model}}
      mcp-evals run \
        --config matrix.yaml \
        --filter "{{matrix.suite}}" \
        --trace-dir traces/{{matrix.version}}/{{matrix.model}}/{{matrix.suite}}
    matrix:
      setup:
        version:
          - "1.0.0"
          - "2.0.0"
        model:
          - "claude-sonnet-4-6"
          - "claude-sonnet-4-5-20250929"
        suite:
          - "auth.*"
          - "api.*"
          - "integration.*"
    artifact_paths:
      - "traces/**/*.json"
```

This advanced example creates a comprehensive test matrix:
- 2 server versions × 2 models × 3 test suites = 12 parallel jobs
- Each job runs filtered eval suites against specific version/model combinations
- Traces are organized by version, model, and suite for post-build analysis

### Edge Cases

- **Undefined variables**: Expand to empty string (standard shell expansion behavior)
- **Default values**: Support shell-style `${VAR:-default}` syntax via `mvdan.cc/sh/v3/shell`
- **Other shell expansions**: Also supports `${VAR:+value}` (use value if VAR is set), `${VAR:?error}` (error if unset)
- **Literal dollar signs**: Use `$$` to escape
- **Nested structures**: Expansion works recursively through all string values

### Security Considerations

- Environment variables may contain secrets (API keys, tokens)
- Configuration files with env vars should be treated as templates, not production configs
- Trace files will contain expanded values - ensure trace directories have appropriate permissions
- Document that sensitive values should use environment variables rather than hardcoding

## Eval Pattern Filtering

### Requirements

- Add `--filter` flag to the `run` command
- Accept a regular expression pattern to match against eval names
- Skip evals that don't match the pattern
- Display clear messaging about which evals are being run vs skipped
- Exit with error if no evals match the filter pattern
- Filter is optional - omitting it runs all evals (current behavior)

### Implementation Details

**File**: `internal/commands/run.go`

Add filter flag to `RunCmd`:
```go
type RunCmd struct {
    Quiet    bool   `help:"Suppress progress output, only show summary" short:"q"`
    TraceDir string `help:"Directory to write trace files" type:"path"`
    Config   string `help:"Path to evaluation configuration file (YAML or JSON)" required:"" type:"path"`
    APIKey   string `help:"Anthropic API key (overrides ANTHROPIC_API_KEY env var)"`
    BaseURL  string `help:"Base URL for Anthropic API (overrides ANTHROPIC_BASE_URL env var)"`
    Verbose  bool   `help:"Show detailed per-eval breakdown" short:"v"`
    Filter   string `help:"Regex pattern to filter which evals to run (matches against eval name)" short:"f"`
}
```

Add filtering logic before running evals:
```go
func (r *RunCmd) Run(globals *Globals) error {
    // Load configuration
    config, err := evaluations.LoadConfig(r.Config)
    if err != nil {
        return fmt.Errorf("failed to load config: %w", err)
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

    // ... continue with evalsToRun instead of config.Evals
}

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
```

### Usage Examples

**Run a single eval**:
```bash
mcp-evals run --config evals.yaml --filter "^basic_addition$"
```

**Run all auth-related evals**:
```bash
mcp-evals run --config evals.yaml --filter "auth"
```

**Run multiple specific evals**:
```bash
mcp-evals run --config evals.yaml --filter "add|echo|get_user"
```

**Run evals matching a pattern**:
```bash
mcp-evals run --config evals.yaml --filter "troubleshoot_.*"
```

**Run evals excluding a pattern** (negative lookahead):
```bash
mcp-evals run --config evals.yaml --filter "^(?!slow_).*"
```

### Error Handling

**Invalid regex pattern**:
```bash
$ mcp-evals run --config evals.yaml --filter "[invalid"
Error: invalid filter pattern: error parsing regexp: missing closing ]: `[invalid`
```

**No matching evals**:
```bash
$ mcp-evals run --config evals.yaml --filter "nonexistent"
Error: no evals matched filter pattern: nonexistent
```

### Output Examples

**With filter applied**:
```
Filter 'auth' matched 3 of 10 eval(s)
Running 3 evaluation(s)...

[1/3] Running eval: auth_basic
        Test basic authentication flow
        ✓ Completed (avg score: 4.8/5)

[2/3] Running eval: auth_token
        Test token-based authentication
        ✓ Completed (avg score: 4.6/5)

[3/3] Running eval: auth_refresh
        Test token refresh mechanism
        ✓ Completed (avg score: 4.9/5)
```

## MCP Server Command-Line Overrides

### Requirements

- Add `--mcp-command`, `--mcp-args`, and `--mcp-env` flags to the `run` command
- Allow overriding the MCP server configuration from the command line
- Command-line flags take precedence over configuration file values
- Enables quick ad-hoc testing without modifying config files
- Works seamlessly with environment variable interpolation

### Implementation Details

**File**: `internal/commands/run.go`

Add MCP server override flags to `RunCmd`:
```go
type RunCmd struct {
    Quiet    bool   `help:"Suppress progress output, only show summary" short:"q"`
    TraceDir string `help:"Directory to write trace files" type:"path"`
    Config   string `help:"Path to evaluation configuration file (YAML or JSON)" required:"" type:"path"`
    APIKey   string `help:"Anthropic API key (overrides ANTHROPIC_API_KEY env var)"`
    BaseURL  string `help:"Base URL for Anthropic API (overrides ANTHROPIC_BASE_URL env var)"`
    Verbose  bool   `help:"Show detailed per-eval breakdown" short:"v"`
    Filter   string `help:"Regex pattern to filter which evals to run" short:"f"`

    // MCP Server overrides
    MCPCommand string   `help:"Override MCP server command from config"`
    MCPArgs    []string `help:"Override MCP server args from config"`
    MCPEnv     []string `help:"Override MCP server env vars from config"`
}
```

Apply overrides after loading config:
```go
func (r *RunCmd) Run(globals *Globals) error {
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

    // ... continue with eval execution
}
```

### Usage Examples

**Override server command**:
```bash
mcp-evals run --config evals.yaml --mcp-command /releases/v2.0.0/server
```

**Override command with arguments**:
```bash
mcp-evals run --config evals.yaml \
  --mcp-command /path/to/server \
  --mcp-args="--port=9000" \
  --mcp-args="--verbose"
```

**Override environment variables**:
```bash
mcp-evals run --config evals.yaml \
  --mcp-env="API_TOKEN=xyz123" \
  --mcp-env="DEBUG=true"
```

**Combine with filter for targeted testing**:
```bash
mcp-evals run --config evals.yaml \
  --mcp-command /path/to/dev/server \
  --filter "^new_feature_.*" \
  --trace-dir traces/dev-test
```

### Use Cases

1. **Local development**: Quickly test against a locally built server without modifying config
2. **Matrix testing**: Override server path in CI/CD loops while keeping config consistent
3. **Debugging**: Add debug flags or change env vars for troubleshooting
4. **Ad-hoc testing**: Test experimental server builds without creating new config files

## Testing Strategy

### Unit Tests

**File**: `mcp_eval_config_test.go`

```go
func TestLoadConfig_EnvironmentVariableExpansion(t *testing.T) {
    assert := require.New(t)

    // Set test environment variables
    t.Setenv("TEST_COMMAND", "/usr/bin/test-server")
    t.Setenv("TEST_PORT", "8080")
    t.Setenv("TEST_VERSION", "1.0.0")

    // Create temporary config file with env vars
    configContent := `
model: claude-sonnet-4-6
mcp_server:
  command: ${TEST_COMMAND}
  args:
    - --port=${TEST_PORT}
  env:
    - VERSION=${TEST_VERSION}
evals:
  - name: test
    prompt: "Version is ${TEST_VERSION}"
    expected_result: "Should report ${TEST_VERSION}"
`

    tmpFile := filepath.Join(t.TempDir(), "config.yaml")
    assert.NoError(os.WriteFile(tmpFile, []byte(configContent), 0644))

    // Load and verify expansion
    config, err := LoadConfig(tmpFile)
    assert.NoError(err)
    assert.Equal("/usr/bin/test-server", config.MCPServer.Command)
    assert.Equal([]string{"--port=8080"}, config.MCPServer.Args)
    assert.Contains(config.MCPServer.Env, "VERSION=1.0.0")
    assert.Contains(config.Evals[0].Prompt, "Version is 1.0.0")
}

func TestLoadConfig_UndefinedEnvironmentVariable(t *testing.T) {
    assert := require.New(t)

    configContent := `
model: claude-sonnet-4-6
mcp_server:
  command: ${UNDEFINED_VAR}
evals:
  - name: test
    prompt: "test"
`

    tmpFile := filepath.Join(t.TempDir(), "config.yaml")
    assert.NoError(os.WriteFile(tmpFile, []byte(configContent), 0644))

    config, err := LoadConfig(tmpFile)
    assert.NoError(err)
    assert.Equal("", config.MCPServer.Command) // Expands to empty string
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

    tmpFile := filepath.Join(t.TempDir(), "config.yaml")
    assert.NoError(os.WriteFile(tmpFile, []byte(configContent), 0644))

    config, err := LoadConfig(tmpFile)
    assert.NoError(err)
    assert.Equal([]string{"--port=9000", "--host=localhost"}, config.MCPServer.Args)
}
```

**File**: `internal/commands/run_test.go` (new)

```go
func TestFilterEvals(t *testing.T) {
    assert := require.New(t)

    evals := []evaluations.Eval{
        {Name: "auth_basic"},
        {Name: "auth_token"},
        {Name: "user_create"},
        {Name: "user_delete"},
        {Name: "admin_auth"},
    }

    tests := []struct {
        name     string
        pattern  string
        expected []string
        wantErr  bool
    }{
        {
            name:     "match prefix",
            pattern:  "^auth",
            expected: []string{"auth_basic", "auth_token"},
        },
        {
            name:     "match suffix",
            pattern:  "auth$",
            expected: []string{"admin_auth"},
        },
        {
            name:     "match multiple",
            pattern:  "auth|user",
            expected: []string{"auth_basic", "auth_token", "user_create", "user_delete", "admin_auth"},
        },
        {
            name:     "match substring",
            pattern:  "token",
            expected: []string{"auth_token"},
        },
        {
            name:     "no matches",
            pattern:  "nonexistent",
            expected: []string{},
        },
        {
            name:     "invalid regex",
            pattern:  "[invalid",
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert := require.New(t)

            result, err := filterEvals(evals, tt.pattern)

            if tt.wantErr {
                assert.Error(err)
                return
            }

            assert.NoError(err)

            var names []string
            for _, e := range result {
                names = append(names, e.Name)
            }
            assert.Equal(tt.expected, names)
        })
    }
}
```

### Integration Tests

**File**: `mcp_evals_e2e_test.go`

```go
func TestE2E_EnvironmentVariableExpansion(t *testing.T) {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        t.Skip("ANTHROPIC_API_KEY not set, skipping e2e test")
    }

    // Build test server
    serverPath := buildTestServer(t)

    // Set environment variables
    t.Setenv("TEST_SERVER_PATH", serverPath)
    t.Setenv("TEST_VERSION", "1.0.0-test")

    // Create config with env vars
    configContent := fmt.Sprintf(`
model: claude-sonnet-4-6
mcp_server:
  command: ${TEST_SERVER_PATH}
  args: []
  env:
    - VERSION=${TEST_VERSION}
evals:
  - name: env_test
    prompt: "What is 2 plus 2?"
    expected_result: "Should return 4"
`)

    configPath := filepath.Join(t.TempDir(), "config.yaml")
    require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

    // Load and verify config expansion
    config, err := evaluations.LoadConfig(configPath)
    require.NoError(t, err)
    require.Equal(t, serverPath, config.MCPServer.Command)
    require.Contains(t, config.MCPServer.Env, "VERSION=1.0.0-test")
}
```

## Documentation Updates

### README.md

Add sections:

**Environment Variable Interpolation**:
```markdown
## Matrix Testing with Environment Variables

Configuration files support environment variable interpolation using `${VAR}` or `$VAR` syntax:

\`\`\`yaml
mcp_server:
  command: ${MCP_SERVER_PATH}
  args:
    - --version=${SERVER_VERSION}
  env:
    - API_TOKEN=${TEST_API_TOKEN}
\`\`\`

This enables testing across multiple server versions:

\`\`\`bash
export MCP_SERVER_PATH=/releases/v1.0.0/server
mcp-evals run --config matrix.yaml --trace-dir traces/v1.0.0

export MCP_SERVER_PATH=/releases/v2.0.0/server
mcp-evals run --config matrix.yaml --trace-dir traces/v2.0.0
\`\`\`

Default values are supported using `${VAR:-default}` syntax.
```

**Eval Filtering**:
```markdown
## Filtering Evals

Run a subset of evals using the `--filter` flag with a regex pattern:

\`\`\`bash
# Run single eval
mcp-evals run --config evals.yaml --filter "^basic_addition$"

# Run all auth evals
mcp-evals run --config evals.yaml --filter "auth"

# Run multiple specific evals
mcp-evals run --config evals.yaml --filter "add|echo|get_user"
\`\`\`
```

### JSON Schema

Document that string fields support environment variable interpolation in the schema description.

## Breaking Changes

This feature introduces no breaking changes:

- Environment variable interpolation is opt-in (only applied when using `$` syntax)
- Filter flag is optional (defaults to running all evals)
- Existing configurations work unchanged

## Success Metrics

- Customers can test against multiple MCP server versions without duplicating configuration
- Development iteration cycles reduce through selective eval execution
- CI/CD pipelines can implement matrix testing strategies
- API costs reduce when running targeted eval subsets during development

## Future Enhancements

- Support for eval tagging (e.g., `tags: [auth, critical]`) and tag-based filtering
- Multiple filter patterns (AND/OR logic)
- Inverse filtering (`--exclude` flag)
- List mode to show which evals would run without executing them (`--dry-run`)
