package main

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/zangezia/UCXSync/internal/config"
	"github.com/zangezia/UCXSync/internal/network"
)

func runMount(cmd *cobra.Command, args []string) {
	setupLogging()

	log.Info().Msg("Mounting network shares...")

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Check requirements
	if err := network.CheckRequirements(); err != nil {
		log.Fatal().Err(err).Msg("Requirements not met")
	}

	// Create network service
	netService := network.New(
		cfg.Nodes,
		cfg.Shares,
		cfg.Credentials.Username,
		cfg.Credentials.Password,
	)

	// Mount all shares
	if err := netService.MountAll(); err != nil {
		log.Error().Err(err).Msg("Failed to mount some shares")
		return
	}

	log.Info().Msg("✓ All shares mounted successfully")
	log.Info().Msg("Mount point: /mnt/ucx")
}

func runUnmount(cmd *cobra.Command, args []string) {
	setupLogging()

	log.Info().Msg("Unmounting network shares...")

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Create network service
	netService := network.New(
		cfg.Nodes,
		cfg.Shares,
		cfg.Credentials.Username,
		cfg.Credentials.Password,
	)

	// Unmount all shares
	if err := netService.UnmountAll(); err != nil {
		log.Error().Err(err).Msg("Failed to unmount some shares")
		return
	}

	log.Info().Msg("✓ All shares unmounted successfully")
}

func runCheck(cmd *cobra.Command, args []string) {
	setupLogging()

	log.Info().Msg("Checking system requirements...")

	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	log.Info().Msg("✓ Configuration loaded")
	log.Info().Int("nodes", len(cfg.Nodes)).Msg("Configured nodes")
	log.Info().Int("shares", len(cfg.Shares)).Msg("Configured shares")

	// Check network requirements
	if err := network.CheckRequirements(); err != nil {
		log.Error().Err(err).Msg("✗ Network requirements not met")
		log.Info().Msg("Install: sudo apt-get install cifs-utils")
		log.Info().Msg("Run as: sudo ucxsync")
		return
	}

	log.Info().Msg("✓ Network requirements met")
	log.Info().Msg("✓ CIFS utilities installed")
	log.Info().Msg("✓ Running with required privileges")
	log.Info().Msg("")
	log.Info().Msg("System ready! You can now:")
	log.Info().Msg("  1. Mount shares: sudo ucxsync mount")
	log.Info().Msg("  2. Start server: sudo ucxsync")
}
