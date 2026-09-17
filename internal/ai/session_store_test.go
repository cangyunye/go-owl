package ai

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/internal/common/model"
)

func newTestSQLiteStore(t *testing.T) *SQLiteSessionStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	store, err := NewSQLiteSessionStore(db)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	return store
}

func TestSQLiteSessionStore_RoundTrip(t *testing.T) {
	store := newTestSQLiteStore(t)

	rec := &SessionRecord{
		SessionID: "sess-1",
		Host:      "web",
		Title:     "列出节点",
		State: &SessionState{
			Messages: []Message{
				{Role: "system", Content: "sys"},
				{Role: "user", Content: "列出节点"},
				{Role: "assistant", Content: "完成"},
			},
			Dialogue: []Message{{Role: "user", Content: "列出节点"}},
			NodeContext: &NodeContext{
				Nodes: []string{"node1", "node2"}, Source: "group=web 的节点",
			},
		},
	}
	if err := store.Save(rec); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load("sess-1", "web")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got.Title != "列出节点" {
		t.Errorf("expected title, got %q", got.Title)
	}
	if len(got.State.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got.State.Messages))
	}
	if got.State.NodeContext == nil || len(got.State.NodeContext.Nodes) != 2 {
		t.Errorf("expected node context restored, got %+v", got.State.NodeContext)
	}

	// 覆盖保存（upsert）
	rec.Title = "更新后的标题"
	rec.State.Messages = append(rec.State.Messages, Message{Role: "user", Content: "第二条"})
	if err := store.Save(rec); err != nil {
		t.Fatalf("upsert Save failed: %v", err)
	}
	got, _ = store.Load("sess-1", "web")
	if got.Title != "更新后的标题" || len(got.State.Messages) != 4 {
		t.Errorf("expected upserted record, got title=%q msgs=%d", got.Title, len(got.State.Messages))
	}

	// host 隔离
	if _, err := store.Load("sess-1", "cli"); err == nil {
		t.Error("expected not-found for other host")
	}

	// List
	metas, err := store.List("web", 10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(metas) != 1 || metas[0].SessionID != "sess-1" {
		t.Errorf("unexpected list result: %+v", metas)
	}

	// Delete
	if err := store.Delete("sess-1", "web"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := store.Load("sess-1", "web"); err == nil {
		t.Error("expected not-found after delete")
	}
}

// TestSessionManager_PersistAndRestore 会话经 Send 后持久化；新的 Manager
// （模拟进程重启）从同一 store 恢复后，多轮上下文必须延续。
func TestSessionManager_PersistAndRestore(t *testing.T) {
	store := newTestSQLiteStore(t)

	toolCallJSON := "```json\n" + `{"tool_calls":[{"name":"query_nodes","arguments":{}}]}` + "\n```"

	// 第一进程：两轮对话
	m1 := &mockChatModel{responses: []string{
		"node_list", toolCallJSON, "第一轮完成：有节点。",
		"第一轮完成：有节点。", // 恢复后第二轮的 ProcessWithContext 直接总结（历史里已有工具结果）
	}}
	agent1, _ := NewAgent(nil, &Config{}, &mockNodeMgrForAI{nodes: nodesForAI()}, nil, nil)
	agent1.SetChatModel(m1)

	mgr1 := NewSessionManagerWithStore(store, "cli")
	sess1 := mgr1.CreateSession("my-session", agent1)
	if _, err := sess1.Send(context.Background(), "列出所有节点"); err != nil {
		t.Fatalf("first send failed: %v", err)
	}

	// 第二进程：全新 Manager + 全新 agent，恢复同一会话
	agent2, _ := NewAgent(nil, &Config{}, &mockNodeMgrForAI{nodes: nodesForAI()}, nil, nil)
	m2 := &mockChatModel{responses: []string{
		"上一轮你问的是节点列表，已完成。",
	}}
	agent2.SetChatModel(m2)

	mgr2 := NewSessionManagerWithStore(store, "cli")
	sess2, ok := mgr2.GetOrLoadSession("my-session", agent2)
	if !ok {
		t.Fatal("expected session restored from store")
	}
	if len(sess2.messages) == 0 {
		t.Fatal("expected persisted messages to be restored")
	}

	reply, err := sess2.Send(context.Background(), "我刚才问了什么？")
	if err != nil {
		t.Fatalf("second send failed: %v", err)
	}
	if reply != "上一轮你问的是节点列表，已完成。" {
		t.Fatalf("unexpected reply %q", reply)
	}

	// 恢复后的会话发给 LLM 的消息必须包含上一轮的用户输入（上下文延续）
	foundPrev := false
	for _, msg := range m2.responses {
		_ = msg
	}
	for _, msg := range sess2.messages {
		if msg.Role == "user" && strings.Contains(msg.Content, "列出所有节点") {
			foundPrev = true
		}
	}
	if !foundPrev {
		t.Error("expected previous user input in restored messages")
	}
}

// TestSessionManager_MemoryOnly 原构造保持纯内存语义（store 为 nil）。
func TestSessionManager_MemoryOnly(t *testing.T) {
	mgr := NewSessionManager()
	agent, _ := NewAgent(nil, &Config{}, &mockNodeMgrForAI{nodes: nodesForAI()}, nil, nil)
	mgr.CreateSession("s1", agent)
	if _, ok := mgr.GetOrLoadSession("s1", agent); !ok {
		t.Error("expected in-memory hit")
	}
	if _, ok := mgr.GetOrLoadSession("missing", agent); ok {
		t.Error("expected miss without store")
	}
}

func nodesForAI() []*model.Node {
	return []*model.Node{
		{Name: "node1", Address: "127.0.0.1", Port: 22, Status: "online"},
	}
}
