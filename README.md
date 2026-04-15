# mcp-evals

A Go library and CLI for evaluating Model Context Protocol (MCP) servers using Claude. This tool connects to an MCP server, runs an agentic evaluation loop where Claude uses the server's tools to answer questions, and grades the responses across five dimensions: accuracy, completeness, relevance, clarity, and reasoning.

## Use Cases

**As a library**: Programmatically evaluate MCP servers in Go code, integrate evaluation results into CI/CD pipelines, or build custom evaluation workflows.

**As a CLI**: Run evaluations from YAML/JSON configuration files with immediate pass/fail feedback, detailed scoring breakdowns, and optional trace output for debugging.

## Installation

### Using install script (recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/wolfeidau/mcp-evals/main/install.sh | sh
```

### Using Go

```bash
go install github.com/wolfeidau/mcp-evals/cmd/mcp-evals@latest
```

## Quick Start

Create an evaluation config file (e.g., `evals.yaml`):

```yaml
model: claude-sonnet-4-6
mcp_server:
  command: npx
  args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

evals:
  - name: list-files
    description: Test filesystem listing
    prompt: "List files in the current directory"
    expected_result: "Should enumerate files with details"
```

Run evaluations:

```bash
export ANTHROPIC_API_KEY=your-api-key
mcp-evals run --config evals.yaml
```

## CLI Commands

- `run` - Execute evaluations (default command)
- `validate` - Validate config file against JSON schema
- `schema` - Generate JSON schema for configuration
- `help` - Show help information

See `mcp-evals <command> --help` for detailed usage.

## Advanced Features

### Environment Variable Interpolation

Configuration files support environment variable interpolation using shell syntax. This enables matrix testing across different MCP server versions without duplicating configuration.

**Supported syntax:**
- `${VAR}` - Expand environment variable
- `$VAR` - Short form expansion
- `${VAR:-default}` - Use default value if unset
- `${VAR:+value}` - Use value if VAR is set

**Example configuration** (`matrix.yaml`):
```yaml
model: claude-sonnet-4-6
mcp_server:
  command: ${MCP_SERVER_PATH}
  args:
    - --port=${SERVER_PORT:-8080}
  env:
    - VERSION=${SERVER_VERSION}

evals:
  - name: version_check
    prompt: "What version are you running?"
    expected_result: "Should report ${SERVER_VERSION}"
```

**Matrix testing across versions:**
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

**CI/CD matrix example (Buildkite)**:
```yaml
steps:
  - label: ":test_tube: MCP Evals - {{matrix.version}}"
    command: |
      export MCP_SERVER_PATH=/releases/{{matrix.version}}/mcp-server
      export SERVER_VERSION={{matrix.version}}
      mcp-evals run --config matrix.yaml --trace-dir traces/{{matrix.version}}
    matrix:
      setup:
        version: ["1.0.0", "1.1.0", "2.0.0"]
    artifact_paths:
      - "traces/{{matrix.version}}/*.json"
```

### Eval Filtering

Run a subset of evals using the `--filter` flag with a regex pattern:

```bash
# Run single eval
mcp-evals run --config evals.yaml --filter "^basic_addition$"

# Run all auth-related evals
mcp-evals run --config evals.yaml --filter "auth"

# Run multiple specific evals
mcp-evals run --config evals.yaml --filter "add|echo|get_user"

# Run all troubleshooting evals
mcp-evals run --config evals.yaml --filter "troubleshoot_.*"
```

This is useful for:
- Fast iteration during development
- Running specific test suites in CI/CD
- Debugging individual evals without running the full suite

### MCP Server Command-Line Overrides

Override MCP server configuration from the command line for quick testing:

```bash
# Override server command
mcp-evals run --config evals.yaml --mcp-command /path/to/dev/server

# Override with arguments
mcp-evals run --config evals.yaml \
  --mcp-command /path/to/server \
  --mcp-args="--port=9000" \
  --mcp-args="--verbose"

# Override environment variables
mcp-evals run --config evals.yaml \
  --mcp-env="API_TOKEN=xyz123" \
  --mcp-env="DEBUG=true"

# Combine with filtering for targeted testing
mcp-evals run --config evals.yaml \
  --mcp-command /path/to/dev/server \
  --filter "^new_feature_.*"
```

This is useful for:
- Local development without modifying config files
- Ad-hoc testing of experimental builds
- Debugging with different server flags

## Configuration

Evaluation configs support both YAML and JSON formats:

- `model` - Anthropic model ID (required)
- `grading_model` - Optional separate model for grading
- `timeout` - Per-evaluation timeout (e.g., "2m", "30s")
- `max_steps` - Maximum agentic loop iterations (default: 10)
- `max_tokens` - Maximum tokens per LLM request (default: 4096)
- `mcp_server` - Server command, args, and environment
- `evals` - List of test cases with name, prompt, and expected result

## Custom Grading Rubrics

Custom grading rubrics allow you to define specific, measurable criteria for each evaluation dimension. This makes grading more consistent and meaningful by providing concrete guidance to the grading LLM.

### Why Use Rubrics?

Without rubrics, the grading LLM uses generic 1-5 scoring criteria. This can lead to:
- **Inconsistent scoring**: Same response quality gets different grades
- **Lack of specificity**: Generic criteria don't capture domain-specific requirements
- **Difficult iteration**: Can't specify what matters most for your use case

Rubrics solve this by defining exactly what "accurate" or "complete" means for each evaluation.

### Basic Example

```yaml
evals:
  - name: troubleshoot_build
    prompt: "Troubleshoot the failed build at https://example.com/builds/123"
    expected_result: "Should identify root cause and provide remediation"

    grading_rubric:
      # Optional: Focus on specific dimensions (defaults to all 5)
      dimensions: ["accuracy", "completeness", "reasoning"]

      accuracy:
        description: "Correctness of root cause identification"
        must_have:
          - "Identifies actual failing job(s) by name or ID"
          - "Extracts real error messages from logs"
        penalties:
          - "Misidentifies root cause"
          - "Fabricates error messages not in logs"

      completeness:
        description: "Thoroughness of investigation"
        must_have:
          - "Examines job logs"
          - "Provides specific remediation steps"
        nice_to_have:
          - "Suggests preventive measures"

      # Optional: Minimum acceptable scores for pass/fail
      minimum_scores:
        accuracy: 4
        completeness: 3
```

### Rubric Structure

Each dimension can specify:

- **`description`**: What this dimension means for this specific eval
- **`must_have`**: Required elements for high scores (4-5)
- **`nice_to_have`**: Optional elements that improve scores
- **`penalties`**: Elements that reduce scores (errors, omissions)

Available dimensions: `accuracy`, `completeness`, `relevance`, `clarity`, `reasoning`

### LLM-Assisted Rubric Creation

Manually writing rubrics is time-consuming. Use an LLM to draft initial rubrics:

```bash
# Generate rubric from eval description
claude "Create a grading rubric for this eval: [paste your eval config]"

# Refine rubric from actual results
mcp-evals run --config evals.yaml --trace-dir traces
claude "Refine this rubric based on these results: $(cat traces/my_eval.json | jq '.grade')"
```

**Best practices:**
1. Start generic, refine iteratively
2. Use actual tool outputs and responses in prompts
3. Focus on measurable criteria (not vague requirements)
4. Run eval 3-5 times to validate consistency

See [specs/grading_rubric.md](specs/grading_rubric.md) for detailed guidance on creating rubrics.

## How It Works

1. Connects to the specified MCP server via command/transport
2. Retrieves available tools from the MCP server
3. Runs an agentic loop (max 10 steps) where Claude:
   - Receives the evaluation prompt and available MCP tools
   - Calls tools via the MCP protocol as needed
   - Accumulates tool results and continues reasoning
4. Evaluates the final response using a separate LLM call that scores five dimensions on a 1-5 scale
5. Returns structured results with pass/fail status (passing threshold: average score ≥ 3.0)

## License

Apache License, Version 2.0 - Copyright [Mark Wolfe](https://www.wolfe.id.au)