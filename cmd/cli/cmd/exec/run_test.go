package exec

import (
	"testing"
)

func TestNewRunCmd_HasSyncNodesFlag(t *testing.T) {
	cmd := NewRunCmd()
	flag := cmd.Flags().Lookup("sync-nodes")
	if flag == nil {
		t.Error("expected --sync-nodes flag to be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default value 'false', got %q", flag.DefValue)
	}
}
