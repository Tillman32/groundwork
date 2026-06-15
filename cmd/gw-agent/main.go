package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/groundwork/groundwork/internal/agent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "gw-agent",
		Short: "Groundwork endpoint agent",
		Long:  `Groundwork agent for endpoint policy reconciliation.`,
		RunE:  runAgent,
	}
)

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./agent.yaml)")
	rootCmd.Flags().String("server", "", "Groundwork server URL (e.g., wss://gw.example.com)")
	rootCmd.Flags().String("token", "", "Enrollment token")
	rootCmd.Flags().String("fingerprint", "", "Server certificate SHA256 fingerprint")
	rootCmd.Flags().String("data-dir", "./data", "Agent data directory")
	rootCmd.Flags().Bool("install-service", false, "Install as system service")
	rootCmd.Flags().Bool("uninstall-service", false, "Uninstall system service")
	viper.BindPFlag("server", rootCmd.Flags().Lookup("server"))
	viper.BindPFlag("token", rootCmd.Flags().Lookup("token"))
	viper.BindPFlag("fingerprint", rootCmd.Flags().Lookup("fingerprint"))
	viper.BindPFlag("data-dir", rootCmd.Flags().Lookup("data-dir"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/groundwork")
		viper.SetConfigName("agent")
		viper.SetConfigType("yaml")
	}
	viper.AutomaticEnv()
	viper.SetEnvPrefix("GW_AGENT")
	_ = viper.ReadInConfig()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runAgent(cmd *cobra.Command, args []string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Handle service install/uninstall
	if viper.GetBool("install-service") {
		return agent.Install()
	}
	if viper.GetBool("uninstall-service") {
		return agent.Uninstall()
	}

	// Create agent instance
	a := agent.New(agent.Config{
		ServerURL:      viper.GetString("server"),
		EnrollmentToken: viper.GetString("token"),
		CertFingerprint: viper.GetString("fingerprint"),
		DataDir:        viper.GetString("data-dir"),
		Logger:         logger,
	})

	// Run agent
	go a.Run()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down agent...")

	return nil
}