package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

type GoBinaryRuntime struct {
	logger *zap.Logger
}

func NewGoBinaryRuntime(logger *zap.Logger) *GoBinaryRuntime {
	return &GoBinaryRuntime{logger: logger}
}

type ExecutionResult struct {
	ObservedHash string
	Status       string
	Output       []LogLine
	Error        string
}

type LogLine struct {
	Sequence int
	Line     string
	IsStderr bool
	Time     time.Time
}

type PluginConfig struct {
	PolicyID       string
	PluginName     string
	PluginVersion  string
	Config         map[string]any
	DesiredHash    string
}

func (r *GoBinaryRuntime) Execute(ctx context.Context, pluginDir string, manifest *Manifest, config *PluginConfig) (*ExecutionResult, error) {
	// Write config to temp file
	configPath := filepath.Join(pluginDir, "policy.yaml")
	configYAML, err := MarshalConfig(config.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	// Prepare plugin binary path
	binaryPath := filepath.Join(pluginDir, manifest.Entrypoint)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin binary not found: %s", binaryPath)
	}

	// Ensure binary is executable
	if err := os.Chmod(binaryPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to make binary executable: %w", err)
	}

	// Execute Go plugin binary
	cmd := exec.CommandContext(ctx, binaryPath, "-config", configPath)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(),
		"GROUNDWORK_POLICY_ID="+config.PolicyID,
		"GROUNDWORK_PLUGIN_NAME="+config.PluginName,
		"GROUNDWORK_PLUGIN_VERSION="+config.PluginVersion,
		"GROUNDWORK_DESIRED_HASH="+config.DesiredHash,
	)

	result := &ExecutionResult{
		Status: "running",
		Output: []LogLine{},
	}

	// For Go plugins, we expect JSON output on stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start plugin: %w", err)
	}

	// Read output
	done := make(chan error, 2)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				result.Output = append(result.Output, LogLine{
					Sequence: len(result.Output),
					Line:     line,
					IsStderr: false,
					Time:     time.Now(),
				})
				if parsed := r.parseOutputLine(line); parsed != nil {
					if parsed.ObservedHash != "" {
						result.ObservedHash = parsed.ObservedHash
					}
					if parsed.Status != "" {
						result.Status = parsed.Status
					}
				}
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				line := string(buf[:n])
				result.Output = append(result.Output, LogLine{
					Sequence: len(result.Output),
					Line:     line,
					IsStderr: true,
					Time:     time.Now(),
				})
			}
			if err != nil {
				done <- err
				return
			}
		}
	}()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			// EOF is expected
		}
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.Status = "failed"
			result.Error = fmt.Sprintf("plugin exited with code %d", exitErr.ExitCode())
		} else {
			result.Status = "failed"
			result.Error = err.Error()
		}
	} else if result.Status == "running" {
		result.Status = "converged"
	}

	return result, nil
}

type parsedOutput struct {
	ObservedHash string
	Status       string
}

func (r *GoBinaryRuntime) parseOutputLine(line string) *parsedOutput {
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}

	result := &parsedOutput{}
	if v, ok := msg["observed_hash"].(string); ok {
		result.ObservedHash = v
	}
	if v, ok := msg["status"].(string); ok {
		result.Status = v
	}
	return result
}

func MarshalConfig(config map[string]any) (string, error) {
	var lines []string
	for k, v := range config {
		switch val := v.(type) {
		case string:
			lines = append(lines, fmt.Sprintf("%s: \"%s\"", k, val))
		case int, int64, float64:
			lines = append(lines, fmt.Sprintf("%s: %v", k, val))
		case bool:
			lines = append(lines, fmt.Sprintf("%s: %v", k, val))
		case []any:
			lines = append(lines, fmt.Sprintf("%s:", k))
			for _, item := range val {
				lines = append(lines, fmt.Sprintf("  - \"%v\"", item))
			}
		default:
			lines = append(lines, fmt.Sprintf("%s: \"%v\"", k, val))
		}
	}
	return strings.Join(lines, "\n"), nil
}