package blacklist

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Rule struct {
	User     string   `yaml:"user"`
	Patterns []string `yaml:"patterns"`
}

type Config struct {
	Rules []Rule `yaml:"rules"`
}

func DefaultRules() []Rule {
	return []Rule{
		{
			User: "root",
			Patterns: []string{
				"rm ",
				"mv ",
				"su",
				"sudo ",
				"ssh ",
				"scp ",
				"dd ",
				"mkfs",
				"fdisk ",
				"shutdown",
				"reboot",
				"halt",
				"poweroff",
				"init ",
				"chmod ",
				"chown ",
				"chattr ",
				"iptables ",
				"ufw ",
				"firewall-cmd ",
				"systemctl stop ",
				"systemctl disable ",
				"systemctl mask ",
				"killall ",
				"pkill ",
				"parted ",
				"mkswap",
				"mount ",
				"umount ",
			},
		},
		{
			User: "*",
			Patterns: []string{
				"rm -rf /",
				"rm -rf /*",
				"dd if=/dev/",
				"mkfs.",
				":(){ :|:& };:",
				">/dev/sd",
				"chmod 777 /",
			},
		},
	}
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".owl", "blacklist.yaml")
}

func LoadConfig() (*Config, error) {
	path := configPath()
	if path == "" {
		return &Config{Rules: DefaultRules()}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Rules: DefaultRules()}, nil
		}
		return &Config{Rules: DefaultRules()}, fmt.Errorf("读取黑名单配置文件失败，使用默认规则: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  黑名单配置文件格式错误，使用默认规则: %v\n", err)
		return &Config{Rules: DefaultRules()}, nil
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path := configPath()
	if path == "" {
		return fmt.Errorf("无法获取用户主目录")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}
