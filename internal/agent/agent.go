package agent

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/groundwork/groundwork/internal/plugin"
	"github.com/groundwork/groundwork/internal/store"
	"go.uber.org/zap"
)

type Config struct {
	ServerURL       string
	EnrollmentToken string
	CertFingerprint string
	DataDir         string
	Logger          *zap.Logger
}

type Agent struct {
	cfg    Config
	store  *store.Store
	logger *zap.Logger
	wsConn *websocket.Conn
	mu     sync.Mutex
}

type MessageType string

const (
	TypeEnrollment    MessageType = "enrollment"
	TypeReconcileClaim MessageType = "reconcile_claim"
	TypeReconcileDone  MessageType = "reconcile_done"
	TypeHeartbeat    MessageType = "heartbeat"
	TypeJobRequest   MessageType = "job_request"
	TypeJobResult    MessageType = "job_result"
)

type Message struct {
	Type       MessageType `json:"type"`
	MachineID  string      `json:"machine_id,omitempty"`
	Token      string      `json:"token,omitempty"`
	PolicyID   string      `json:"policy_id,omitempty"`
	PluginName string      `json:"plugin_name,omitempty"`
	ConfigYAML string      `json:"config_yaml,omitempty"`
	DesiredHash string     `json:"desired_hash,omitempty"`
	RunID      string      `json:"run_id,omitempty"`
	Status     string      `json:"status,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

// New creates a new Agent instance
func New(cfg Config) *Agent {
	return &Agent{
		cfg:    cfg,
		logger: cfg.Logger,
	}
}

// Run starts the agent enrollment and message processing
func (a *Agent) Run() error {
	// Enrollment phase
	if err := a.enroll(); err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}

	// Connect to server WebSocket
	if err := a.connect(); err != nil {
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	// Run heartbeat
	go a.runHeartbeat()

	// Process messages
	return a.processMessages()
}

// Install installs the agent as a system service
func Install() error {
	return fmt.Errorf("service installation not implemented")
}

// Uninstall removes the agent as a system service
func Uninstall() error {
	return fmt.Errorf("service uninstallation not implemented")
}

func (a *Agent) enroll() error {
	fingerprint := a.cfg.CertFingerprint
	if fingerprint == "" {
		fingerprint = generateMachineFingerprint()
	}

	endpoint := fmt.Sprintf("%s/api/v1/enrollment", a.cfg.ServerURL)
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(jsonBody(map[string]string{
		"token": a.cfg.EnrollmentToken,
		"fingerprint": fingerprint,
	})))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enrollment rejected: %d", resp.StatusCode)
	}
	return nil
}

func (a *Agent) connect() error {
	u := url.URL{Scheme: "ws", Host: a.cfg.ServerURL, Path: "/ws"}
	dialer := websocket.DefaultDialer
	
	// Configure TLS if needed
	if u.Scheme == "wss" {
		dialer.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	a.wsConn = conn
	return nil
}

func (a *Agent) runHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if a.wsConn == nil {
			continue
		}
		a.mu.Lock()
		a.wsConn.WriteJSON(Message{Type: TypeHeartbeat})
		a.mu.Unlock()
	}
}

func (a *Agent) processMessages() error {
	for {
		_, data, err := a.wsConn.ReadMessage()
		if err != nil {
			return err
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case TypeReconcileClaim:
			a.handleReconcileClaim(msg)
		case TypeJobRequest:
			a.handleJobRequest(msg)
		}
	}
}

func (a *Agent) handleReconcileClaim(msg Message) {
	a.logger.Info("handling reconcile claim", zap.String("policy", msg.PolicyID))

	pluginDir := plugin.ArtifactDir(a.cfg.DataDir, msg.PluginName, "latest", detectOS(), detectArch())
	a.logger.Debug("plugin directory", zap.String("dir", pluginDir))
}

func (a *Agent) handleJobRequest(msg Message) {
	a.logger.Info("handling job request", zap.String("job", msg.RunID))
}

func generateMachineFingerprint() string {
	hostname, _ := os.Hostname()
	data := fmt.Sprintf("%s-%d", hostname, os.Getuid())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:16]
}

func detectOS() string {
	switch {
	case filepath.IsAbs("/proc"):
		return "linux"
	case filepath.IsAbs("/etc"):
		return "darwin"
	default:
		return "windows"
	}
}

func detectArch() string {
	return "amd64"
}

func jsonBody(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}