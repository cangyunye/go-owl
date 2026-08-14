package nodes

import (
	"net"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestRunPing_AllReachable(t *testing.T) {
	dial := func(addr string, timeout time.Duration) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2
		return c1, nil
	}
	nodes := []*common.NodeInfo{
		{ID: "n1", Address: "10.0.0.1", Port: 22},
		{ID: "n2", Address: "10.0.0.2", Port: 22},
	}
	results := runPing(nodes, time.Second, dial)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Fatalf("expected success for %s", r.Node.ID)
		}
	}
}

func TestRunPing_MixedReachability(t *testing.T) {
	dial := func(addr string, timeout time.Duration) (net.Conn, error) {
		if strings.Contains(addr, "10.0.0.2") {
			return nil, &net.OpError{Err: net.ErrClosed}
		}
		c1, _ := net.Pipe()
		return c1, nil
	}
	nodes := []*common.NodeInfo{
		{ID: "n1", Address: "10.0.0.1", Port: 22},
		{ID: "n2", Address: "10.0.0.2", Port: 22},
	}
	results := runPing(nodes, time.Second, dial)
	if !results[0].Success || results[1].Success {
		t.Fatalf("expected first ok second fail: %+v", results)
	}
}

func TestPingModel_StartAndDone(t *testing.T) {
	old := pingDial
	pingDial = func(addr string, timeout time.Duration) (net.Conn, error) {
		c1, _ := net.Pipe()
		return c1, nil
	}
	defer func() { pingDial = old }()

	nodes := []*common.NodeInfo{{ID: "n1", Address: "10.0.0.1", Port: 22}}
	pm := NewPingModel(nodes)
	cmd := pm.Start()
	msg := cmd()
	dm, ok := msg.(PingDoneMsg)
	if !ok {
		t.Fatalf("expected PingDoneMsg, got %T", msg)
	}
	if len(dm.Results) != 1 || !dm.Results[0].Success {
		t.Fatalf("unexpected results: %+v", dm.Results)
	}
}

func TestModel_OpenPing_FromList(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('p'))
	m = nm.(Model)
	if m.current() != LocPing {
		t.Fatalf("expected LocPing, got %v", m.current())
	}
	if m.ping == nil || !m.ping.loading {
		t.Fatal("expected ping model loading")
	}
	path := m.Path()
	if len(path) != 2 || path[1] != "ping" {
		t.Fatalf("unexpected path: %v", path)
	}
}

func TestModel_UpdatePing_DoneAndBack(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	m := NewModel(store)
	nm, _ := m.Update(runeKey('p'))
	m = nm.(Model)
	// 注入结果
	m.ping.results = []PingResult{{Node: m.visible()[0], Success: true}}
	m.ping.loading = false
	// Enter 返回列表
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(Model)
	if m.current() != LocList {
		t.Fatalf("expected back to list, got %v", m.current())
	}
}
