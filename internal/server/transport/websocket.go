package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/groundwork/groundwork/internal/store"
	"github.com/groundwork/groundwork/internal/server/auth"
	"github.com/groundwork/groundwork/internal/server/policy"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Restrict in production
	},
}

type Hub struct {
	store     *store.Store
	authMgr   *auth.Manager
	policyMgr *policy.Manager
	logger    *zap.Logger

	// Registered connections
	connections map[string]*Connection
	mu          sync.RWMutex
}

type Connection struct {
	Conn     *websocket.Conn
	MachineID string
	Send     chan Message
}

type MessageType string

const (
	TypeEnrollment    MessageType = "enrollment"
	TypeReconcileClaim MessageType = "reconcile_claim"
	TypeReconcileDone  MessageType = "reconcile_done"
	TypeHeartbeat    MessageType = "heartbeat"
	TypeJobRequest   MessageType = "job_request"
	TypeJobResult    MessageType = "job_result"
	TypeError        MessageType = "error"
)

type Message struct {
	Type       MessageType `json:"type"`
	MachineID  string      `json:"machine_id,omitempty"`
	Token      string      `json:"token,omitempty"`
	PolicyID   string      `json:"policy_id,omitempty"`
	ConfigYAML string      `json:"config_yaml,omitempty"`
	DesiredHash string     `json:"desired_hash,omitempty"`
	RunID      string      `json:"run_id,omitempty"`
	Status     string      `json:"status,omitempty"`
	ObservedHash string    `json:"observed_hash,omitempty"`
	Output     []LogLine   `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type LogLine struct {
	Sequence int     `json:"sequence"`
	Line     string  `json:"line"`
	IsStderr bool    `json:"is_stderr"`
	Time     string  `json:"time"`
}

func NewHub(s *store.Store, authMgr *auth.Manager, policyMgr *policy.Manager, logger *zap.Logger) *Hub {
	return &Hub{
		store:     s,
		authMgr:   authMgr,
		policyMgr: policyMgr,
		logger:    logger,
		connections: make(map[string]*Connection),
	}
}

func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("WebSocket hub stopped")
			return
		case <-ticker.C:
			h.broadcastHeartbeat(ctx)
		}
	}
}

func (h *Hub) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	// Read enrollment message
	msgType, data, err := conn.ReadMessage()
	if err != nil || msgType != websocket.TextMessage {
		conn.Close()
		return
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		conn.Close()
		return
	}

	if msg.Type != TypeEnrollment {
		conn.Close()
		return
	}

	// Validate enrollment token
	tokenID, err := h.authMgr.ValidateEnrollmentToken(msg.Token)
	if err != nil {
		h.logger.Warn("invalid enrollment token", zap.String("ip", c.ClientIP()))
		conn.Close()
		return
	}

	// Register connection
	machineID := generateMachineID()
	h.mu.Lock()
	h.connections[machineID] = &Connection{
		Conn:     conn,
		MachineID: machineID,
		Send:     make(chan Message, 256),
	}
	h.mu.Unlock()

	// Mark token as used
	h.authMgr.MarkEnrollmentTokenUsed(tokenID)

	// Send enrollment response
	resp := Message{
		Type:      TypeEnrollment,
		MachineID: machineID,
	}
	respData, _ := json.Marshal(resp)
	conn.WriteMessage(websocket.TextMessage, respData)

	// Broadcast policies to agent
	go h.sendPolicies(machineID)
}

func (h *Hub) sendPolicies(machineID string) {
	policies, err := h.policyMgr.GetMachinePolicies(machineID)
	if err != nil {
		h.logger.Error("failed to get machine policies", zap.Error(err))
		return
	}

	h.mu.RLock()
	conn, ok := h.connections[machineID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	h.mu.RUnlock()

	for _, p := range policies {
		msg := Message{
			Type: TypeReconcileClaim,
			PolicyID: p.PolicyID,
			ConfigYAML: p.ConfigYAML,
			DesiredHash: p.DesiredHash,
		}
		msgData, _ := json.Marshal(msg)
		conn.Send <- msg
		conn.Conn.WriteMessage(websocket.TextMessage, msgData)
	}
}

func (h *Hub) broadcastHeartbeat(ctx context.Context) {
	h.mu.RLock()
	conns := make([]*Connection, 0, len(h.connections))
	for _, c := range h.connections {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		h.heartbeat(c)
	}
}

func (h *Hub) heartbeat(c *Connection) {
	// Update last seen
	now := time.Now().Format(time.RFC3339)
	_, _ = h.store.DB().Exec("UPDATE machines SET last_seen = ? WHERE id = ?", now, c.MachineID)
}

func (h *Hub) SendReconcileClaim(machineID, policyID, configYAML, desiredHash string) error {
	h.mu.RLock()
	conn, ok := h.connections[machineID]
	if !ok {
		h.mu.RUnlock()
		return nil // Machine not connected
	}
	h.mu.RUnlock()

	msg := Message{
		Type: TypeReconcileClaim,
		PolicyID: policyID,
		ConfigYAML: configYAML,
		DesiredHash: desiredHash,
	}
	msgData, _ := json.Marshal(msg)
	return conn.Conn.WriteMessage(websocket.TextMessage, msgData)
}

func (h *Hub) HandleReconcileDone(machineID, runID, status, observedHash string, output []LogLine) error {
	// Record completion
	_, err := h.store.DB().Exec(`
		UPDATE reconciliation_runs SET status = ?, observed_hash = ?, finished_at = ?
		WHERE id = ?
	`, status, observedHash, time.Now(), runID)
	return err
}

func generateMachineID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}