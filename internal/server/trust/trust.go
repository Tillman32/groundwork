package trust

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/groundwork/groundwork/internal/store"
	"go.uber.org/zap"
)

type Manager struct {
	store  *store.Store
	logger *zap.Logger
}

func NewManager(s *store.Store, logger *zap.Logger) *Manager {
	return &Manager{
		store:  s,
		logger: logger,
	}
}

type TrustedPublisher struct {
	ID        string
	Name      string
	PublicKey string
	Status    string
	CreatedAt string
}

func (m *Manager) AddTrustedPublisher(name, publicKeyPEM string) (*TrustedPublisher, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid PEM format")
	}

	id := generateID()
	now := time.Now().Format(time.RFC3339)

	_, err := m.store.DB().Exec(
		`INSERT INTO trusted_publishers (id, name, public_key, status, created_at) VALUES (?, ?, ?, 'active', ?)`,
		id, name, publicKeyPEM, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add trusted publisher: %w", err)
	}

	return &TrustedPublisher{
		ID:        id,
		Name:      name,
		PublicKey: publicKeyPEM,
		Status:    "active",
		CreatedAt: now,
	}, nil
}

func (m *Manager) GetTrustedPublisher(id string) (*TrustedPublisher, error) {
	var p TrustedPublisher
	err := m.store.DB().QueryRow(
		`SELECT id, name, public_key, status, created_at FROM trusted_publishers WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Name, &p.PublicKey, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (m *Manager) ListTrustedPublishers() ([]*TrustedPublisher, error) {
	rows, err := m.store.DB().Query(
		`SELECT id, name, public_key, status, created_at FROM trusted_publishers WHERE status = 'active'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var publishers []*TrustedPublisher
	for rows.Next() {
		var p TrustedPublisher
		if err := rows.Scan(&p.ID, &p.Name, &p.PublicKey, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		publishers = append(publishers, &p)
	}
	return publishers, nil
}

func (m *Manager) RevokePublisher(id string) error {
	_, err := m.store.DB().Exec(
		`UPDATE trusted_publishers SET status = 'revoked' WHERE id = ?`,
		id,
	)
	return err
}

func (m *Manager) GetPublisherPublicKey(publisherID string) (string, error) {
	var publicKey string
	err := m.store.DB().QueryRow(
		`SELECT public_key FROM trusted_publishers WHERE id = ? AND status = 'active'`,
		publisherID,
	).Scan(&publicKey)
	if err != nil {
		return "", err
	}
	return publicKey, nil
}

func (m *Manager) VerifyServerFingerprint(fingerprint string) bool {
	var stored string
	err := m.store.DB().QueryRow(
		`SELECT value FROM settings WHERE key = 'server_cert_fingerprint'`,
	).Scan(&stored)
	if err != nil {
		return false
	}
	return stored == fingerprint
}

func (m *Manager) SetServerFingerprint(fingerprint string) error {
	_, err := m.store.DB().Exec(
		`INSERT OR REPLACE INTO settings (key, value) VALUES ('server_cert_fingerprint', ?)`,
		fingerprint,
	)
	return err
}

func ComputeCertificateFingerprint(certPEM string) (string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", fmt.Errorf("invalid certificate PEM")
	}
	hash := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(hash[:]), nil
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}