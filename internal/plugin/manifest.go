package plugin

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Name         string          `yaml:"name"`
	Version      string          `yaml:"version"`
	Publisher    string          `yaml:"publisher"`
	Description  string          `yaml:"description"`
	Platforms    []string        `yaml:"platforms"`
	Runtime      string          `yaml:"runtime"` // "powershell" or "gobinary"
	Entrypoint   string          `yaml:"entrypoint"`
	Capabilities []string        `yaml:"capabilities"`
	Config       []ConfigField   `yaml:"config"`
	Reconcile    ReconcileConfig `yaml:"reconcile"`
}

type ConfigField struct {
	Key         string   `yaml:"key"`
	Type        string   `yaml:"type"` // string, number, boolean, select, list, password
	Label       string   `yaml:"label"`
	Description string   `yaml:"description"`
	Required    bool     `yaml:"required"`
	Default     any      `yaml:"default"`
	Options     []string `yaml:"options"` // for select type
}

type ReconcileConfig struct {
	Strategy string `yaml:"strategy"` // "converge"
	Interval int    `yaml:"interval"` // seconds
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest: version is required")
	}
	if m.Publisher == "" {
		return fmt.Errorf("manifest: publisher is required")
	}
	if m.Runtime == "" {
		return fmt.Errorf("manifest: runtime is required")
	}
	if m.Entrypoint == "" {
		return fmt.Errorf("manifest: entrypoint is required")
	}
	if len(m.Platforms) == 0 {
		return fmt.Errorf("manifest: at least one platform is required")
	}
	if m.Runtime != "powershell" && m.Runtime != "gobinary" {
		return fmt.Errorf("manifest: runtime must be 'powershell' or 'gobinary'")
	}
	for _, cap := range m.Capabilities {
		if cap == "" {
			return fmt.Errorf("manifest: capability cannot be empty")
		}
	}
	for _, field := range m.Config {
		if field.Key == "" {
			return fmt.Errorf("manifest: config field key is required")
		}
		if field.Type == "" {
			return fmt.Errorf("manifest: config field type is required for %s", field.Key)
		}
	}
	if m.Reconcile.Strategy == "" {
		m.Reconcile.Strategy = "converge"
	}
	if m.Reconcile.Interval == 0 {
		m.Reconcile.Interval = 3600
	}
	return nil
}

func (m *Manifest) ConfigSchema() map[string]ConfigField {
	schema := make(map[string]ConfigField)
	for _, field := range m.Config {
		schema[field.Key] = field
	}
	return schema
}