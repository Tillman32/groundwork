package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/groundwork/groundwork/internal/store"
	"github.com/groundwork/groundwork/internal/server/trust"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type Manager struct {
	store          *store.Store
	trustMgr       *trust.Manager
	logger         *zap.Logger
	sessionSecret  []byte
}

func NewManager(s *store.Store, trustMgr *trust.Manager, logger *zap.Logger) *Manager {
	secret := make([]byte, 32)
	rand.Read(secret)
	return &Manager{
		store:         s,
		trustMgr:      trustMgr,
		logger:        logger,
		sessionSecret: secret,
	}
}

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	LastLogin    *time.Time
}

type MachineCredential struct {
	MachineID string
	Token     string
	ExpiresAt time.Time
}

func (m *Manager) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func (m *Manager) CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (m *Manager) CreateUser(email, password, role string) (*User, error) {
	if role == "" {
		role = "viewer"
	}
	hash, err := m.HashPassword(password)
	if err != nil {
		return nil, err
	}

	id := generateID()
	now := time.Now()
	user := &User{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
		Role:         role,
		CreatedAt:    now,
	}

	_, err = m.store.DB().Exec(
		`INSERT INTO users (id, email, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, email, hash, role, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

func (m *Manager) AuthenticateUser(email, password string) (*User, error) {
	var user User
	err := m.store.DB().QueryRow(
		`SELECT id, email, password_hash, role, created_at, last_login FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.LastLogin)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	if !m.CheckPassword(user.PasswordHash, password) {
		return nil, fmt.Errorf("invalid password")
	}

	now := time.Now()
	_, _ = m.store.DB().Exec(`UPDATE users SET last_login = ? WHERE id = ?`, now, user.ID)
	user.LastLogin = &now

	return &user, nil
}

func (m *Manager) GetUser(id string) (*User, error) {
	var user User
	err := m.store.DB().QueryRow(
		`SELECT id, email, password_hash, role, created_at, last_login FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.LastLogin)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (m *Manager) CreateMachineCredential(machineID string) (*MachineCredential, error) {
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	tokenHash := sha256.Sum256([]byte(token))
	tokenHashStr := hex.EncodeToString(tokenHash[:])

	expiresAt := time.Now().Add(365 * 24 * time.Hour) // 1 year

	_, err := m.store.DB().Exec(
		`INSERT INTO machines (id, name, enrolled_at, status, config_hash) VALUES (?, ?, ?, 'online', ?)`,
		machineID, machineID, time.Now(), tokenHashStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine record: %w", err)
	}

	return &MachineCredential{
		MachineID: machineID,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (m *Manager) ValidateMachineToken(machineID, token string) bool {
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashStr := hex.EncodeToString(tokenHash[:])

	var storedHash string
	err := m.store.DB().QueryRow(
		`SELECT config_hash FROM machines WHERE id = ? AND status != 'decommissioned'`,
		machineID,
	).Scan(&storedHash)
	if err != nil {
		return false
	}

	return storedHash == tokenHashStr
}

func (m *Manager) ValidateEnrollmentToken(token string) (string, error) {
	tokenHash := sha256.Sum256([]byte(token))
	tokenHashStr := hex.EncodeToString(tokenHash[:])

	var tokenID, fingerprintHash string
	var expiresAt time.Time
	var usedAt *time.Time
	err := m.store.DB().QueryRow(
		`SELECT id, fingerprint_hash, expires_at, used_at FROM enrollment_tokens WHERE token_hash = ?`,
		tokenHashStr,
	).Scan(&tokenID, &fingerprintHash, &expiresAt, &usedAt)
	if err != nil {
		return "", fmt.Errorf("invalid enrollment token")
	}

	if usedAt != nil {
		return "", fmt.Errorf("enrollment token already used")
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("enrollment token expired")
	}

	return tokenID, nil
}

func (m *Manager) MarkEnrollmentTokenUsed(tokenID string) error {
	_, err := m.store.DB().Exec(
		`UPDATE enrollment_tokens SET used_at = ? WHERE id = ?`,
		time.Now(), tokenID,
	)
	return err
}

func (m *Manager) CreateEnrollmentToken(label, fingerprint string, ttl time.Duration) (string, error) {
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := hex.EncodeToString(tokenBytes)

	tokenHash := sha256.Sum256([]byte(token))
	tokenHashStr := hex.EncodeToString(tokenHash[:])

	var fpHash string
	if fingerprint != "" {
		fpHashBytes := sha256.Sum256([]byte(fingerprint))
		fpHash = hex.EncodeToString(fpHashBytes[:])
	}

	id := generateID()
	expiresAt := time.Now().Add(ttl)

	_, err := m.store.DB().Exec(
		`INSERT INTO enrollment_tokens (id, token_hash, label, fingerprint_hash, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, tokenHashStr, label, fpHash, expiresAt,
	)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (m *Manager) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement session/JWT auth
		c.Next()
	}
}

func (m *Manager) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement role-based auth
		c.Next()
	}
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}