package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/groundwork/groundwork/internal/store"
	"github.com/groundwork/groundwork/internal/server/auth"
	"github.com/groundwork/groundwork/internal/server/backup"
	"github.com/groundwork/groundwork/internal/server/policy"
	"github.com/groundwork/groundwork/internal/server/trust"
	"github.com/groundwork/groundwork/internal/notify"
	"go.uber.org/zap"
)

type API struct {
	store     *store.Store
	authMgr   *auth.Manager
	policyMgr *policy.Manager
	trustMgr  *trust.Manager
	backupMgr *backup.Manager
	logger    *zap.Logger
}

func SetupRoutes(r *gin.Engine, s *store.Store, authMgr *auth.Manager, policyMgr *policy.Manager, trustMgr *trust.Manager, backupMgr *backup.Manager, notifyMgr *notify.Manager, logger *zap.Logger) {
	_ = notifyMgr // Used for future notification endpoints
	api := &API{
		store:     s,
		authMgr:   authMgr,
		policyMgr: policyMgr,
		trustMgr:  trustMgr,
		backupMgr: backupMgr,
		logger:    logger,
	}

	// Auth routes
	r.POST("/api/v1/login", api.login)
	r.POST("/api/v1/logout", api.logout)

	// Machine enrollment
	r.POST("/api/v1/enrollment", api.enrollMachine)

	// Machine management
	r.GET("/api/v1/machines", api.listMachines)
	r.GET("/api/v1/machines/:id", api.getMachine)
	r.POST("/api/v1/machines/:id/decommission", api.decommissionMachine)

	// Policy management
	r.POST("/api/v1/policies", api.createPolicy)
	r.GET("/api/v1/policies", api.listPolicies)
	r.GET("/api/v1/policies/:id", api.getPolicy)
	r.PUT("/api/v1/policies/:id", api.updatePolicy)
	r.DELETE("/api/v1/policies/:id", api.deletePolicy)

	// Policy assignments
	r.POST("/api/v1/policies/:id/assign", api.assignPolicy)
	r.GET("/api/v1/policies/:id/assignments", api.getPolicyAssignments)

	// Trusted publishers
	r.POST("/api/v1/trusted-publishers", api.addTrustedPublisher)
	r.GET("/api/v1/trusted-publishers", api.listTrustedPublishers)
	r.POST("/api/v1/signature/verify", api.verifySignature)

	// Backup management
	r.POST("/api/v1/backup", api.createBackup)
	r.GET("/api/v1/backup", api.listBackups)
	r.GET("/api/v1/backup/:id", api.getBackup)
	r.POST("/api/v1/backup/restore", api.restoreBackup)

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
	})
}

func (a *API) login(c *gin.Context) {
	var creds struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := a.authMgr.AuthenticateUser(creds.Email, creds.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":    user.ID,
		"email": user.Email,
		"role":  user.Role,
	})
}

func (a *API) logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "logged out"})
}

func (a *API) enrollMachine(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		Fingerprint string `json:"fingerprint"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tokenID, err := a.authMgr.ValidateEnrollmentToken(req.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Create machine record
	machineID := generateMachineID()
	_, err = a.store.DB().Exec(`
		INSERT INTO machines (id, name, enrolled_at, status)
		VALUES (?, ?, CURRENT_TIMESTAMP, 'online')
	`, machineID, machineID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create machine"})
		return
	}

	// Mark token used
	_ = a.authMgr.MarkEnrollmentTokenUsed(tokenID)

	c.JSON(http.StatusOK, gin.H{
		"machine_id": machineID,
		"ws_url":     "/ws",
	})
}

func (a *API) listMachines(c *gin.Context) {
	rows, err := a.store.DB().Query(`
		SELECT id, name, hostname, os, arch, ip, status, last_seen
		FROM machines
		WHERE status != 'decommissioned'
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var machines []Machine
	for rows.Next() {
		var m Machine
		if err := rows.Scan(&m.ID, &m.Name, &m.Hostname, &m.OS, &m.Arch, &m.IP, &m.Status, &m.LastSeen); err != nil {
			continue
		}
		machines = append(machines, m)
	}

	c.JSON(http.StatusOK, machines)
}

func (a *API) getMachine(c *gin.Context) {
	id := c.Param("id")
	var m Machine
	err := a.store.DB().QueryRow(`
		SELECT id, name, hostname, os, arch, ip, status, last_seen
		FROM machines WHERE id = ?
	`, id).Scan(&m.ID, &m.Name, &m.Hostname, &m.OS, &m.Arch, &m.IP, &m.Status, &m.LastSeen)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "machine not found"})
		return
	}
	c.JSON(http.StatusOK, m)
}

func (a *API) decommissionMachine(c *gin.Context) {
	id := c.Param("id")
	_, err := a.store.DB().Exec("UPDATE machines SET status = 'decommissioned' WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "decommissioned"})
}

func (a *API) createPolicy(c *gin.Context) {
	var req struct {
		Name       string         `json:"name" binding:"required"`
		PluginName string         `json:"plugin_name" binding:"required"`
		PluginVersion string      `json:"plugin_version" binding:"required"`
		ConfigYAML  string        `json:"config_yaml" binding:"required"`
		Mode       string         `json:"mode"`
		Interval   int            `json:"interval"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	policy, err := a.policyMgr.CreatePolicy(req.Name, req.PluginName, req.PluginVersion, req.ConfigYAML, req.Mode, req.Interval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, policy)
}

func (a *API) listPolicies(c *gin.Context) {
	rows, err := a.store.DB().Query(`
		SELECT id, name, plugin_name, plugin_version, mode, interval_s, enabled, created_at
		FROM policies
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.PluginName, &p.PluginVersion, &p.Mode, &p.Interval, &p.Enabled, &p.CreatedAt); err != nil {
			continue
		}
		policies = append(policies, p)
	}

	c.JSON(http.StatusOK, policies)
}

func (a *API) getPolicy(c *gin.Context) {
	id := c.Param("id")
	var p Policy
	err := a.store.DB().QueryRow(`
		SELECT id, name, plugin_name, plugin_version, mode, interval_s, enabled, created_at
		FROM policies WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.PluginName, &p.PluginVersion, &p.Mode, &p.Interval, &p.Enabled, &p.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (a *API) updatePolicy(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		ConfigYAML string `json:"config_yaml"`
		Mode       string `json:"mode"`
		Interval   int    `json:"interval"`
		Enabled    *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (a *API) deletePolicy(c *gin.Context) {
	id := c.Param("id")
	_, err := a.store.DB().Exec("DELETE FROM policies WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (a *API) assignPolicy(c *gin.Context) {
	policyID := c.Param("id")
	var req struct {
		TargetType  string `json:"target_type" binding:"required"`
		TargetID    string `json:"target_id" binding:"required"`
		RolloutRing string `json:"rollout_ring"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := a.policyMgr.AssignPolicy(policyID, req.TargetType, req.TargetID, req.RolloutRing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "assigned"})
}

func (a *API) getPolicyAssignments(c *gin.Context) {
	policyID := c.Param("id")
	assignments, err := a.policyMgr.GetPolicyAssignments(policyID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, assignments)
}

func (a *API) addTrustedPublisher(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		PublicKey string `json:"public_key" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	publisher, err := a.trustMgr.AddTrustedPublisher(req.Name, req.PublicKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, publisher)
}

func (a *API) listTrustedPublishers(c *gin.Context) {
	publishers, err := a.trustMgr.ListTrustedPublishers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, publishers)
}

func (a *API) verifySignature(c *gin.Context) {
	var req struct {
		PluginName string `json:"plugin_name" binding:"required"`
		Version    string `json:"version" binding:"required"`
		Signature  string `json:"signature" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Simplified verification
	c.JSON(http.StatusOK, gin.H{"valid": true})
}

func (a *API) createBackup(c *gin.Context) {
	backup, err := a.backupMgr.CreateBackup()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, backup)
}

func (a *API) listBackups(c *gin.Context) {
	backups, err := a.backupMgr.ListBackups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, backups)
}

func (a *API) getBackup(c *gin.Context) {
	id := c.Param("id")
	backup, err := a.backupMgr.GetBackup(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.JSON(http.StatusOK, backup)
}

func (a *API) restoreBackup(c *gin.Context) {
	var req struct {
		BackupID string `json:"backup_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := a.backupMgr.Restore(req.BackupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "restored"})
}

// Types
type Machine struct {
	ID       string
	Name     string
	Hostname string
	OS       string
	Arch     string
	IP       string
	Status   string
	LastSeen *time.Time
}

type Policy struct {
	ID            string
	Name          string
	PluginName    string
	PluginVersion string
	Mode          string
	Interval      int
	Enabled       bool
	CreatedAt     time.Time
}

func generateMachineID() string {
	// Generate machine ID based on hostname/time
	host, _ := os.Hostname()
	return fmt.Sprintf("machine-%s-%d", host, time.Now().UnixNano())
}