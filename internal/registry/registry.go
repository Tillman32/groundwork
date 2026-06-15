package registry

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/groundwork/groundwork/internal/store"
	"go.uber.org/zap"
)

type Manager struct {
	store  *store.Store
	logger *zap.Logger
}

type PluginRegistry struct {
	ID       string
	Name     string
	URL      string
	APIKey   string
	Created  time.Time
	LastSync time.Time
}

type RemotePlugin struct {
	Name         string
	Version      string
	Description  string
	Platforms    []string
	Runtimes     []string
	Capabilities []string
	Downloads    int
	Rating       float64
	UpdatedAt    time.Time
}

func NewManager(s *store.Store, logger *zap.Logger) *Manager {
	return &Manager{
		store:  s,
		logger: logger,
	}
}

func (m *Manager) AddRegistry(name, url string) (*PluginRegistry, error) {
	reg := &PluginRegistry{
		ID:     generateID(),
		Name:   name,
		URL:    url,
		Created: time.Now(),
	}

	_, err := m.store.DB().Exec(`
		INSERT INTO registries (id, name, url, created_at)
		VALUES (?, ?, ?, ?)
	`, reg.ID, reg.Name, reg.URL, reg.Created)
	if err != nil {
		return nil, fmt.Errorf("failed to add registry: %w", err)
	}

	return reg, nil
}

func (m *Manager) ListRegistries() ([]*PluginRegistry, error) {
	rows, err := m.store.DB().Query(`
		SELECT id, name, url, created_at, last_sync
		FROM registries
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var registries []*PluginRegistry
	for rows.Next() {
		var r PluginRegistry
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.Created, &r.LastSync); err != nil {
			return nil, err
		}
		registries = append(registries, &r)
	}
	return registries, nil
}

func (m *Manager) SyncRegistry(registryID string) error {
	var reg PluginRegistry
	err := m.store.DB().QueryRow(`
		SELECT id, name, url FROM registries WHERE id = ?
	`, registryID).Scan(&reg.ID, &reg.Name, &reg.URL)
	if err != nil {
		return fmt.Errorf("registry not found: %w", err)
	}

	resp, err := http.Get(reg.URL + "/api/v1/plugins")
	if err != nil {
		return fmt.Errorf("failed to fetch plugins: %w", err)
	}
	defer resp.Body.Close()

	var plugins []RemotePlugin
	if err := json.NewDecoder(resp.Body).Decode(&plugins); err != nil {
		return fmt.Errorf("failed to decode plugin list: %w", err)
	}

	for _, p := range plugins {
		_ = m.cachePlugin(p)
	}

	_, err = m.store.DB().Exec(`
		UPDATE registries SET last_sync = ? WHERE id = ?
	`, time.Now(), registryID)

	return err
}

func (m *Manager) SearchPlugins(query string) ([]*RemotePlugin, error) {
	rows, err := m.store.DB().Query(`
		SELECT name, version, description, platforms, capabilities, downloads, rating, updated_at
		FROM cached_plugins
		WHERE name LIKE ? OR description LIKE ?
		ORDER BY rating DESC, downloads DESC
		LIMIT 50
	`, "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPlugins(rows)
}

func (m *Manager) GetPlugin(name, version string) (*RemotePlugin, error) {
	var p RemotePlugin
	var platformsJSON, capsJSON string

	err := m.store.DB().QueryRow(`
		SELECT name, version, description, platforms, capabilities, downloads, rating, updated_at
		FROM cached_plugins WHERE name = ? AND version = ?
	`, name, version).Scan(
		&p.Name, &p.Version, &p.Description,
		&platformsJSON, &capsJSON,
		&p.Downloads, &p.Rating, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	_ = json.Unmarshal([]byte(platformsJSON), &p.Platforms)
	_ = json.Unmarshal([]byte(capsJSON), &p.Capabilities)

	return &p, nil
}

func (m *Manager) DownloadPlugin(name, version, platform, arch, publisherID string) error {
	url := "https://registry.groundwork.local/api/v1/plugins/" + name + "/" + version + "/download?platform=" + platform + "&arch=" + arch

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dir := filepath.Join(m.store.DataDir(), "artifacts", name, version, platform, arch)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "plugin"))
	if err != nil {
		return err
	}
	defer f.Close()
	_, _ = io.Copy(f, resp.Body)
	return nil
}

func (m *Manager) cachePlugin(p RemotePlugin) error {
	platformsJSON, _ := json.Marshal(p.Platforms)
	capsJSON, _ := json.Marshal(p.Capabilities)

	_, err := m.store.DB().Exec(`
		INSERT OR REPLACE INTO cached_plugins
		(name, version, description, platforms, capabilities, downloads, rating, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, p.Name, p.Version, p.Description, platformsJSON, capsJSON,
		p.Downloads, p.Rating, p.UpdatedAt)
	return err
}

func scanPlugins(rows *sql.Rows) ([]*RemotePlugin, error) {
	var plugins []*RemotePlugin
	for rows.Next() {
		var p RemotePlugin
		var platformsJSON, capsJSON string
		if err := rows.Scan(&p.Name, &p.Version, &p.Description,
			&platformsJSON, &capsJSON, &p.Downloads, &p.Rating, &p.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(platformsJSON), &p.Platforms)
		_ = json.Unmarshal([]byte(capsJSON), &p.Capabilities)
		plugins = append(plugins, &p)
	}
	return plugins, nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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