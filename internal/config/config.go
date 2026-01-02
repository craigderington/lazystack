package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Systemd    SystemdConfig    `mapstructure:"systemd"`
	Kubernetes KubernetesConfig `mapstructure:"kubernetes"`
	UI         UIConfig         `mapstructure:"ui"`
}

// SystemdConfig contains systemd-specific configuration
type SystemdConfig struct {
	UnitsToWatch        []string `mapstructure:"units_to_watch"`
	AutoRefreshInterval string   `mapstructure:"auto_refresh_interval"`
}

// KubernetesConfig contains Kubernetes-specific configuration
type KubernetesConfig struct {
	Kubeconfig          string `mapstructure:"kubeconfig"`
	DefaultContext      string `mapstructure:"default_context"`
	DefaultNamespace    string `mapstructure:"default_namespace"`
	AutoRefreshInterval string `mapstructure:"auto_refresh_interval"`
}

// UIConfig contains UI-specific configuration
type UIConfig struct {
	Theme      string  `mapstructure:"theme"`
	VimMode    bool    `mapstructure:"vim_mode"`
	SplitRatio float64 `mapstructure:"split_ratio"`
}

// Load loads the configuration from file
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("systemd.auto_refresh_interval", "5s")
	v.SetDefault("kubernetes.default_namespace", "default")
	v.SetDefault("kubernetes.auto_refresh_interval", "3s")
	v.SetDefault("ui.theme", "default")
	v.SetDefault("ui.vim_mode", true)
	v.SetDefault("ui.split_ratio", 0.5)

	// Set config file path if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Look for config in standard locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("$HOME/.config/lazystack")
		v.AddConfigPath("/etc/lazystack")
		v.AddConfigPath(".")
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		// Config file not found is not an error - we'll use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set default kubeconfig path if not specified
	if config.Kubernetes.Kubeconfig == "" {
		home := viper.GetString("HOME")
		config.Kubernetes.Kubeconfig = filepath.Join(home, ".kube", "config")
	}

	return &config, nil
}

// GetDefaultConfig returns a configuration with default values
func GetDefaultConfig() *Config {
	return &Config{
		Systemd: SystemdConfig{
			UnitsToWatch:        []string{},
			AutoRefreshInterval: "5s",
		},
		Kubernetes: KubernetesConfig{
			DefaultNamespace:    "default",
			AutoRefreshInterval: "3s",
		},
		UI: UIConfig{
			Theme:      "default",
			VimMode:    true,
			SplitRatio: 0.5,
		},
	}
}
