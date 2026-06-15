package notify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/groundwork/groundwork/internal/store"
	"go.uber.org/zap"
)

type Manager struct {
	store  *store.Store
	logger *zap.Logger
}

type Notification struct {
	ID        string
	UserID    string
	EventType string
	Recipient string
	Subject   string
	Body      string
	SentAt    time.Time
	Status    string // "pending", "sent", "failed"
}

func NewManager(s *store.Store, logger *zap.Logger) *Manager {
	return &Manager{
		store:  s,
		logger: logger,
	}
}

func (m *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Notification processor stopped")
			return
		case <-ticker.C:
			m.processNotifications()
		}
	}
}

func (m *Manager) processNotifications() {
	m.logger.Debug("processing notifications")
}

func (m *Manager) CreateNotification(userID, eventType, recipient, subject, body string) error {
	id := generateID()
	_, err := m.store.DB().Exec(`
		INSERT INTO notifications (id, user_id, event_type, recipient, subject, body, status)
		VALUES (?, ?, ?, ?, ?, ?, 'pending')
	`, id, userID, eventType, recipient, subject, body)
	return err
}

func (m *Manager) GetNotificationSettings(userID, eventType string) (*NotificationSettings, error) {
	var settings NotificationSettings
	err := m.store.DB().QueryRow(`
		SELECT id, user_id, event_type, email_enabled, created_at
		FROM notification_settings WHERE user_id = ? AND event_type = ?
	`, userID, eventType).Scan(
		&settings.ID, &settings.UserID, &settings.EventType,
		&settings.EmailEnabled, &settings.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func (m *Manager) NotifyReconcileFailed(machineID, policyID, errorMsg string) error {
	m.logger.Info("reconcile failed notification",
		zap.String("machine", machineID),
		zap.String("policy", policyID),
		zap.String("error", errorMsg))
	return nil
}

func (m *Manager) NotifyMachineOffline(machineID string) error {
	m.logger.Info("machine offline notification", zap.String("machine", machineID))
	return nil
}

type NotificationSettings struct {
	ID           string
	UserID       string
	EventType    string
	EmailEnabled bool
	CreatedAt    time.Time
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}