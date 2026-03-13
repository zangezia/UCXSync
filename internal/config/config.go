package config

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	Nodes       []string    `mapstructure:"nodes"`
	Shares      []string    `mapstructure:"shares"`
	Credentials Credentials `mapstructure:"credentials"`
	Network     Network     `mapstructure:"network"`
	Sync        Sync        `mapstructure:"sync"`
	Web         Web         `mapstructure:"web"`
	Monitoring  Monitoring  `mapstructure:"monitoring"`
	Logging     Logging     `mapstructure:"logging"`
}

// Credentials holds authentication information
type Credentials struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// Network holds CIFS mount settings
type Network struct {
	MountRoot string `mapstructure:"mount_root"`
}

// Sync holds synchronization settings
type Sync struct {
	Project               string        `mapstructure:"project"`
	Destination           string        `mapstructure:"destination"`
	MaxParallelism        int           `mapstructure:"max_parallelism"`
	ServiceLoopInterval   time.Duration `mapstructure:"service_loop_interval"`
	MinFreeDiskSpace      int64         `mapstructure:"min_free_disk_space"`
	DiskSpaceSafetyMargin int64         `mapstructure:"disk_space_safety_margin"`
}

// Web holds web server settings
type Web struct {
	Host      string        `mapstructure:"host"`
	Port      int           `mapstructure:"port"`
	Dashboard WebDashboard  `mapstructure:"dashboard"`
}

// WebDashboard holds optional multi-instance dashboard settings.
type WebDashboard struct {
	Instances []DashboardInstance `mapstructure:"instances"`
}

// DashboardInstance describes one UCXSync instance shown in the shared dashboard.
type DashboardInstance struct {
	ID   string `mapstructure:"id"`
	Name string `mapstructure:"name"`
	URL  string `mapstructure:"url"`
}

// Monitoring holds monitoring settings
type Monitoring struct {
	PerformanceUpdateInterval time.Duration `mapstructure:"performance_update_interval"`
	UIUpdateInterval          time.Duration `mapstructure:"ui_update_interval"`
	CPUSmoothingSamples       int           `mapstructure:"cpu_smoothing_samples"`
	MaxDiskThroughputMBps     float64       `mapstructure:"max_disk_throughput_mbps"`
	NetworkSpeedBps           int64         `mapstructure:"network_speed_bps"`
}

// Logging holds logging settings
type Logging struct {
	Level      string `mapstructure:"level"`
	File       string `mapstructure:"file"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
}

// Load reads configuration from file or uses defaults
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.ucxsync")
		v.AddConfigPath("/etc/ucxsync")
	}

	// Read environment variables
	v.SetEnvPrefix("UCXSYNC")
	v.AutomaticEnv()

	// Try to read config file (not required)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, use defaults
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to decode config: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Default nodes
	v.SetDefault("nodes", []string{
		"WU01", "WU02", "WU03", "WU04", "WU05", "WU06", "WU07",
		"WU08", "WU09", "WU10", "WU11", "WU12", "WU13", "CU",
	})

	// Default shares
	v.SetDefault("shares", []string{"E$", "F$"})

	// Default credentials
	v.SetDefault("credentials.username", "Administrator")
	v.SetDefault("credentials.password", "ultracam")

	// Network defaults
	v.SetDefault("network.mount_root", "/ucmount")

	// Sync defaults
	v.SetDefault("sync.max_parallelism", 8)
	v.SetDefault("sync.service_loop_interval", "10s")
	v.SetDefault("sync.min_free_disk_space", 52428800)       // 50 MB
	v.SetDefault("sync.disk_space_safety_margin", 104857600) // 100 MB

	// Web defaults
	v.SetDefault("web.host", "localhost")
	v.SetDefault("web.port", 8080)
	v.SetDefault("web.dashboard.instances", []map[string]any{})

	// Monitoring defaults
	v.SetDefault("monitoring.performance_update_interval", "1s")
	v.SetDefault("monitoring.ui_update_interval", "2s")
	v.SetDefault("monitoring.cpu_smoothing_samples", 3)
	v.SetDefault("monitoring.max_disk_throughput_mbps", 200.0)
	v.SetDefault("monitoring.network_speed_bps", 1000000000) // 1 Gbps

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.file", "logs/ucxsync.log")
	v.SetDefault("logging.max_size", 100)
	v.SetDefault("logging.max_backups", 5)
	v.SetDefault("logging.max_age", 30)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if len(c.Nodes) == 0 {
		return fmt.Errorf("no nodes configured")
	}

	if len(c.Shares) == 0 {
		return fmt.Errorf("no shares configured")
	}

	c.Network.MountRoot = path.Clean(strings.TrimSpace(c.Network.MountRoot))
	if c.Network.MountRoot == "." || c.Network.MountRoot == "" {
		return fmt.Errorf("network.mount_root must not be empty")
	}

	if !strings.HasPrefix(c.Network.MountRoot, "/") {
		return fmt.Errorf("network.mount_root must be an absolute path: %s", c.Network.MountRoot)
	}

	if c.Network.MountRoot == "/" {
		return fmt.Errorf("network.mount_root must not be /")
	}

	if c.Sync.MaxParallelism < 1 {
		return fmt.Errorf("max_parallelism must be at least 1")
	}

	if c.Web.Port < 1 || c.Web.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Web.Port)
	}

	seenDashboardIDs := make(map[string]struct{}, len(c.Web.Dashboard.Instances))
	for i := range c.Web.Dashboard.Instances {
		inst := &c.Web.Dashboard.Instances[i]
		inst.ID = strings.TrimSpace(inst.ID)
		inst.Name = strings.TrimSpace(inst.Name)
		inst.URL = strings.TrimRight(strings.TrimSpace(inst.URL), "/")

		if inst.ID == "" {
			return fmt.Errorf("web.dashboard.instances[%d].id must not be empty", i)
		}

		if _, exists := seenDashboardIDs[inst.ID]; exists {
			return fmt.Errorf("web.dashboard.instances[%d].id duplicates %q", i, inst.ID)
		}
		seenDashboardIDs[inst.ID] = struct{}{}

		if inst.Name == "" {
			inst.Name = inst.ID
		}

		if inst.URL == "" {
			return fmt.Errorf("web.dashboard.instances[%d].url must not be empty", i)
		}

		if !strings.HasPrefix(inst.URL, "http://") && !strings.HasPrefix(inst.URL, "https://") {
			return fmt.Errorf("web.dashboard.instances[%d].url must start with http:// or https://: %s", i, inst.URL)
		}
	}

	return nil
}

// SaveSettings persists user settings to file
func SaveSettings(project, destination string, parallelism int) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsDir := fmt.Sprintf("%s/.ucxsync", homeDir)
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(fmt.Sprintf("%s/settings.yaml", settingsDir))

	v.Set("last_project", project)
	v.Set("last_destination", destination)
	v.Set("parallelism", parallelism)

	return v.WriteConfig()
}

// LoadSettings loads persisted user settings
func LoadSettings() (project, destination string, parallelism int, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", 8, nil // Return defaults
	}

	v := viper.New()
	v.SetConfigFile(fmt.Sprintf("%s/.ucxsync/settings.yaml", homeDir))

	if err := v.ReadInConfig(); err != nil {
		return "", "", 8, nil // Return defaults if file doesn't exist
	}

	return v.GetString("last_project"),
		v.GetString("last_destination"),
		v.GetInt("parallelism"),
		nil
}
