package nodes

import (
	"net"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestRunCheck_SuccessAndFail(t *testing.T) {
	fn := func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
		if n.ID == "n2" {
			return false, "", &net.OpError{Err: net.ErrClosed}
		}
		return true, "key", nil
	}
	nodes := []*common.NodeInfo{
		{ID: "n1", Address: "10.0.0.1", Port: 22, SSHKey: "~/.ssh/id_rsa"},
		{ID: "n2", Address: "10.0.0.2", Port: 22},
	}
	results := runCheck(nodes, time.Second, fn)
	if !results[0].Success || results[1].Success {
		t.Fatalf("unexpected: %+v", results)
	}
	if results[0].Method != "key" {
		t.Fatalf("expected method key, got %q", results[0].Method)
	}
}

func TestModel_Check_WritesBackStatus(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store) // n1 online, n2 offline, n3 online
	old := sshCheck
	sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
		return true, "key", nil
	}
	defer func() { sshCheck = old }()

	m := NewModel(store)
	nm, _ := m.Update(runeKey('k'))
	m = nm.(Model)
	if m.current() != LocCheck {
		t.Fatalf("expected LocCheck, got %v", m.current())
	}
	// 模拟 check 完成(全部成功)
	nm, _ = m.updateCheck(CheckDoneMsg{Results: []CheckResult{
		{Node: m.visible()[0], Success: true, Method: "key"},
		{Node: m.visible()[1], Success: true, Method: "password"},
		{Node: m.visible()[2], Success: true, Method: "key"},
	}})
	m = nm.(Model)
	// 状态应全部回写 online
	for _, n := range m.nodes {
		got, err := store.Get(n.ID)
		if err != nil {
			t.Fatalf("get %s: %v", n.ID, err)
		}
		if got.Status != "online" {
			t.Fatalf("expected %s online, got %q", n.ID, got.Status)
		}
		if got.LastCheckAt == "" {
			t.Fatalf("expected %s last_check set", n.ID)
		}
	}
	if m.current() != LocList {
		t.Fatalf("expected back to list after done, got %v", m.current())
	}
}

func TestModel_Check_FailKeepsOffline(t *testing.T) {
	store := newTestStore(t)
	seedNodes(t, store)
	old := sshCheck
	sshCheck = func(n *common.NodeInfo, timeout time.Duration) (bool, string, error) {
		return false, "", &net.OpError{Err: net.ErrClosed}
	}
	defer func() { sshCheck = old }()

	m := NewModel(store)
	nm, _ := m.Update(runeKey('k'))
	m = nm.(Model)
	nm, _ = m.updateCheck(CheckDoneMsg{Results: []CheckResult{
		{Node: m.visible()[0], Success: false, Err: &net.OpError{Err: net.ErrClosed}},
	}})
	m = nm.(Model)
	got, _ := store.Get("n1")
	if got.Status != "offline" {
		t.Fatalf("expected n1 offline, got %q", got.Status)
	}
	if got.LastCheckAt == "" {
		t.Fatal("expected last_check set even on failure")
	}
}
