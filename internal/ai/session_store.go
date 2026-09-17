package ai

import (
	"strings"
	"time"
)

// SessionState 是会话的可持久化快照。messages 保存 LLM 上下文（含工具结果
// 回注消息），dialogue/operations/nodeContext 供记忆拼接与上下文复用。
type SessionState struct {
	Messages    []Message          `json:"messages"`
	History     []string           `json:"history,omitempty"`
	Operations  []OperationSummary `json:"operations,omitempty"`
	Dialogue    []Message          `json:"dialogue,omitempty"`
	NodeContext *NodeContext       `json:"node_context,omitempty"`
}

// SessionRecord 是持久化层的一行会话记录
type SessionRecord struct {
	SessionID string
	Host      string // 宿主标识：web / cli
	Title     string // 会话标题（首条用户输入截断）
	State     *SessionState
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SessionMeta 是会话列表的元信息
type SessionMeta struct {
	SessionID string
	Host      string
	Title     string
	UpdatedAt time.Time
}

// SessionStore 是会话持久化接口。实现方可为 SQLite（双宿主共用）、
// 内存或其它存储。
type SessionStore interface {
	Save(rec *SessionRecord) error
	Load(sessionID, host string) (*SessionRecord, error)
	List(host string, limit int) ([]SessionMeta, error)
	Delete(sessionID, host string) error
}

// maxPersistMessages 是单会话持久化的 LLM 消息上限（超出截断最旧的）。
const maxPersistMessages = 40

// Snapshot 导出会话快照（messages 截断保留最近 maxPersistMessages 条）。
func (s *Session) Snapshot() *SessionState {
	msgs := s.messages
	if len(msgs) > maxPersistMessages {
		msgs = msgs[len(msgs)-maxPersistMessages:]
	}
	st := &SessionState{
		Messages:   append([]Message{}, msgs...),
		History:    append([]string{}, s.history...),
		Operations: append([]OperationSummary{}, s.operations...),
		Dialogue:   append([]Message{}, s.dialogue...),
	}
	if s.nodeContext != nil {
		nc := *s.nodeContext
		st.NodeContext = &nc
	}
	return st
}

// restoreSession 用持久化快照重建会话（绑定给定 agent）。
// pendingContext 不持久化：重启后确认态视为取消（安全侧默认）。
func restoreSession(agent *Agent, rec *SessionRecord) *Session {
	s := NewSession(agent)
	if rec.State == nil {
		return s
	}
	if len(rec.State.Messages) > 0 {
		s.messages = append([]Message{}, rec.State.Messages...)
	}
	if len(rec.State.History) > 0 {
		s.history = append([]string{}, rec.State.History...)
	}
	if len(rec.State.Operations) > 0 {
		s.operations = append([]OperationSummary{}, rec.State.Operations...)
	}
	if len(rec.State.Dialogue) > 0 {
		s.dialogue = append([]Message{}, rec.State.Dialogue...)
	}
	if rec.State.NodeContext != nil {
		nc := *rec.State.NodeContext
		s.nodeContext = &nc
	}
	if rec.CreatedAt.IsZero() {
		s.createdAt = rec.CreatedAt
	}
	return s
}

// sessionTitle 从会话历史提取标题（首条用户输入截断）。
func sessionTitle(s *Session) string {
	if len(s.history) > 0 && strings.HasPrefix(s.history[0], "User: ") {
		title := strings.TrimPrefix(s.history[0], "User: ")
		title = strings.TrimSpace(title)
		if len(title) > 50 {
			title = title[:50] + "…"
		}
		return title
	}
	return ""
}
