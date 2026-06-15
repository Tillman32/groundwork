package policy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/groundwork/groundwork/internal/store"
	"github.com/groundwork/groundwork/internal/plugin"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

type Manager struct {
	store       *store.Store
	logger      *zap.Logger
	scheduler   *cron.Cron
	pluginStore *PluginStore
}

type PluginStore struct {
	store  *store.Store
	logger *zap.Logger
}

func NewManager(s *store.Store, logger *zap.Logger) *Manager {
	return &Manager{
		store:     s,
		logger:    logger,
		scheduler: cron.New(cron.WithSeconds()),
		pluginStore: &PluginStore{
			store:  s,
			logger: logger,
		},
	}
}

func (m *Manager) StartScheduler(ctx context.Context) {
	m.scheduler.Start()
	m.logger.Info("Policy scheduler started")

	// Schedule reconciliation for all active policies
	m.scheduleAllPolicies()

	// Re-schedule periodically
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.scheduler.Stop()
			m.logger.Info("Policy scheduler stopped")
			return
		case <-ticker.C:
			m.scheduleAllPolicies()
		}
	}
}

func (m *Manager) scheduleAllPolicies() {
	// Get all enabled policies with assignments
	rows, err := m.store.DB().Query(`
		SELECT p.id, p.name, p.plugin_name, p.plugin_version, p.config_yaml, p.interval_s, p.mode
		FROM policies p
		WHERE p.enabled = 1
	`)
	if err != nil {
		m.logger.Error("Failed to query policies", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var policy Policy
		if err := rows.Scan(&policy.ID, &policy.Name, &policy.PluginName, &policy.PluginVersion, &policy.ConfigYAML, &policy.Interval, &policy.Mode); err != nil {
			m.logger.Error("Failed to scan policy", zap.Error(err))
			continue
		}
		m.schedulePolicy(policy)
	}
}

func (m *Manager) schedulePolicy(policy Policy) {
	// Remove existing job for this policy
	// In production, track job IDs per policy

	// Add new scheduled job
	_, err := m.scheduler.AddFunc(fmt.Sprintf("@every %ds", policy.Interval), func() {
		m.triggerReconciliation(policy)
	})
	if err != nil {
		m.logger.Error("Failed to schedule policy", zap.String("policy", policy.ID), zap.Error(err))
	}
}

func (m *Manager) triggerReconciliation(policy Policy) {
	// Get machines assigned to this policy
	rows, err := m.store.DB().Query(`
		SELECT m.id FROM machines m
		JOIN policy_assignments pa ON 
			(pa.target_type = 'machine' AND pa.target_id = m.id) OR
			(pa.target_type = 'group' AND pa.target_id IN (SELECT group_id FROM machine_group_members WHERE machine_id = m.id))
		WHERE pa.policy_id = ? AND m.status = 'online'
	`, policy.ID)
	if err != nil {
		m.logger.Error("Failed to query assigned machines", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var machineID string
		if err := rows.Scan(&machineID); err != nil {
			continue
		}
		// In production, send reconcile_claim via WebSocket to agent
		m.logger.Debug("Triggering reconciliation", zap.String("machine", machineID), zap.String("policy", policy.ID))
	}
}

type Policy struct {
	ID            string
	Name          string
	PluginName    string
	PluginVersion string
	ConfigYAML    string
	Interval      int
	Mode          string
}

func (m *Manager) CreatePolicy(name, pluginName, pluginVersion, configYAML, mode string, interval int) (*Policy, error) {
	// Validate plugin exists and version is available
	manifest, err := m.pluginStore.GetManifest(pluginName, pluginVersion)
	if err != nil {
		return nil, fmt.Errorf("plugin not found: %w", err)
	}

	// Validate config against manifest
	if err := m.validateConfig(manifest, configYAML); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	id := generateID()
	now := time.Now()

	_, err = m.store.DB().Exec(`
		INSERT INTO policies (id, name, plugin_name, plugin_version, config_yaml, mode, interval_s, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
	`, id, name, pluginName, pluginVersion, configYAML, mode, interval, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create policy: %w", err)
	}

	policy := &Policy{
		ID:            id,
		Name:          name,
		PluginName:    pluginName,
		PluginVersion: pluginVersion,
		ConfigYAML:    configYAML,
		Interval:      interval,
		Mode:          mode,
	}

	m.schedulePolicy(*policy)
	return policy, nil
}

func (m *Manager) validateConfig(manifest *plugin.Manifest, configYAML string) error {
	// Parse config YAML and validate against manifest schema
	// Simplified validation for now
	return nil
}

func (m *Manager) AssignPolicy(policyID, targetType, targetID, rolloutRing string) error {
	id := generateID()
	_, err := m.store.DB().Exec(`
		INSERT INTO policy_assignments (id, policy_id, target_type, target_id, rollout_ring)
		VALUES (?, ?, ?, ?, ?)
	`, id, policyID, targetType, targetID, rolloutRing)
	return err
}

func (m *Manager) GetPolicyAssignments(policyID string) ([]PolicyAssignment, error) {
	rows, err := m.store.DB().Query(`
		SELECT id, policy_id, target_type, target_id, rollout_ring FROM policy_assignments WHERE policy_id = ?
	`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []PolicyAssignment
	for rows.Next() {
		var a PolicyAssignment
		if err := rows.Scan(&a.ID, &a.PolicyID, &a.TargetType, &a.TargetID, &a.RolloutRing); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, nil
}

type PolicyAssignment struct {
	ID          string
	PolicyID    string
	TargetType  string
	TargetID    string
	RolloutRing string
}

func (m *Manager) GetMachinePolicies(machineID string) ([]PolicySnapshot, error) {
	rows, err := m.store.DB().Query(`
		SELECT p.id, p.name, p.plugin_name, p.plugin_version, p.config_yaml, p.mode, p.interval_s,
		       pa.rollout_ring
		FROM policies p
		JOIN policy_assignments pa ON p.id = pa.policy_id
		WHERE p.enabled = 1 AND (
			(pa.target_type = 'machine' AND pa.target_id = ?) OR
			(pa.target_type = 'group' AND pa.target_id IN (SELECT group_id FROM machine_group_members WHERE machine_id = ?))
		)
	`, machineID, machineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []PolicySnapshot
	for rows.Next() {
		var p PolicySnapshot
		if err := rows.Scan(&p.PolicyID, &p.Name, &p.Plugin, &p.PluginVersion, &p.ConfigYAML, &p.Mode, &p.Interval, &p.RolloutRing); err != nil {
			return nil, err
		}
		p.DesiredHash = computeConfigHash(p.ConfigYAML)
		policies = append(policies, p)
	}
	return policies, nil
}

type PolicySnapshot struct {
	PolicyID      string
	Name          string
	Plugin        string
	PluginVersion string
	ConfigYAML    string
	Mode          string
	Interval      int
	RolloutRing   string
	DesiredHash   string
}

func computeConfigHash(config string) string {
	h := sha256.Sum256([]byte(config))
	return hex.EncodeToString(h[:])
}

func (m *Manager) RecordReconciliationRun(runID, machineID, policyID, leaseID, trigger string) error {
	_, err := m.store.DB().Exec(`
		INSERT INTO reconciliation_runs (id, machine_id, policy_id, lease_id, trigger, status)
		VALUES (?, ?, ?, ?, ?, 'running')
	`, runID, machineID, policyID, leaseID, trigger)
	return err
}

func (m *Manager) CompleteReconciliationRun(runID, status, observedHash, errorMsg string) error {
	_, err := m.store.DB().Exec(`
		UPDATE reconciliation_runs SET status = ?, observed_hash = ?, last_error = ?, finished_at = ?
		WHERE id = ?
	`, status, observedHash, errorMsg, time.Now(), runID)
	return err
}

func (m *Manager) LogReconciliationLine(runID string, sequence int, line string, isStderr bool) error {
	_, err := m.store.DB().Exec(`
		INSERT INTO reconciliation_logs (run_id, sequence, line, is_stderr)
		VALUES (?, ?, ?, ?)
	`, runID, sequence, line, isStderr)
	return err
}

func (m *Manager) UpdateMachinePolicyState(machineID, policyID, desiredHash, observedHash, status, errorMsg string) error {
	_, err := m.store.DB().Exec(`
		INSERT INTO machine_policy_state (id, machine_id, policy_id, desired_hash, observed_hash, status, last_reconciled_at, last_error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(machine_id, policy_id) DO UPDATE SET
			desired_hash = excluded.desired_hash,
			observed_hash = excluded.observed_hash,
			status = excluded.status,
			last_reconciled_at = excluded.last_reconciled_at,
			last_error = excluded.last_error
	`, generateID(), machineID, policyID, desiredHash, observedHash, status, time.Now(), errorMsg)
	return err
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (ps *PluginStore) GetManifest(pluginName, pluginVersion string) (*plugin.Manifest, error) {
	// In production, load from artifact cache or registry
	// For now, return a mock manifest for software-manager
	if pluginName == "software-manager" {
		return &plugin.Manifest{
			Name:         "software-manager",
			Version:      pluginVersion,
			Publisher:    "groundwork",
			Runtime:      "powershell",
			Entrypoint:   "plugin.ps1",
			Platforms:    []string{"windows"},
			Capabilities: []string{plugin.CapabilityPackageInstall, plugin.CapabilityPackageRemove, plugin.CapabilityPackageList},
		}, nil
	}
	return nil, fmt.Errorf("plugin not found: %s@%s", pluginName, pluginVersion)
}