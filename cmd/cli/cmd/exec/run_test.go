package exec

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestPrintConflictReportFromCommon(t *testing.T) {
	conflicts := []common.NodeConflict{
		{
			Type:        common.ConflictCrossSourceName,
			Description: "Same name 'web' but different IDs: DB=db-1, JSON=json-1",
			DBNode:      &common.NodeInfo{ID: "db-1", Name: "web", Address: "10.0.0.1"},
			JSONNode:    &common.NodeInfo{ID: "json-1", Name: "web", Address: "10.0.0.2"},
		},
	}

	common.PrintConflictReport(conflicts, 3, 2)
}

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
