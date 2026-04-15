# CLI Specification

## Overview

A minimal command-line interface for running MCP evaluations from configuration files. The CLI uses a subcommand architecture for extensibility, with `run` as the default command for executing evaluations.

## Implementation Status

✅ **Phase 1 Complete**: `run` command fully implemented and tested
⏸️ **Phase 2 Deferred**: `validate` command to be implemented later

### Currently Implemented
- `run` command with all flags (`--config`, `--api-key`, `--trace-dir`, `--quiet`)
- Progress output with evaluation status
- Summary table with scoring breakdown
- Trace file writing to JSON
- Exit code handling for CI/CD integration
- Help system and usage documentation

### Deferred to Later
- `validate` command for schema validation

## Problem Statement

Users need a simple way to:
- Run evaluations defined in YAML/JSON config files
- Get quick pass/fail feedback on stdout with detailed scoring
- Optionally capture detailed trace data for debugging and analysis
- Integrate evals into CI/CD pipelines (exit codes for success/failure)
- See progress during evaluation execution

The CLI should be minimal and focused, deferring rich reporting (HTML, markdown) to future work.

## Command Architecture

The CLI uses subcommands for different operations:

```bash
mcp-evals <command> [options]
```

### Available Commands

- `run`: Execute evaluations (default if no command specified) ✅ **IMPLEMENTED**
- `validate`: Validate config file against JSON schema ⏸️ **DEFERRED**
- `schema`: Generate JSON schema for evaluation configuration ✅ **IMPLEMENTED**
- `help`: Show help information ✅ **IMPLEMENTED**

## Command: `run`

Executes evaluations defined in a configuration file.

### Syntax

```bash
mcp-evals run --config <path> [options]
mcp-evals --config <path> [options]  # 'run' is default
```

### Flags

#### Required

- `--config`, `-c` (string): Path to YAML or JSON configuration file
  - Must contain `model`, `mcp_server`, and `evals` sections
  - Example: `--config testdata/mcp-test-evals.yaml`

#### Optional

- `--api-key` (string): Anthropic API key
  - Falls back to `ANTHROPIC_API_KEY` environment variable if not provided
  - Example: `--api-key sk-ant-...`

- `--trace-dir` (string): Directory to write trace JSON files
  - If not specified, no trace files are written
  - Creates directory if it doesn't exist
  - One file per eval: `{trace-dir}/{eval-name}.trace.json`
  - Example: `--trace-dir ./traces`

- `--quiet`, `-q` (bool): Suppress progress output
  - Only prints final summary
  - Useful for CI/CD where you only care about exit code
  - Default: false

### Output Format

#### Standard Output

The CLI prints structured, readable output:

**During execution (unless --quiet):**
```
Running 3 evaluation(s)...

[1/3] Running eval: add
        Test basic addition
        ✓ Completed (avg score: 4.0/5)

[2/3] Running eval: echo
        Test echo tool
        ✓ Completed (avg score: 1.8/5)

[3/3] Running eval: get_user_info
        Test realistic JSON API response handling
        ✓ Completed (avg score: 2.0/5)
```

**After completion:**
```
================================================================================
EVALUATION SUMMARY
================================================================================

Name                 Status     Acc      Comp     Rel      Clar     Reas
--------------------------------------------------------------------------------
add                  PASS       5        5        4        4        2
echo                 FAIL       2        1        3        2        1
get_user_info        FAIL       1        1        4        3        1

Total: 3 | Pass: 1 | Fail: 2 | Error: 0
```

**Legend:**
- PASS = eval passed (average score ≥ 3.0)
- FAIL = eval failed (average score < 3.0)
- ERROR = execution error occurred
- Acc = Accuracy, Comp = Completeness, Rel = Relevance, Clar = Clarity, Reas = Reasoning
- Scores range 1-5

**On execution error:**
```
Name                 Status     Acc      Comp     Rel      Clar     Reas
--------------------------------------------------------------------------------
add                  ERROR      Error: connection timeout
```

#### Trace Files (Optional)

When `--trace-dir` is specified, write one JSON file per eval containing the complete `EvalTrace` structure:

```json
{
  "steps": [
    {
      "step_number": 1,
      "start_time": "2025-10-02T10:30:00Z",
      "end_time": "2025-10-02T10:30:02Z",
      "duration": 2000000000,
      "model_response": "I'll use the add tool...",
      "stop_reason": "tool_use",
      "tool_calls": [
        {
          "tool_id": "toolu_123",
          "tool_name": "add",
          "start_time": "2025-10-02T10:30:01Z",
          "end_time": "2025-10-02T10:30:01.5Z",
          "duration": 500000000,
          "input": {"a": 5, "b": 3},
          "output": {"result": "8"},
          "success": true
        }
      ],
      "input_tokens": 250,
      "output_tokens": 50
    }
  ],
  "grading": {
    "user_prompt": "What is 5 plus 3?",
    "model_response": "The result is 8.",
    "expected_result": "Should return 8",
    "grading_prompt": "Here is the user input: ...",
    "raw_grading_output": "{\"accuracy\": 5, ...}",
    "start_time": "2025-10-02T10:30:05Z",
    "end_time": "2025-10-02T10:30:06Z",
    "duration": 1000000000,
    "input_tokens": 100,
    "output_tokens": 50
  },
  "total_duration": 6000000000,
  "total_input_tokens": 350,
  "total_output_tokens": 100,
  "step_count": 1,
  "tool_call_count": 1
}
```

#### Exit Codes

- `0`: All evals passed
- `1`: One or more evals failed or execution error occurred

### Usage Examples

#### Basic Execution

```bash
mcp-evals run --config testdata/mcp-test-evals.yaml
mcp-evals --config testdata/mcp-test-evals.yaml  # 'run' is default
```

Output:
```
Running 3 evaluation(s)...

[1/3] Running eval: add
        Test basic addition
        ✓ Completed (avg score: 4.0/5)

[2/3] Running eval: echo
        Test echo tool
        ✓ Completed (avg score: 5.0/5)

[3/3] Running eval: get_user_info
        Test realistic JSON API response handling
        ✓ Completed (avg score: 4.6/5)

================================================================================
EVALUATION SUMMARY
================================================================================

Name                 Status     Acc      Comp     Rel      Clar     Reas
--------------------------------------------------------------------------------
add                  PASS       5        5        4        4        2
echo                 PASS       5        5        5        5        5
get_user_info        PASS       4        5        5        4        5

Total: 3 | Pass: 3 | Fail: 0 | Error: 0
```

#### With Trace Output

```bash
mcp-evals run --config testdata/mcp-test-evals.yaml --trace-dir ./traces
```

Creates:
- `traces/add.json`
- `traces/echo.json`
- `traces/get_user_info.json`

#### Quiet Mode (CI/CD)

```bash
mcp-evals run --config evals.yaml --quiet
echo $?  # Check exit code
```

Output (only summary, no progress):
```
================================================================================
EVALUATION SUMMARY
================================================================================

Name                 Status     Acc      Comp     Rel      Clar     Reas
--------------------------------------------------------------------------------
add                  PASS       5        5        4        4        3
echo                 PASS       5        5        5        5        5
get_user_info        PASS       4        5        5        4        5

Total: 3 | Pass: 3 | Fail: 0 | Error: 0
```

#### Custom API Key

```bash
mcp-evals run --config evals.yaml --api-key sk-ant-custom-key
```

---

## Command: `validate`

Validates a configuration file against the JSON schema without running evaluations.

### Syntax

```bash
mcp-evals validate --config <path> [options]
```

### Flags

#### Required

- `--config`, `-c` (string): Path to YAML or JSON configuration file to validate
  - Example: `--config testdata/mcp-test-evals.yaml`

#### Optional

- `--schema` (string): Path to custom JSON schema file
  - Default: Uses embedded `eval-config-schema.json`
  - Example: `--schema ./custom-schema.json`

### Output Format

#### Success Case

```bash
mcp-evals validate --config testdata/mcp-test-evals.yaml
```

Output:
```
✓ Configuration is valid
  - Model: claude-sonnet-4-6
  - MCP Server: go run testdata/mcp-test-server/main.go
  - Evals: 3
```

#### Validation Errors

```bash
mcp-evals validate --config invalid.yaml
```

Output:
```
✗ Configuration validation failed:

Error at $.model: required property missing
Error at $.evals[0].prompt: must not be empty
Error at $.mcp_server.env[2]: must match pattern "^[A-Z_][A-Z0-9_]*=.*$" (got "invalid-env")

3 errors found
```

#### File Not Found

```bash
mcp-evals validate --config missing.yaml
```

Output:
```
✗ Error: failed to read config file: open missing.yaml: no such file or directory
```

#### YAML/JSON Parse Error

```bash
mcp-evals validate --config malformed.yaml
```

Output:
```
✗ Error: failed to parse YAML: yaml: line 5: did not find expected key
```

#### Exit Codes

- `0`: Configuration is valid
- `1`: Validation failed or error occurred

### Usage Examples

#### Validate Before Running

```bash
# Validate first
mcp-evals validate --config evals.yaml && \
  mcp-evals run --config evals.yaml
```

#### CI/CD Pipeline

```yaml
# .github/workflows/evals.yml
- name: Validate config
  run: mcp-evals validate --config evals.yaml

- name: Run evals
  run: mcp-evals run --config evals.yaml
```

#### Custom Schema

```bash
mcp-evals validate --config evals.yaml --schema ./custom-schema.json
```

---

## Configuration

The CLI loads configuration from YAML or JSON files using the existing `LoadConfig()` function.

### Config File Structure

```yaml
model: claude-sonnet-4-6
grading_model: claude-sonnet-4-6  # optional
timeout: 2m                                  # optional
max_steps: 10                                # optional
max_tokens: 4096                             # optional

mcp_server:
  command: go
  args:
    - run
    - testdata/mcp-test-server/main.go
  env:
    - TEST_API_TOKEN=test-secret-123

evals:
  - name: add
    description: Test basic addition
    prompt: "What is 5 plus 3?"
    expected_result: "Should return 8"

  - name: echo
    description: Test echo tool
    prompt: "Echo the message 'hello world'"
    expected_result: "Should return 'hello world'"
```

### Config Overrides

CLI flags override config file values:
- `--api-key` overrides environment variable (config doesn't contain API key for security)

## Actual Implementation

The CLI has been implemented in [cmd/mcp-evals/main.go](../cmd/mcp-evals/main.go) with the following structure:

### Key Implementation Details

**Subcommand Routing:**
- Defaults to `run` command when flags (starting with `-`) are provided
- Explicit subcommand support: `run`, `schema`, `help`
- Returns errors instead of calling `os.Exit()` to avoid defer issues

**Progress Output:**
- Shows `[n/total]` progress indicators
- Displays eval name and description
- Shows completion status with average score
- Suppressed entirely with `--quiet` flag

**Summary Table:**
- 80-character wide formatted table
- Shows all 5 scoring dimensions (1-5 scale)
- PASS/FAIL based on average score ≥ 3.0
- Total statistics line at bottom

**Trace Files:**
- Named `{eval-name}.json` (no `.trace` suffix)
- Contains complete `EvalTrace` structure
- Includes all agentic steps, tool calls, and grading data
- Written with 0644 permissions for debugging

**Exit Codes:**
- Returns error from main functions
- Exit handled at top level in main()
- Exit code 1 when any eval fails

### Makefile Integration

Added targets for testing:
```make
make run-test        # Run with test config using 1Password
make run-test-trace  # Same but saves traces to ./traces/
```

## Original Implementation Plan (Reference)

### 1. Main Function Structure (Subcommand Architecture)

```go
func main() {
    if len(os.Args) < 2 {
        // Default to 'run' command if no subcommand provided
        os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
    }

    command := os.Args[1]

    switch command {
    case "run":
        runCommand()
    case "validate":
        validateCommand()
    case "help", "--help", "-h":
        printHelp()
        os.Exit(0)
    default:
        // Check if it looks like a flag (starts with -)
        if strings.HasPrefix(command, "-") {
            // Assume 'run' command
            os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
            runCommand()
        } else {
            fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
            fmt.Fprintf(os.Stderr, "Run 'mcp-evals help' for usage.\n")
            os.Exit(1)
        }
    }
}

func runCommand() {
    // Create FlagSet for run command
    runFlags := flag.NewFlagSet("run", flag.ExitOnError)
    configPath := runFlags.String("config", "", "Path to config file (required)")
    runFlags.StringVar(configPath, "c", "", "Path to config file (shorthand)")
    apiKey := runFlags.String("api-key", "", "Anthropic API key")
    traceDir := runFlags.String("trace-dir", "", "Directory to write trace files")
    quiet := runFlags.Bool("quiet", false, "Suppress progress output")
    runFlags.BoolVar(quiet, "q", false, "Suppress progress output (shorthand)")

    runFlags.Parse(os.Args[2:])

    if *configPath == "" {
        fmt.Fprintf(os.Stderr, "Error: --config is required\n")
        os.Exit(1)
    }

    // Load config
    config, err := evaluations.LoadConfig(*configPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
        os.Exit(1)
    }

    // Create eval client
    client := createClient(config, *apiKey)

    // Run evals
    results := runEvals(context.Background(), client, config.Evals, *quiet)

    // Write traces if requested
    if *traceDir != "" {
        writeTraces(results, *traceDir)
    }

    // Print summary and exit
    exitCode := printSummary(results)
    os.Exit(exitCode)
}

func validateCommand() {
    // Create FlagSet for validate command
    validateFlags := flag.NewFlagSet("validate", flag.ExitOnError)
    configPath := validateFlags.String("config", "", "Path to config file (required)")
    validateFlags.StringVar(configPath, "c", "", "Path to config file (shorthand)")
    schemaPath := validateFlags.String("schema", "", "Path to custom schema file (optional)")

    validateFlags.Parse(os.Args[2:])

    if *configPath == "" {
        fmt.Fprintf(os.Stderr, "Error: --config is required\n")
        os.Exit(1)
    }

    exitCode := validateConfig(*configPath, *schemaPath)
    os.Exit(exitCode)
}

func printHelp() {
    fmt.Println(`mcp-evals - Run evaluations against MCP servers

Usage:
  mcp-evals <command> [options]

Commands:
  run       Execute evaluations (default)
  validate  Validate configuration file
  help      Show this help message

Run 'mcp-evals <command> --help' for more information on a command.`)
}
```

### 2. Helper Functions

**createClient**: Merge config + CLI flags into `EvalClientConfig`

```go
func createClient(config *evaluations.EvalConfig, apiKey string) *evaluations.EvalClient {
    return evaluations.NewEvalClient(evaluations.EvalClientConfig{
        APIKey:       apiKey,  // Falls back to ANTHROPIC_API_KEY env var
        Command:      config.MCPServer.Command,
        Args:         config.MCPServer.Args,
        Env:          config.MCPServer.Env,
        Model:        config.Model,
        GradingModel: config.GradingModel,
        MaxSteps:     config.MaxSteps,
        MaxTokens:    config.MaxTokens,
    })
}
```

**runEvals**: Execute evals with progress output

```go
func runEvals(ctx context.Context, client *evaluations.EvalClient, evals []evaluations.Eval, quiet bool) []evaluations.EvalRunResult {
    results := make([]evaluations.EvalRunResult, 0, len(evals))

    for _, eval := range evals {
        if !quiet {
            fmt.Printf("Running eval: %s...\n", eval.Name)
        }

        result, err := client.RunEval(ctx, eval)
        if err != nil {
            results = append(results, evaluations.EvalRunResult{
                Eval:  eval,
                Error: err,
            })
            continue
        }

        results = append(results, *result)
    }

    return results
}
```

**writeTraces**: Save trace JSON files

```go
func writeTraces(results []evaluations.EvalRunResult, traceDir string) {
    // Create directory if it doesn't exist
    if err := os.MkdirAll(traceDir, 0755); err != nil {
        fmt.Fprintf(os.Stderr, "Warning: failed to create trace directory: %v\n", err)
        return
    }

    for _, result := range results {
        if result.Trace == nil {
            continue
        }

        filename := filepath.Join(traceDir, result.Eval.Name+".trace.json")
        traceJSON, err := json.MarshalIndent(result.Trace, "", "  ")
        if err != nil {
            fmt.Fprintf(os.Stderr, "Warning: failed to marshal trace for %s: %v\n", result.Eval.Name, err)
            continue
        }

        if err := os.WriteFile(filename, traceJSON, 0644); err != nil {
            fmt.Fprintf(os.Stderr, "Warning: failed to write trace file %s: %v\n", filename, err)
            continue
        }
    }
}
```

**printSummary**: Terse output with pass/fail

```go
func printSummary(results []evaluations.EvalRunResult) int {
    passed := 0
    failed := 0

    for _, result := range results {
        if result.Error != nil {
            fmt.Printf("✗ %s - Error: %v\n", result.Eval.Name, result.Error)
            failed++
            continue
        }

        if result.Grade == nil {
            fmt.Printf("✓ %s [grading failed]\n", result.Eval.Name)
            passed++
            continue
        }

        fmt.Printf("✓ %s [A:%d C:%d R:%d Cl:%d Re:%d]\n",
            result.Eval.Name,
            result.Grade.Accuracy,
            result.Grade.Completeness,
            result.Grade.Relevance,
            result.Grade.Clarity,
            result.Grade.Reasoning,
        )
        passed++
    }

    fmt.Printf("\n%d/%d passed (%.1f%%)\n", passed, len(results),
        float64(passed)/float64(len(results))*100)

    if failed > 0 {
        return 1
    }
    return 0
}
```

**validateConfig**: Validate config against JSON schema

```go
import (
    _ "embed"
    "github.com/google/jsonschema-go"
)

//go:embed eval-config-schema.json
var defaultSchemaJSON []byte

func validateConfig(configPath string, schemaPath string) int {
    // Read config file
    configData, err := os.ReadFile(configPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "✗ Error: failed to read config file: %v\n", err)
        return 1
    }

    // Convert YAML to JSON if needed
    ext := strings.ToLower(filepath.Ext(configPath))
    var configJSON []byte

    switch ext {
    case ".yaml", ".yml":
        var configMap map[string]any
        if err := yaml.Unmarshal(configData, &configMap); err != nil {
            fmt.Fprintf(os.Stderr, "✗ Error: failed to parse YAML: %v\n", err)
            return 1
        }
        configJSON, err = json.Marshal(configMap)
        if err != nil {
            fmt.Fprintf(os.Stderr, "✗ Error: failed to convert config to JSON: %v\n", err)
            return 1
        }
    case ".json":
        configJSON = configData
    default:
        fmt.Fprintf(os.Stderr, "✗ Error: unsupported file extension: %s\n", ext)
        return 1
    }

    // Load schema
    var schemaData []byte
    if schemaPath != "" {
        schemaData, err = os.ReadFile(schemaPath)
        if err != nil {
            fmt.Fprintf(os.Stderr, "✗ Error: failed to read schema file: %v\n", err)
            return 1
        }
    } else {
        schemaData = defaultSchemaJSON
    }

    // Parse schema
    var schema jsonschema.Schema
    if err := json.Unmarshal(schemaData, &schema); err != nil {
        fmt.Fprintf(os.Stderr, "✗ Error: failed to parse schema: %v\n", err)
        return 1
    }

    // Validate
    var configValue any
    if err := json.Unmarshal(configJSON, &configValue); err != nil {
        fmt.Fprintf(os.Stderr, "✗ Error: failed to parse config JSON: %v\n", err)
        return 1
    }

    result := schema.Validate(configValue)

    if !result.Valid() {
        fmt.Println("✗ Configuration validation failed:")
        fmt.Println()
        for _, err := range result.Errors {
            fmt.Printf("Error at %s: %s\n", err.InstanceLocation, err.Message)
        }
        fmt.Printf("\n%d errors found\n", len(result.Errors))
        return 1
    }

    // Success - also load and display summary
    config, err := evaluations.LoadConfig(configPath)
    if err != nil {
        // Schema valid but LoadConfig failed - shouldn't happen
        fmt.Printf("✓ Configuration is valid (schema check passed)\n")
        fmt.Fprintf(os.Stderr, "Warning: LoadConfig failed: %v\n", err)
        return 0
    }

    fmt.Println("✓ Configuration is valid")
    fmt.Printf("  - Model: %s\n", config.Model)

    cmdStr := config.MCPServer.Command
    if len(config.MCPServer.Args) > 0 {
        cmdStr += " " + strings.Join(config.MCPServer.Args, " ")
    }
    fmt.Printf("  - MCP Server: %s\n", cmdStr)
    fmt.Printf("  - Evals: %d\n", len(config.Evals))

    return 0
}
```

### 3. Dependencies

**Standard library:**
- `flag`: CLI parsing with FlagSet for subcommands
- `os`: File operations, exit codes
- `fmt`: Output formatting
- `encoding/json`: Trace serialization and schema validation
- `path/filepath`: File path handling
- `context`: Timeout and cancellation
- `strings`: String operations
- `embed`: Embed schema file in binary

**External:**
- `gopkg.in/yaml.v3`: Already in use for YAML parsing
- `github.com/google/jsonschema-go`: Already in dependencies for schema validation

### 4. File Location

- Implementation: `cmd/mcp-evals/main.go`
- Package: `main`
- Schema embedded: `eval-config-schema.json` (via `//go:embed`)

### 5. Implementation Notes

**Subcommand Parsing:**
- Use `flag.NewFlagSet` for each subcommand
- Each subcommand has its own flag set to avoid conflicts
- Default to `run` command when no subcommand specified
- Support both `mcp-evals run --config` and `mcp-evals --config` syntax

**Schema Embedding:**
- Use `//go:embed` to include schema in binary
- Eliminates need to distribute schema file separately
- Users can still provide custom schema via `--schema` flag

**Error Handling:**
- All errors write to stderr
- Exit codes: 0 for success, 1 for any error
- Validation errors show clear JSON path and message

## Testing Strategy

### Manual Testing

```bash
# Build CLI
go build -o mcp-evals cmd/mcp-evals/main.go

# Test run command (explicit)
./mcp-evals run --config testdata/mcp-test-evals.yaml

# Test run command (default)
./mcp-evals --config testdata/mcp-test-evals.yaml

# Test trace output
./mcp-evals run --config testdata/mcp-test-evals.yaml --trace-dir /tmp/traces
ls -la /tmp/traces/

# Test quiet mode
./mcp-evals run --config testdata/mcp-test-evals.yaml --quiet

# Test exit code
./mcp-evals run --config testdata/mcp-test-evals.yaml && echo "Success" || echo "Failed"

# Test validate command
./mcp-evals validate --config testdata/mcp-test-evals.yaml

# Test validate with invalid config
echo "invalid: yaml: [" > /tmp/bad.yaml
./mcp-evals validate --config /tmp/bad.yaml

# Test help
./mcp-evals help
./mcp-evals --help
```

### Integration with Existing Tests

The E2E tests in `mcp_evals_e2e_test.go` already validate the underlying functionality. The CLI is a thin wrapper, so focus on:
1. Subcommand routing (run, validate, help)
2. Flag parsing correctness for each command
3. Output formatting
4. File writing (traces)
5. Exit codes
6. Schema validation logic

## Future Enhancements (Out of Scope)

These are explicitly **not** included in this initial implementation:

### Additional Subcommands
1. **`report`**: Generate HTML/markdown reports from trace files
2. **`init`**: Interactive config file creation wizard
3. **`list`**: List evals in a config file without running
4. **`schema`**: Print embedded JSON schema to stdout

### Run Command Enhancements
1. **Filtering**: Run specific evals by name or tag (`--filter`, `--tag`)
2. **Parallelization**: Run evals concurrently (`--parallel`)
3. **Progress Bars**: Fancy terminal UI with progress indicators
4. **Model Override**: CLI flag to override config model (`--model`)
5. **Timeout Override**: CLI flag to override config timeout (`--timeout`)
6. **Watch Mode**: Re-run on config file changes (`--watch`)

### Validate Command Enhancements
1. **JSON Output**: Machine-readable validation results (`--json`)
2. **Strict Mode**: Additional validation beyond schema (`--strict`)
3. **Lint Mode**: Check for common issues and best practices (`--lint`)

### Trace Analysis
1. **Built-in trace viewer**: Interactive TUI for trace files
2. **Trace diff**: Compare traces across runs
3. **Trace query**: SQL-like queries on trace data

## Non-Goals

- **Not a test framework**: This is for LLM evaluations, not unit tests
- **Not a reporter**: Trace files are raw JSON; analysis tools come later
- **Not a scheduler**: Run once and exit; no daemon mode or cron integration
- **Not a server**: No HTTP API or web interface

## Success Criteria

1. User can run evals from config file with single command
2. User can validate config files before running (CI/CD friendly)
3. Pass/fail visible at a glance on stdout
4. Detailed traces available when needed for debugging
5. Exit codes enable CI/CD integration (0 = success, 1 = failure)
6. Subcommand architecture allows easy extension
7. Schema embedded in binary (no external files required)
8. Minimal implementation (~300-400 lines of code total)
9. Zero breaking changes to library
10. Works with existing test configs in `testdata/`

## Summary

### Phase 1: Completed ✅

The `run` command has been fully implemented and tested:

**Implemented Features:**
- ✅ `run` command: Execute evals with structured output and optional trace files
- ✅ `schema` command: Generate JSON schema (pre-existing)
- ✅ Progress output with eval descriptions and average scores
- ✅ Summary table with 5-dimension scoring breakdown
- ✅ Trace file writing to JSON
- ✅ `--quiet` flag for CI/CD pipelines
- ✅ `--api-key` flag for API key override
- ✅ `--trace-dir` flag for trace output
- ✅ Subcommand architecture for future extensibility
- ✅ Exit code handling (0 = success, 1 = failure)
- ✅ Makefile targets with 1Password integration

**Implementation Size:**
- ~44 lines: `main()` + subcommand routing
- ~80 lines: `runCommand()` with flag parsing and orchestration
- ~35 lines: `runEvals()` with progress output
- ~25 lines: `writeTraces()` for JSON file writing
- ~75 lines: `printSummary()` with formatted table output
- ~40 lines: Helper functions (`createClient`, `avgScore`, `repeatString`, `printUsage`)
- Total: ~300 lines

**Design Principles:**
- Structured, readable output (not terse - table format for clarity)
- Detailed traces opt-in via `--trace-dir`
- CI/CD friendly (exit codes, quiet mode)
- Backward compatible (`run` is default command)

### Phase 2: Deferred ⏸️

To be implemented in a future update:

**Pending Features:**
- ⏸️ `validate` command: Schema validation for config files
- ⏸️ Embedded JSON schema (via `//go:embed`)
- ⏸️ Validation error reporting with JSON paths

The architecture supports these enhancements without breaking changes.
