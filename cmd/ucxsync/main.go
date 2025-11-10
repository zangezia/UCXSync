package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/zangezia/UCXSync/internal/config"
	"github.com/zangezia/UCXSync/internal/web"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	cfgFile   string
	debug     bool
)

var rootCmd = &cobra.Command{
	Use:   "ucxsync",
	Short: "UCXSync - High-performance file synchronization tool",
	Long: `UCXSync synchronizes files from multiple UCX worker nodes to a local destination
with real-time monitoring through a web interface.`,
	Version: fmt.Sprintf("%s (built: %s)", Version, BuildTime),
	Run:     runApp,
}

var mountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mount network shares",
	Long:  "Mount all configured network shares to /mnt/ucx",
	Run:   runMount,
}

var unmountCmd = &cobra.Command{
	Use:   "unmount",
	Short: "Unmount network shares",
	Long:  "Unmount all network shares from /mnt/ucx",
	Run:   runUnmount,
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check system requirements",
	Long:  "Check if all system requirements are met",
	Run:   runCheck,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.Flags().String("project", "", "project name to sync")
	rootCmd.Flags().String("dest", "", "destination directory")
	rootCmd.Flags().Int("port", 8080, "web server port")
	rootCmd.Flags().Int("parallelism", 8, "max parallel file operations")

	rootCmd.AddCommand(mountCmd)
	rootCmd.AddCommand(unmountCmd)
	rootCmd.AddCommand(checkCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runApp(cmd *cobra.Command, args []string) {
	// Setup logging
	setupLogging()

	log.Info().
		Str("version", Version).
		Str("build_time", BuildTime).
		Msg("Starting UCXSync")

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Override config with command-line flags
	if project, _ := cmd.Flags().GetString("project"); project != "" {
		cfg.Sync.Project = project
	}
	if dest, _ := cmd.Flags().GetString("dest"); dest != "" {
		cfg.Sync.Destination = dest
	}
	if port, _ := cmd.Flags().GetInt("port"); port != 0 {
		cfg.Web.Port = port
	}
	if parallelism, _ := cmd.Flags().GetInt("parallelism"); parallelism != 0 {
		cfg.Sync.MaxParallelism = parallelism
	}

	// Display startup banner
	log.Info().Msg("========================================")
	log.Info().Msg("       UCXSync - File Synchronization   ")
	log.Info().Msg("========================================")
	log.Info().Int("nodes", len(cfg.Nodes)).Msg("Configured nodes")
	log.Info().Int("shares", len(cfg.Shares)).Msg("Configured shares")
	log.Info().Int("parallelism", cfg.Sync.MaxParallelism).Msg("Max parallelism")

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start web server
	server := web.NewServer(cfg)

	log.Info().
		Str("address", fmt.Sprintf("http://%s:%d", cfg.Web.Host, cfg.Web.Port)).
		Msg("Starting web interface...")

	go func() {
		if err := server.Start(ctx); err != nil {
			log.Error().Err(err).Msg("Web server error")
		}
	}()

	log.Info().Msg("Server is ready! Open your browser to access the web interface")
	log.Info().Msg("========================================")

	// Wait for shutdown signal
	<-sigChan
	log.Info().Msg("")
	log.Info().Msg("========================================")
	log.Info().Msg("Shutting down gracefully...")
	cancel()
}

func setupLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}
}
