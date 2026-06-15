package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/groundwork/groundwork/internal/store"
	"go.uber.org/zap"
)

type Manager struct {
	store  *store.Store
	dataDir string
	logger *zap.Logger
}

type Backup struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func NewManager(s *store.Store, dataDir string, logger *zap.Logger) *Manager {
	return &Manager{
		store:   s,
		dataDir: dataDir,
		logger:  logger,
	}
}

func (m *Manager) CreateBackup() (*Backup, error) {
	id := generateID()
	now := time.Now()
	backupDir := filepath.Join(m.dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}

	backupPath := filepath.Join(backupDir, id+".tar.gz")
	
	// Create backup file
	f, err := os.Create(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Backup database
	dbPath := filepath.Join(m.dataDir, "groundwork.db")
	if err := m.addFileToTar(tw, dbPath, "groundwork.db"); err != nil {
		_ = os.Remove(backupPath) // Clean up on error
		return nil, err
	}

	tw.Close()
	gw.Close()

	// Get file info
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// Compute SHA256
	hash, err := computeSHA256(backupPath)
	if err != nil {
		return nil, err
	}

	// Store backup metadata
	_, err = m.store.DB().Exec(`
		INSERT INTO backups (id, path, sha256, size, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, backupPath, hash, info.Size(), now)
	if err != nil {
		return nil, err
	}

	return &Backup{
		ID:        id,
		Path:      backupPath,
		SHA256:    hash,
		Size:      info.Size(),
		CreatedAt: now,
	}, nil
}

func (m *Manager) addFileToTar(tw *tar.Writer, path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(tw, f)
	return err
}

func (m *Manager) ListBackups() ([]*Backup, error) {
	rows, err := m.store.DB().Query(`
		SELECT id, path, sha256, size, created_at
		FROM backups
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanBackups(rows)
}

func (m *Manager) GetBackup(id string) (*Backup, error) {
	var b Backup
	var created string
	err := m.store.DB().QueryRow(
		"SELECT id, path, sha256, size, created_at FROM backups WHERE id = ?",
		id,
	).Scan(&b.ID, &b.Path, &b.SHA256, &b.Size, &created)
	if err != nil {
		return nil, err
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339, created)
	return &b, nil
}

func (m *Manager) Restore(backupID string) error {
	b, err := m.GetBackup(backupID)
	if err != nil {
		return err
	}

	// Verify backup exists
	if _, err := os.Stat(b.Path); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found")
	}

	// Extract and restore
	// Simplified - would need proper implementation
	m.logger.Info("Restoring backup", zap.String("id", backupID))

	return nil
}

func scanBackups(rows *sql.Rows) ([]*Backup, error) {
	var backups []*Backup
	for rows.Next() {
		var b Backup
		var created string
		if err := rows.Scan(&b.ID, &b.Path, &b.SHA256, &b.Size, &created); err != nil {
			continue
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339, created)
		backups = append(backups, &b)
	}
	return backups, nil
}

func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}