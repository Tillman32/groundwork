package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/groundwork/groundwork/internal/server/api"
	"github.com/groundwork/groundwork/internal/server/auth"
	"github.com/groundwork/groundwork/internal/server/backup"
	"github.com/groundwork/groundwork/internal/server/policy"
	"github.com/groundwork/groundwork/internal/server/transport"
	"github.com/groundwork/groundwork/internal/server/trust"
	"github.com/groundwork/groundwork/internal/server/update"
	"github.com/groundwork/groundwork/internal/server/web"
	"github.com/groundwork/groundwork/internal/store"
	"github.com/groundwork/groundwork/internal/notify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "groundworkd",
		Short: "Groundwork control plane server",
		Long:  `Single-tenant endpoint policy management control plane.`,
		RunE:  runServer,
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./groundwork.yaml)")
	rootCmd.Flags().String("addr", ":8080", "HTTP listen address")
	rootCmd.Flags().String("db", "groundwork.db", "SQLite database path")
	rootCmd.Flags().String("data-dir", "./data", "Data directory for artifacts and cache")
	viper.BindPFlag("addr", rootCmd.Flags().Lookup("addr"))
	viper.BindPFlag("db", rootCmd.Flags().Lookup("db"))
	viper.BindPFlag("data-dir", rootCmd.Flags().Lookup("data-dir"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("groundwork")
		viper.SetConfigType("yaml")
	}
	viper.AutomaticEnv()
	viper.SetEnvPrefix("GROUNDWORK")
	_ = viper.ReadInConfig()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database
	dbPath := viper.GetString("db")
	dataDir := viper.GetString("data-dir")
	s, err := store.New(dbPath, dataDir, logger)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer s.Close()

	// Run migrations
	if err := s.Migrate(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Initialize core services
	trustMgr := trust.NewManager(s, logger)
	authMgr := auth.NewManager(s, trustMgr, logger)
	policyMgr := policy.NewManager(s, logger)
	backupMgr := backup.NewManager(s, dataDir, logger)
	_ = update.NewManager(s, logger) // Initialize update manager
	notifyMgr := notify.NewManager(s, logger)

	// Initialize WebSocket hub
	wsHub := transport.NewHub(s, authMgr, policyMgr, logger)

	// Initialize Gin router
	r := gin.Default()
	r.Use(gin.Recovery())

	// Setup API routes
	api.SetupRoutes(r, s, authMgr, policyMgr, trustMgr, backupMgr, notifyMgr, logger)

	// Setup WebSocket endpoint
	r.GET("/ws", wsHub.HandleWebSocket)

	// Serve embedded web UI
	web.ServeUI(r)

	addr := viper.GetString("addr")
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Start server
	go func() {
		logger.Info("Starting Groundwork server", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// Start WebSocket hub
	go wsHub.Run(ctx)

	// Start policy scheduler
	go policyMgr.StartScheduler(ctx)

	// Start notification processor
	go notifyMgr.Start(ctx)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Forced shutdown", zap.Error(err))
	}

	return nil
}