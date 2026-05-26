package daemon

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// SubsystemConfig controls which daemon subsystems are active.
type SubsystemConfig struct {
	Watcher   bool `yaml:"watcher"`
	Scheduler bool `yaml:"scheduler"`
	Reporter  bool `yaml:"reporter"`
	Miner     bool `yaml:"miner"`
}

// DaemonConfig holds all configuration for the agent daemon.
type DaemonConfig struct {
	Enabled     bool            `yaml:"enabled"`
	Interval    time.Duration   `yaml:"interval"`
	LogDir      string          `yaml:"log_dir"`
	ReportBridge string          `yaml:"report_bridge"`
	Subsystems  SubsystemConfig `yaml:"subsystems"`
	PIDFile     string          `yaml:"pid_file"`
	MaxAgents   int             `yaml:"max_agents"`
}

// DefaultConfig returns sensible defaults for the daemon.
func DefaultConfig() DaemonConfig {
	return DaemonConfig{
		Enabled:      true,
		Interval:     10 * time.Second,
		LogDir:       os.ExpandEnv("$HOME/.config/agentfs/logs"),
		ReportBridge: "",
		Subsystems: SubsystemConfig{
			Watcher:   true,
			Scheduler: true,
			Reporter:  true,
			Miner:     true,
		},
		PIDFile:   os.ExpandEnv("$HOME/.config/agentfs/daemon.pid"),
		MaxAgents: 50,
	}
}

// LoadConfig reads and parses a YAML daemon configuration file.
func LoadConfig(path string) (DaemonConfig, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
