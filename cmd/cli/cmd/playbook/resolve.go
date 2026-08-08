package playbook

import (
	"os"
	"path/filepath"
)

func defaultPlaybookDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./playbooks"
	}
	p := filepath.Join(home, ".owl", "playbooks")
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return p
	}
	if fi, err := os.Stat("./playbooks"); err == nil && fi.IsDir() {
		abs, _ := filepath.Abs("./playbooks")
		return abs
	}
	return p
}
