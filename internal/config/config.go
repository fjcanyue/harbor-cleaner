package config

import (
	"strings"

	"github.com/spf13/viper"
)

// K8sEnvConfig represents the configuration for a single Kubernetes environment.
type K8sEnvConfig struct {
	Name        string   `mapstructure:"name"`
	Kubeconfig  string   `mapstructure:"kubeconfig"`
	Namespaces  []string `mapstructure:"namespaces"`
	Keep        int      `mapstructure:"keep"`
	PodWhitelist []string `mapstructure:"pod-whitelist"`
	PodBlacklist []string `mapstructure:"pod-blacklist"`
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

// MatchWildcard checks if a string matches a pattern with wildcards (* and ?)
func MatchWildcard(pattern, str string) bool {
	return matchWildcardHelper(pattern, str, 0, 0)
}

func matchWildcardHelper(pattern, str string, pIdx, sIdx int) bool {
	pLen, sLen := len(pattern), len(str)
	
	for pIdx < pLen {
		if sIdx >= sLen {
			// String exhausted, check if remaining pattern is all *
			for pIdx < pLen && pattern[pIdx] == '*' {
				pIdx++
			}
			return pIdx == pLen
		}
		
		if pattern[pIdx] == '*' {
			// Try matching 0 or more characters
			for pIdx < pLen && pattern[pIdx] == '*' {
				pIdx++
			}
			if pIdx == pLen {
				return true // Trailing * matches everything
			}
			// Try matching the rest of pattern against remaining string
			for i := sIdx; i <= sLen; i++ {
				if matchWildcardHelper(pattern, str, pIdx, i) {
					return true
				}
			}
			return false
		}
		
		if pattern[pIdx] == '?' || pattern[pIdx] == str[sIdx] {
			pIdx++
			sIdx++
		} else {
			return false
		}
	}
	
	return sIdx == sLen
}

// ShouldProcessWorkload checks if a workload name should be processed based on whitelist and blacklist
// Returns true if the workload should be processed
func ShouldProcessWorkload(workloadName string, whitelist, blacklist []string) bool {
	// If blacklist is provided and workload matches, skip it
	if len(blacklist) > 0 {
		for _, pattern := range blacklist {
			if MatchWildcard(pattern, workloadName) {
				return false
			}
		}
	}
	
	// If whitelist is provided, only process if workload matches
	if len(whitelist) > 0 {
		for _, pattern := range whitelist {
			if MatchWildcard(pattern, workloadName) {
				return true
			}
		}
		return false
	}
	
	// No filters, process all
	return true
}
