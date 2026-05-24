package file_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/file"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestFileCmdExists(t *testing.T) {
	parent := file.NewFileCmd()

	if parent.Use != "file" {
		t.Errorf("expected Use 'file', got '%s'", parent.Use)
	}

	expected := []string{"upload", "download", "transfer"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestFileUploadFlags(t *testing.T) {
	cmd := file.NewUploadCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "dest")
	testutil.AssertFlagShorthand(t, cmd, "dest", "d")
	testutil.AssertFlagDefault(t, cmd, "dest", "/tmp")

	testutil.AssertFlagExists(t, cmd, "mode")
	testutil.AssertFlagDefault(t, cmd, "mode", "0644")

	testutil.AssertFlagExists(t, cmd, "parallel")
	testutil.AssertFlagDefault(t, cmd, "parallel", "true")

	testutil.AssertFlagExists(t, cmd, "overwrite")
	testutil.AssertFlagDefault(t, cmd, "overwrite", "false")

	testutil.AssertFlagExists(t, cmd, "no-overwrite")
	testutil.AssertFlagDefault(t, cmd, "no-overwrite", "false")

	testutil.AssertFlagExists(t, cmd, "resume")
	testutil.AssertFlagDefault(t, cmd, "resume", "true")
}

func TestFileDownloadFlags(t *testing.T) {
	cmd := file.NewDownloadCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "dest")
	testutil.AssertFlagShorthand(t, cmd, "dest", "d")
	testutil.AssertFlagDefault(t, cmd, "dest", ".")

	testutil.AssertFlagExists(t, cmd, "node")
	testutil.AssertFlagDefault(t, cmd, "node", "")

	testutil.AssertFlagExists(t, cmd, "parallel")
	testutil.AssertFlagDefault(t, cmd, "parallel", "true")

	testutil.AssertFlagExists(t, cmd, "subdir")
	testutil.AssertFlagDefault(t, cmd, "subdir", "false")

	testutil.AssertFlagExists(t, cmd, "name-format")
	testutil.AssertFlagDefault(t, cmd, "name-format", "")

	testutil.AssertFlagExists(t, cmd, "resume")
	testutil.AssertFlagDefault(t, cmd, "resume", "true")
}

func TestFileTransferFlags(t *testing.T) {
	cmd := file.NewTransferCmd()

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")

	testutil.AssertFlagExists(t, cmd, "all-nodes")
	testutil.AssertFlagDefault(t, cmd, "all-nodes", "false")

	testutil.AssertFlagExists(t, cmd, "group")
	testutil.AssertFlagDefault(t, cmd, "group", "")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "dest")
	testutil.AssertFlagShorthand(t, cmd, "dest", "d")
	testutil.AssertFlagDefault(t, cmd, "dest", "/tmp")

	testutil.AssertFlagExists(t, cmd, "source-count")
	testutil.AssertFlagDefault(t, cmd, "source-count", "2")

	testutil.AssertFlagExists(t, cmd, "fan-out")
	testutil.AssertFlagDefault(t, cmd, "fan-out", "3")

	testutil.AssertFlagExists(t, cmd, "threshold")
	testutil.AssertFlagDefault(t, cmd, "threshold", "5")
}

func TestFileHelpContainsSubcommands(t *testing.T) {
	parent := file.NewFileCmd()

	testutil.AssertHelpContains(t, parent, "upload")
	testutil.AssertHelpContains(t, parent, "download")
	testutil.AssertHelpContains(t, parent, "transfer")
}
