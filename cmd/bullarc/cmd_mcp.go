package main

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc/internal/mcp"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server over stdio",
	Long: `Starts a Model Context Protocol (MCP) server that exposes bullarc tools
over stdio transport. Connect any MCP client (Claude Desktop, Cursor, etc.)
to this process to run backtests and inspect indicators programmatically.

Available tools:
  backtest_strategy  Run a backtest on historical CSV data
  list_indicators    List all registered technical indicators`,
	RunE: runMCP,
}

var mcpConfig string

func init() {
	mcpCmd.Flags().StringVarP(&mcpConfig, "config", "c", "", "path to config file")
}

func runMCP(cmd *cobra.Command, _ []string) error {
	e, err := buildEngine(mcpConfig, "", "")
	if err != nil {
		return fmt.Errorf("build engine: %w", err)
	}

	srv := mcp.New("bullarc", "0.1.0")
	mcp.RegisterTools(srv, e)

	slog.Info("mcp: server started", "transport", "stdio", "tools", 2)
	return srv.Serve(cmd.Context())
}
