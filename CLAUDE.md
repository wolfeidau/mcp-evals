# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a Go library and CLI for running evaluations (evals) against Model Context Protocol (MCP) servers. It uses the Anthropic SDK to evaluate how well an LLM answers questions when given access to MCP tools.

## Commands

### Build
```bash
go build ./...
```

### Lint
```bash
golangci-lint run --fix
```

### Run Tests
```bash
go test ./...
```

### Run Single Test
```bash
go test -run TestName ./...
```

### Run Tests with Coverage
```bash
go test -cover ./...
```

## Architecture

### Core Components

**EvalClient** ([mcp_evals.go](mcp_evals.go)): Main orchestrator that:
1. Connects to an MCP server via command/transport
2. Retrieves available tools from the MCP server
3. Runs an agentic loop where Claude uses those tools to answer a prompt
4. Grades the final response using a separate LLM call

**Agentic Loop**: The evaluation uses a multi-step agentic pattern (max 10 steps) where:
- Claude receives the user prompt and available MCP tools
- Claude can call tools via the MCP protocol
- Tool results are fed back to Claude
- The loop continues until Claude provides a final answer or reaches max steps

**Grading System**: After getting Claude's answer, a separate LLM call evaluates the response on 5 dimensions:
- Accuracy (factual correctness)
- Completeness (addresses all parts of question)
- Relevance (stays on topic)
- Clarity (understandable structure)
- Reasoning (logical thinking/evidence)

### Key Dependencies

- `github.com/anthropics/anthropic-sdk-go`: For Claude API interactions
- `github.com/modelcontextprotocol/go-sdk`: For MCP client and transport
- Uses streaming for real-time token accumulation during evaluation

### Configuration

`EvalClientConfig` requires:
- `APIKey`: Anthropic API key (or uses `ANTHROPIC_API_KEY` env var)
- `Command`: MCP server command to execute
- `Args`: Arguments for the MCP server command
- `Env`: Environment variables for the MCP server
- `Model`: Anthropic model ID to use (e.g., "claude-sonnet-4-6")

## Code Style
- **Logging**: ALWAYS use `"github.com/rs/zerolog/log"` for all logging operations
- Error handling: return errors up the stack, log at top level
- Package names: lowercase, descriptive (buildkite, commands, trace, tokens)
- Use contexts for cancellation

## Testing Style
- **Assertions**: ALWAYS use `assert := require.New(t)` at the start of each test function or table-driven test case
- Use the testify/require assertion style for all test assertions:
  - `assert.NoError(err)` instead of `if err != nil { t.Fatal(...) }`
  - `assert.Equal(expected, actual)` instead of `if actual != expected { t.Errorf(...) }`
  - `assert.True(condition)` instead of `if !condition { t.Error(...) }`
  - `assert.NotNil(value)` instead of `if value == nil { t.Fatal(...) }`
  - `assert.Len(slice, expectedLen)` instead of `if len(slice) != expectedLen { t.Errorf(...) }`
  - `assert.Contains(str, substr)` instead of `if !strings.Contains(str, substr) { t.Errorf(...) }`
- For table-driven tests, create `assert := require.New(t)` inside each `t.Run()` subtest

## Documentation Style
When creating any documentation (README files, code comments, design docs), write in the style of an Amazon engineer:
- Start with the customer problem and work backwards
- Use clear, concise, and data-driven language
- Include specific examples and concrete details
- Structure documents with clear headings and bullet points
- Focus on operational excellence, security, and scalability considerations
- Always include implementation details and edge cases
- Use the passive voice sparingly; prefer active, direct statements

## Breaking Changes Policy

**Pre-1.0 Development**: This library is currently in pre-1.0 development phase, which means:

- **Breaking changes are acceptable** and expected as the API evolves
- **All breaking changes must be clearly documented** in commit messages and release notes
- **Major API changes should be discussed** before implementation when possible