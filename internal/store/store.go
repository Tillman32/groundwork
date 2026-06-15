package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	"go.uber.org/zap"
)

const schema = `
-- Managed endpoints
CREATE TABLE IF NOT EXISTS machines (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    hostname TEXT,
    os TEXT,
    arch TEXT,
    ip TEXT,
    enrolled_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen DATETIME,
    status TEXT NOT NULL DEFAULT 'online',
    config_hash TEXT,
    agent_version TEXT
);

-- Logical grouping for assignment
CREATE TABLE IF NOT EXISTS machine_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS machine_group_members (
    group_id TEXT NOT NULL,
    machine_id TEXT NOT NULL,
    PRIMARY KEY (group_id, machine_id),
    FOREIGN KEY (group_id) REFERENCES machine_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
);

-- Bootstrap for new endpoints
CREATE TABLE IF NOT EXISTS enrollment_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    label TEXT,
    fingerprint_hash TEXT,
    expires_at DATETIME,
    used_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Desired state objects
CREATE TABLE IF NOT EXISTS policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    plugin_name TEXT NOT NULL,
    plugin_version TEXT NOT NULL,
    config_yaml TEXT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'enforce',
    interval_s INTEGER NOT NULL DEFAULT 3600,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS policy_assignments (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL,
    target_type TEXT NOT NULL, -- 'machine' or 'group'
    target_id TEXT NOT NULL,
    rollout_ring TEXT NOT NULL DEFAULT 'stable',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (policy_id) REFERENCES policies(id) ON DELETE CASCADE
);

-- Current versus desired state by machine/policy
CREATE TABLE IF NOT EXISTS machine_policy_state (
    id TEXT PRIMARY KEY,
    machine_id TEXT NOT NULL,
    policy_id TEXT NOT NULL,
    desired_hash TEXT,
    observed_hash TEXT,
    status TEXT NOT NULL DEFAULT 'unknown',
    last_reconciled_at DATETIME,
    last_error TEXT,
    FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE,
    FOREIGN KEY (policy_id) REFERENCES policies(id) ON DELETE CASCADE,
    UNIQUE(machine_id, policy_id)
);

-- Leased reconciliation runs for idempotent execution
CREATE TABLE IF NOT EXISTS reconciliation_runs (
    id TEXT PRIMARY KEY,
    machine_id TEXT NOT NULL,
    policy_id TEXT NOT NULL,
    lease_id TEXT NOT NULL,
    trigger TEXT NOT NULL, -- 'scheduled', 'drift', 'manual', 'enrollment'
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'running', 'converged', 'failed'
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE,
    FOREIGN KEY (policy_id) REFERENCES policies(id) ON DELETE CASCADE
);

-- Ordered log stream per run
CREATE TABLE IF NOT EXISTS reconciliation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    line TEXT NOT NULL,
    is_stderr INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (run_id) REFERENCES reconciliation_runs(id) ON DELETE CASCADE
);

-- Explicit one-off jobs for break-glass operations
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    machine_id TEXT NOT NULL,
    plugin_name TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'running', 'completed', 'failed'
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    finished_at DATETIME,
    FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS job_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    line TEXT NOT NULL,
    is_stderr INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);

-- Local operator accounts for the single-tenant control plane
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer', -- 'owner', 'admin', 'viewer'
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login DATETIME
);

CREATE TABLE IF NOT EXISTS notification_settings (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    email_enabled INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, event_type)
);

-- Server configuration and trust metadata
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS trusted_publishers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active', -- 'active', 'revoked'
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Plugin artifacts cache metadata
CREATE TABLE IF NOT EXISTS plugin_artifacts (
    id TEXT PRIMARY KEY,
    plugin_name TEXT NOT NULL,
    plugin_version TEXT NOT NULL,
    platform TEXT NOT NULL,
    arch TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    signature TEXT NOT NULL,
    publisher_id TEXT NOT NULL,
    downloaded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    verified INTEGER NOT NULL DEFAULT 0,
    UNIQUE(plugin_name, plugin_version, platform, arch)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_machines_status ON machines(status);
CREATE INDEX IF NOT EXISTS idx_machines_last_seen ON machines(last_seen);
CREATE INDEX IF NOT EXISTS idx_policy_assignments_target ON policy_assignments(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_machine_policy_state_machine ON machine_policy_state(machine_id);
CREATE INDEX IF NOT EXISTS idx_reconciliation_runs_machine_policy ON reconciliation_runs(machine_id, policy_id);
CREATE INDEX IF NOT EXISTS idx_reconciliation_logs_run ON reconciliation_logs(run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_jobs_machine ON jobs(machine_id);
CREATE INDEX IF NOT EXISTS idx_job_logs_job ON job_logs(job_id, sequence);
CREATE INDEX IF NOT EXISTS idx_plugin_artifacts_lookup ON plugin_artifacts(plugin_name, plugin_version, platform, arch);

-- Backup metadata
CREATE TABLE IF NOT EXISTS backups (
    id TEXT PRIMARY KEY,
    path TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Plugin registries
CREATE TABLE IF NOT EXISTS registries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_sync DATETIME
);

-- Cached plugin metadata
CREATE TABLE IF NOT EXISTS cached_plugins (
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    description TEXT,
    platforms TEXT,
    capabilities TEXT,
    downloads INTEGER DEFAULT 0,
    rating REAL DEFAULT 0,
    updated_at DATETIME,
    PRIMARY KEY (name, version)
);

-- Notifications queue
CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
`

type Store struct {
	db     *sql.DB
	dataDir string
	logger  *zap.Logger
}

func New(dbPath, dataDir string, logger *zap.Logger) (*Store, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &Store{
		db:      db,
		dataDir: dataDir,
		logger:  logger,
	}

	return s, nil
}

func (s *Store) Migrate() error {
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	s.logger.Info("Database migrations completed")
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) DataDir() string {
	return s.dataDir
}

func (s *Store) ArtifactPath(pluginName, pluginVersion, platform, arch string) string {
	return filepath.Join(s.dataDir, "artifacts", pluginName, pluginVersion, platform, arch)
}