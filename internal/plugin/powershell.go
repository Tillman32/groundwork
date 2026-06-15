package plugin

import (
	"bufio"
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

type PowerShellRuntime struct {
	logger *zap.Logger
}

func NewPowerShellRuntime(logger *zap.Logger) *PowerShellRuntime {
	return &PowerShellRuntime{logger: logger}
}

type PSEecutionResult struct {
	ObservedHash string
	Status       string // "converged", "failed", "changed"
	Output       []PSLogLine
	Error        string
}

type PSLogLine struct {
	Sequence int
	Line     string
	IsStderr bool
	Time     time.Time
}

type PSPluginConfig struct {
	PolicyID      string
	PluginName    string
	PluginVersion string
	Config        map[string]any
	DesiredHash   string
}

func (r *PowerShellRuntime) Execute(ctx context.Context, pluginDir string, manifest *Manifest, config *PSPluginConfig) (*PSEecutionResult, error) {
	configPath := filepath.Join(pluginDir, "policy.yaml")
	configYAML, err := MarshalPSConfig(config.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		return nil, fmt.Errorf("failed to write config: %w", err)
	}

	scriptPath := filepath.Join(pluginDir, manifest.Entrypoint)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin entrypoint not found: %s", scriptPath)
	}

	cmd := exec.CommandContext(ctx, "powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-Config", configPath)
	cmd.Dir = pluginDir

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

	result := &PSEecutionResult{
		Status: "running",
		Output: []PSLogLine{},
	}

	done := make(chan error, 2)
	go func() {
		scanner := bufio.NewScanner(stdout)
		seq := 0
		for scanner.Scan() {
			line := scanner.Text()
			result.Output = append(result.Output, PSLogLine{
				Sequence: seq,
				Line:     line,
				IsStderr: false,
				Time:     time.Now(),
			})
			seq++
			if parsed := r.parseOutputLine(line); parsed != nil {
				if parsed.ObservedHash != "" {
					result.ObservedHash = parsed.ObservedHash
				}
				if parsed.Status != "" {
					result.Status = parsed.Status
				}
			}
		}
		done <- scanner.Err()
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		seq := 0
		for scanner.Scan() {
			line := scanner.Text()
			result.Output = append(result.Output, PSLogLine{
				Sequence: seq,
				Line:     line,
				IsStderr: true,
				Time:     time.Now(),
			})
			seq++
		}
		done <- scanner.Err()
	}()

	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			r.logger.Warn("Plugin stream error", zap.Error(err))
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

type PSParsedOutput struct {
	ObservedHash string
	Status       string
}

func (r *PowerShellRuntime) parseOutputLine(line string) *PSParsedOutput {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") {
		return nil
	}

	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil
	}

	result := &PSParsedOutput{}
	if v, ok := msg["observed_hash"].(string); ok {
		result.ObservedHash = v
	}
	if v, ok := msg["status"].(string); ok {
		result.Status = v
	}
	return result
}

func MarshalPSConfig(config map[string]any) (string, error) {
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