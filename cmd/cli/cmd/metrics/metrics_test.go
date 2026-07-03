package metrics_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/metrics"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestMetricsCmdExists(t *testing.T) {
	cmd := metrics.NewMetricsCmd()

	if cmd.Use != "metrics" {
		t.Errorf("expected Use 'metrics', got '%s'", cmd.Use)
	}
	if cmd.Short != "node_exporter 监控" {
		t.Errorf("expected Short 'node_exporter 监控', got '%s'", cmd.Short)
	}
}

func TestMetricsHasWatchSubcommand(t *testing.T) {
	parent := metrics.NewMetricsCmd()

	expected := []string{"watch"}
	testutil.AssertSubCommands(t, parent, expected)

	// Verify watch command through parent lookup
	testutil.AssertCommandExists(t, parent, "watch")
}

func TestMetricsWatchFlags(t *testing.T) {
	parent := metrics.NewMetricsCmd()

	watchCmd, _, err := parent.Find([]string{"watch"})
	if err != nil {
		t.Fatalf("expected watch subcommand to exist: %v", err)
	}

	testutil.AssertFlagExists(t, watchCmd, "config")
	testutil.AssertFlagDefault(t, watchCmd, "config", "")

	testutil.AssertFlagExists(t, watchCmd, "endpoint")
	testutil.AssertFlagDefault(t, watchCmd, "endpoint", "")

	testutil.AssertFlagExists(t, watchCmd, "add-endpoint")
	testutil.AssertFlagDefault(t, watchCmd, "add-endpoint", "")
}

func TestMetricsHelpContainsSections(t *testing.T) {
	cmd := metrics.NewMetricsCmd()

	sections := []string{
		"metrics",
		"node_exporter",
		"watch",
	}
	for _, section := range sections {
		testutil.AssertHelpContains(t, cmd, section)
	}
}
