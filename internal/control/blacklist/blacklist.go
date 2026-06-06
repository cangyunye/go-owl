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
				// 文件操作（危险）
				"rm -rf ",
				"rm -fr ",
				"rm -r ",
				"rm -f ",
				"mkfs.",
				"dd if=",
				"dd of=",
				"fdisk ",
				"parted ",
				"mkswap",
				"mount --bind ",
				"umount -f ",

				// 用户权限提升（需要确认）
				" su ",
				" sudo ",

				// 远程操作（需要确认）
				"ssh -",
				"scp -",
				"rsync -",

				// 系统控制（危险）
				"shutdown",
				"reboot",
				"halt",
				"poweroff",
				"init 0",
				"init 6",

				// 权限修改（危险）
				"chmod -R 777 ",
				"chmod 777 /",
				"chown -R ",
				"chown .*:[0-9]+ ",
				"chattr -",

				// 网络/防火墙（需要确认）
				"iptables -",
				"ufw -",
				"firewall-cmd -",

				// 服务控制（需要确认）
				"systemctl stop ",
				"systemctl disable ",
				"systemctl mask ",
				"service .* stop",

				// 进程控制（危险）
				"killall ",
				"pkill -",
				"kill -9 ",
				"kill -SIGKILL",
			},
		},
		{
			User: "*",
			Patterns: []string{
				// 全局危险命令
				"rm -rf /",
				"rm -rf /*",
				"dd if=/dev/",
				"mkfs.",
				":(){ :|:& };:",
				">/dev/sd",
				"chmod 777 /",
				">/etc/",
				"dd if=/dev/zero of=/dev/",
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
