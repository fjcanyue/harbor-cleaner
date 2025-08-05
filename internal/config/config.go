package config

import (
	"strings"

	"github.com/spf13/viper"
)

// K8sEnvConfig represents the configuration for a single Kubernetes environment.
type K8sEnvConfig struct {
	Name       string   `mapstructure:"name"`
	Kubeconfig string   `mapstructure:"kubeconfig"`
	Namespaces []string `mapstructure:"namespaces"`
	Keep       int      `mapstructure:"keep"`
}

// K8sConfig represents the full Kubernetes configuration.
type K8sConfig struct {
	Environments []K8sEnvConfig `mapstructure:"environments"`
	Stage        string         `mapstructure:"stage"`
	ManifestFile string         `mapstructure:"manifest-file"`
	AuditFile    string         `mapstructure:"audit-file"`
}

// HarborConfig represents the configuration for the Harbor strategy.
type HarborConfig struct {
	URL              string `mapstructure:"url"`
	User             string `mapstructure:"user"`
	Password         string `mapstructure:"password"`
	KeepLastN        int    `mapstructure:"keep-last"`
	MaxSnapshots     int    `mapstructure:"max-snapshots"`
	PageSize         int    `mapstructure:"page-size"`
	ProjectWhitelist string `mapstructure:"project-whitelist"`
}

// Config stores all configuration of the application.
// The values are read by viper from a config file or environment variables.
type Config struct {
	Strategy string       `mapstructure:"strategy"`
	K8s      K8sConfig    `mapstructure:"k8s"`
	Harbor   HarborConfig `mapstructure:"harbor"`
	DryRun   bool         `mapstructure:"dry-run"`
	LogLevel string       `mapstructure:"log.level"`
	LogFile  string       `mapstructure:"log.file"`
}

// LoadConfig reads configuration from file or environment variables.
func LoadConfig(path string) (config Config, err error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err = v.ReadInConfig(); err != nil {
		return
	}

	err = v.Unmarshal(&config)
	return
}
