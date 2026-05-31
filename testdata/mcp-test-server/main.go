package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddInput defines the input parameters for the add tool
type AddInput struct {
	A float64 `json:"a" jsonschema:"first number"`
	B float64 `json:"b" jsonschema:"second number"`
}

// AddOutput defines the output for the add tool
type AddOutput struct {
	Result float64 `json:"result" jsonschema:"sum of a and b"`
}

// EchoInput defines the input parameters for the echo tool
type EchoInput struct {
	Message string `json:"message" jsonschema:"message to echo back"`
}

// EchoOutput defines the output for the echo tool
type EchoOutput struct {
	Echoed string `json:"echoed" jsonschema:"the echoed message"`
}

// TimeOutput defines the output for the get_current_time tool
type TimeOutput struct {
	Time   string `json:"time" jsonschema:"current time"`
	Format string `json:"format" jsonschema:"time format used"`
}

// GetEnvInput defines the input parameters for the get_env tool
type GetEnvInput struct {
	Name string `json:"name" jsonschema:"name of the environment variable to retrieve"`
}

// GetEnvOutput defines the output for the get_env tool
type GetEnvOutput struct {
	Name  string `json:"name" jsonschema:"name of the environment variable"`
	Value string `json:"value" jsonschema:"value of the environment variable, or empty if not set"`
	Set   bool   `json:"set" jsonschema:"whether the environment variable is set"`
}

// GetUserInput defines the input parameters for the get_user tool
type GetUserInput struct {
	UserID string `json:"user_id" jsonschema:"user ID to retrieve"`
}

// UserInfo defines the output for the get_user tool (Buildkite-style API response)
type UserInfo struct {
	ID        string   `json:"id" jsonschema:"unique user identifier"`
	Name      string   `json:"name" jsonschema:"full name of the user"`
	Email     string   `json:"email" jsonschema:"email address"`
	CreatedAt string   `json:"created_at" jsonschema:"user creation timestamp in RFC3339 format"`
	AvatarURL string   `json:"avatar_url" jsonschema:"URL to user's avatar image"`
	Teams     []string `json:"teams" jsonschema:"list of teams the user belongs to"`
}

// GetSystemLogsInput defines the input parameters for the get_system_logs tool
type GetSystemLogsInput struct {
	ServiceName string `json:"service_name" jsonschema:"name of the service to retrieve logs from"`
	Level       string `json:"level,omitempty" jsonschema:"log level filter (info, warn, error). Optional, defaults to all levels"`
	Lines       int    `json:"lines,omitempty" jsonschema:"number of recent log lines to retrieve. Optional, defaults to 10"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp string `json:"timestamp" jsonschema:"log entry timestamp in RFC3339 format"`
	Level     string `json:"level" jsonschema:"log level (info, warn, error)"`
	Message   string `json:"message" jsonschema:"log message content"`
	Service   string `json:"service" jsonschema:"service that generated the log"`
}

// GetSystemLogsOutput defines the output for the get_system_logs tool
type GetSystemLogsOutput struct {
	Service string     `json:"service" jsonschema:"name of the service"`
	Logs    []LogEntry `json:"logs" jsonschema:"array of log entries"`
	Count   int        `json:"count" jsonschema:"number of log entries returned"`
}

// Add adds two numbers together
func Add(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, AddOutput, error) {
	return nil, AddOutput{Result: input.A + input.B}, nil
}

// Echo echoes back the input message
func Echo(ctx context.Context, req *mcp.CallToolRequest, input EchoInput) (*mcp.CallToolResult, EchoOutput, error) {
	return nil, EchoOutput{Echoed: input.Message}, nil
}

// GetCurrentTime returns the current time in RFC3339 format
func GetCurrentTime(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, TimeOutput, error) {
	now := time.Now()
	return nil, TimeOutput{
		Time:   now.Format(time.RFC3339),
		Format: "RFC3339",
	}, nil
}

// GetEnv retrieves an environment variable value
func GetEnv(ctx context.Context, req *mcp.CallToolRequest, input GetEnvInput) (*mcp.CallToolResult, GetEnvOutput, error) {
	value, set := os.LookupEnv(input.Name)
	return nil, GetEnvOutput{
		Name:  input.Name,
		Value: value,
		Set:   set,
	}, nil
}

// GetUser retrieves user information (simulates Buildkite-style API)
func GetUser(ctx context.Context, req *mcp.CallToolRequest, input GetUserInput) (*mcp.CallToolResult, UserInfo, error) {
	// Simulate realistic API responses for test users
	users := map[string]UserInfo{
		"user-123": {
			ID:        "user-123",
			Name:      "Alice Johnson",
			Email:     "alice@example.com",
			CreatedAt: "2024-01-15T10:30:00Z",
			AvatarURL: "https://avatars.example.com/alice",
			Teams:     []string{"engineering", "platform", "devops"},
		},
		"user-456": {
			ID:        "user-456",
			Name:      "Bob Smith",
			Email:     "bob@example.com",
			CreatedAt: "2024-02-20T14:45:00Z",
			AvatarURL: "https://avatars.example.com/bob",
			Teams:     []string{"engineering", "frontend"},
		},
	}

	// Return user if found, otherwise return empty user with just the ID
	if user, exists := users[input.UserID]; exists {
		return nil, user, nil
	}

	// User not found - return minimal info
	return nil, UserInfo{
		ID:        input.UserID,
		Name:      "Unknown User",
		Email:     "",
		CreatedAt: "",
		AvatarURL: "",
		Teams:     []string{},
	}, nil
}

// GetSystemLogs retrieves system logs for a service (simulates log aggregation system)
func GetSystemLogs(ctx context.Context, req *mcp.CallToolRequest, input GetSystemLogsInput) (*mcp.CallToolResult, GetSystemLogsOutput, error) {
	// Simulate realistic log data for different services
	serviceLogs := map[string][]LogEntry{
		"api-gateway": {
			{Timestamp: "2024-12-15T10:25:30Z", Level: "error", Message: "Connection timeout to backend service 'user-service' after 30s", Service: "api-gateway"},
			{Timestamp: "2024-12-15T10:25:25Z", Level: "warn", Message: "Retrying connection to backend service 'user-service' (attempt 3/3)", Service: "api-gateway"},
			{Timestamp: "2024-12-15T10:25:20Z", Level: "warn", Message: "Retrying connection to backend service 'user-service' (attempt 2/3)", Service: "api-gateway"},
			{Timestamp: "2024-12-15T10:25:15Z", Level: "warn", Message: "Retrying connection to backend service 'user-service' (attempt 1/3)", Service: "api-gateway"},
			{Timestamp: "2024-12-15T10:25:10Z", Level: "error", Message: "Failed to connect to backend service 'user-service': connection refused", Service: "api-gateway"},
			{Timestamp: "2024-12-15T10:20:00Z", Level: "info", Message: "Request processed successfully: GET /api/v1/users/123", Service: "api-gateway"},
			{Timestamp: "2024-12-15T10:15:00Z", Level: "info", Message: "Request processed successfully: POST /api/v1/orders", Service: "api-gateway"},
		},
		"user-service": {
			{Timestamp: "2024-12-15T10:24:55Z", Level: "error", Message: "Database connection pool exhausted: max 10 connections reached", Service: "user-service"},
			{Timestamp: "2024-12-15T10:24:50Z", Level: "error", Message: "Query timeout: SELECT * FROM users WHERE id = '123' (timeout: 5s)", Service: "user-service"},
			{Timestamp: "2024-12-15T10:24:45Z", Level: "warn", Message: "High database connection usage: 9/10 connections in use", Service: "user-service"},
			{Timestamp: "2024-12-15T10:24:40Z", Level: "warn", Message: "High database connection usage: 8/10 connections in use", Service: "user-service"},
			{Timestamp: "2024-12-15T10:20:30Z", Level: "info", Message: "User authenticated successfully: user-123", Service: "user-service"},
			{Timestamp: "2024-12-15T10:15:30Z", Level: "info", Message: "User profile retrieved successfully: user-456", Service: "user-service"},
		},
		"database": {
			{Timestamp: "2024-12-15T10:24:58Z", Level: "error", Message: "Too many connections: current=150, max=100", Service: "database"},
			{Timestamp: "2024-12-15T10:24:55Z", Level: "error", Message: "Connection rejected: connection limit exceeded", Service: "database"},
			{Timestamp: "2024-12-15T10:24:50Z", Level: "warn", Message: "High connection count: 95/100 connections active", Service: "database"},
			{Timestamp: "2024-12-15T10:20:00Z", Level: "info", Message: "Query executed successfully: SELECT * FROM users", Service: "database"},
		},
	}

	// Default to 10 lines if not specified
	lines := input.Lines
	if lines <= 0 {
		lines = 10
	}

	// Get logs for the requested service
	logs, exists := serviceLogs[input.ServiceName]
	if !exists {
		// Service not found - return empty logs
		return nil, GetSystemLogsOutput{
			Service: input.ServiceName,
			Logs:    []LogEntry{},
			Count:   0,
		}, nil
	}

	// Filter by log level if specified
	filteredLogs := logs
	if input.Level != "" {
		filtered := []LogEntry{}
		for _, log := range logs {
			if log.Level == input.Level {
				filtered = append(filtered, log)
			}
		}
		filteredLogs = filtered
	}

	// Limit to requested number of lines
	if len(filteredLogs) > lines {
		filteredLogs = filteredLogs[:lines]
	}

	return nil, GetSystemLogsOutput{
		Service: input.ServiceName,
		Logs:    filteredLogs,
		Count:   len(filteredLogs),
	}, nil
}

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-mcp-server",
		Version: "v1.0.0",
	}, &mcp.ServerOptions{
		Instructions: "Use the test MCP server tools to answer evaluation prompts. Prefer get_user for user profile questions and get_system_logs for log troubleshooting questions.",
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add",
		Description: "adds two numbers together",
	}, Add)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echoes back the input message",
	}, Echo)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_current_time",
		Description: "returns the current time",
	}, GetCurrentTime)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_env",
		Description: "retrieves an environment variable value",
	}, GetEnv)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_user",
		Description: "retrieves user information from the system, including ID, name, email, creation date, avatar URL, and team memberships",
	}, GetUser)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_system_logs",
		Description: "retrieves recent system logs for a service. Can filter by log level (info, warn, error) and limit number of lines returned. Useful for troubleshooting and debugging service issues",
	}, GetSystemLogs)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
