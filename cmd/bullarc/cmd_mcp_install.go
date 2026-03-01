package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Register bullarc as an MCP server in Claude Code",
	Long: `Adds a "bullarc" entry to the mcpServers section of ~/.claude.json so that
Claude Code automatically starts the bullarc MCP server. Environment variables
(MASSIVE_API_KEY, ALPACA_API_KEY, ALPACA_SECRET_KEY, ANTHROPIC_API_KEY) are
forwarded when set.`,
	RunE: runInstall,
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove bullarc MCP server from Claude Code",
	Long:  `Removes the "bullarc" entry from the mcpServers section of ~/.claude.json.`,
	RunE:  runUninstall,
}

// envKeys lists the environment variables forwarded to the MCP server entry.
var envKeys = []string{
	"MASSIVE_API_KEY",
	"ALPACA_API_KEY",
	"ALPACA_SECRET_KEY",
	"ANTHROPIC_API_KEY",
}

// claudeConfigPath returns the path to ~/.claude.json.
func claudeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

// readClaudeConfig reads and parses ~/.claude.json. If the file does not exist,
// an empty map is returned.
func readClaudeConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// writeClaudeConfig writes the config map back to path with 2-space indent.
func writeClaudeConfig(path string, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// resolveBinaryPath returns the absolute path to the current executable with
// symlinks resolved.
func resolveBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	return resolved, nil
}

func runInstall(_ *cobra.Command, _ []string) error {
	binPath, err := resolveBinaryPath()
	if err != nil {
		return err
	}

	cfgPath, err := claudeConfigPath()
	if err != nil {
		return err
	}

	m, err := readClaudeConfig(cfgPath)
	if err != nil {
		return err
	}

	// Build the env map with only non-empty values.
	envMap := make(map[string]any)
	for _, k := range envKeys {
		if v := os.Getenv(k); v != "" {
			envMap[k] = v
		}
	}

	entry := map[string]any{
		"type":    "stdio",
		"command": binPath,
		"args":    []any{"mcp"},
	}
	if len(envMap) > 0 {
		entry["env"] = envMap
	}

	// Ensure mcpServers exists at the top level.
	servers, ok := m["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}
	servers["bullarc"] = entry
	m["mcpServers"] = servers

	if err := writeClaudeConfig(cfgPath, m); err != nil {
		return err
	}

	// Print human-readable summary.
	fmt.Fprintf(os.Stdout, "\u2713 Installed bullarc MCP server in Claude Code (global)\n")
	fmt.Fprintf(os.Stdout, "  binary: %s\n", binPath)

	fmt.Fprintf(os.Stdout, "  env:   ")
	for i, k := range envKeys {
		if i > 0 {
			fmt.Fprintf(os.Stdout, "  ")
		}
		if os.Getenv(k) != "" {
			fmt.Fprintf(os.Stdout, " %s \u2713", k)
		} else {
			fmt.Fprintf(os.Stdout, " %s \u2717", k)
		}
	}
	fmt.Fprintln(os.Stdout)

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "  Available without API keys (6 tools):")
	fmt.Fprintln(os.Stdout, "    get_signals, compare_symbols, list_indicators,")
	fmt.Fprintln(os.Stdout, "    backtest_strategy, stream_signals, get_risk_metrics")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "  Require ANTHROPIC_API_KEY \u2014 not needed in Claude Code (4 tools):\n")
	fmt.Fprintln(os.Stdout, "    explain_signal, analyze_with_ai, explain_backtest, get_news_sentiment")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "  Restart Claude Code or run /mcp to connect.")

	return nil
}

func runUninstall(_ *cobra.Command, _ []string) error {
	cfgPath, err := claudeConfigPath()
	if err != nil {
		return err
	}

	m, err := readClaudeConfig(cfgPath)
	if err != nil {
		return err
	}

	servers, ok := m["mcpServers"].(map[string]any)
	if !ok {
		fmt.Fprintln(os.Stdout, "bullarc MCP server is not installed.")
		return nil
	}

	if _, exists := servers["bullarc"]; !exists {
		fmt.Fprintln(os.Stdout, "bullarc MCP server is not installed.")
		return nil
	}

	delete(servers, "bullarc")
	m["mcpServers"] = servers

	if err := writeClaudeConfig(cfgPath, m); err != nil {
		return err
	}

	fmt.Fprintln(os.Stdout, "\u2713 Removed bullarc MCP server from Claude Code (global)")
	fmt.Fprintln(os.Stdout, "  Restart Claude Code or run /mcp to apply.")

	return nil
}
