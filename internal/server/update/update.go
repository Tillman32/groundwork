package update

import (
	"crypto/rand"
	"crypto/sha256"
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

type UpdateCheck struct {
	ID         string    `json:"id"`
	PluginName string    `json:"plugin_name"`
	Current    string    `json:"current_version"`
	Latest     string    `json:"latest_version"`
	Available  bool      `json:"available"`
	CheckedAt  time.Time `json:"checked_at"`
}

type PluginVersion struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Changelog string `json:"changelog"`
	URL       string `json:"url"`
}

func NewManager(s *store.Store, logger *zap.Logger) *Manager {
	return &Manager{
		store:  s,
		logger: logger,
	}
}

func (m *Manager) CheckForUpdates(pluginName, currentVersion, platform, arch string) (*UpdateCheck, error) {
	// Check registry for latest version
	resp, err := http.Get(m.registryURL() + "/api/v1/plugins/" + pluginName + "/latest?platform=" + platform + "&arch=" + arch)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var latest PluginVersion
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return nil, err
	}

	available := latest.Version != currentVersion

	return &UpdateCheck{
		ID:         generateID(),
		PluginName: pluginName,
		Current:    currentVersion,
		Latest:     latest.Version,
		Available:  available,
		CheckedAt:  time.Now(),
	}, nil
}

func (m *Manager) DownloadUpdate(pluginName, version, platform, arch string) error {
	// Download plugin artifact
	url := m.registryURL() + "/api/v1/plugins/" + pluginName + "/" + version + "/download?platform=" + platform + "&arch=" + arch

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Save to artifact cache
	dir := filepath.Join(m.store.DataDir(), "artifacts", pluginName, version, platform, arch)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "plugin"))
	if err != nil {
		return err
	}
	defer f.Close()

	// Would stream download here

	return nil
}

func (m *Manager) VerifyUpdate(pluginName, version, platform, arch, expectedSHA256 string) error {
	path := filepath.Join(m.store.DataDir(), "artifacts", pluginName, version, platform, arch, "plugin")
	
	hash, err := computeSHA256(path)
	if err != nil {
		return err
	}

	if hash != expectedSHA256 {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSHA256, hash)
	}

	return nil
}

func (m *Manager) GetCurrentVersions() (map[string]string, error) {
	rows, err := m.store.DB().Query(`
		SELECT plugin_name, MAX(plugin_version) FROM policies
		GROUP BY plugin_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make(map[string]string)
	for rows.Next() {
		var name, version string
		if err := rows.Scan(&name, &version); err != nil {
			continue
		}
		versions[name] = version
	}

	return versions, nil
}

func (m *Manager) ScheduleUpdateCheck(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		m.performBulkUpdateCheck()
	}
}

func (m *Manager) performBulkUpdateCheck() {
	// Check all installed plugins for updates
	m.logger.Debug("checking for plugin updates")
}

func (m *Manager) registryURL() string {
	return "https://registry.groundwork.local"
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