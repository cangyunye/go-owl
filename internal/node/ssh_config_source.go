package node

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SSHConfigSource struct {
	configPath string
	hosts      map[string]*SSHHost
}

type SSHHost struct {
	Name         string
	HostName     string
	Port         int
	User         string
	IdentityFile string
	ProxyJump    string
}

func NewSSHConfigSource() *SSHConfigSource {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".ssh", "config")
	
	s := &SSHConfigSource{
		configPath: configPath,
		hosts:      make(map[string]*SSHHost),
	}
	
	s.parseConfig()
	return s
}

func (s *SSHConfigSource) parseConfig() error {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var currentHost *SSHHost

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		keyword := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		switch keyword {
		case "host":
			if currentHost != nil && currentHost.Name != "" {
				s.hosts[currentHost.Name] = currentHost
			}
			currentHost = &SSHHost{
				Name: value,
				Port: 22,
			}
		case "hostname":
			if currentHost != nil {
				currentHost.HostName = value
			}
		case "port":
			if currentHost != nil {
				fmt.Sscanf(value, "%d", &currentHost.Port)
			}
		case "user":
			if currentHost != nil {
				currentHost.User = value
			}
		case "identityfile":
			if currentHost != nil {
				currentHost.IdentityFile = value
			}
		case "proxyjump":
			if currentHost != nil {
				currentHost.ProxyJump = value
			}
		}
	}

	if currentHost != nil && currentHost.Name != "" {
		s.hosts[currentHost.Name] = currentHost
	}

	return nil
}

func (s *SSHConfigSource) GetNode(name string) (*SSHHost, error) {
	host, ok := s.hosts[name]
	if !ok {
		return nil, fmt.Errorf("SSH 配置中未找到主机: %s", name)
	}
	return host, nil
}

func (s *SSHConfigSource) ListHosts() []*SSHHost {
	hosts := make([]*SSHHost, 0, len(s.hosts))
	for _, h := range s.hosts {
		hosts = append(hosts, h)
	}
	return hosts
}

func (s *SSHConfigSource) Reload() error {
	s.hosts = make(map[string]*SSHHost)
	return s.parseConfig()
}
