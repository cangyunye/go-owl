package serve

import (
	"context"
	"log"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
)

// UserCreatedHook 用户创建成功后的供给钩子(见 ADR-0001)。
// 失败只记日志,不阻塞用户创建。
type UserCreatedHook func(ctx context.Context, userID int64) error

// defaultShortcutCommands 新用户默认快捷命令(New-User Defaults)。
var defaultShortcutCommands = []model.UserCommand{
	{Name: "磁盘占用", Command: "df -h"},
	{Name: "我的进程", Command: "ps -fu $LOGNAME"},
	{Name: "内存", Command: "free -h"},
}

// seedDefaultShortcuts 为新用户播种默认快捷命令。
func seedDefaultShortcuts(ctx context.Context, cs *store.CommandStore, userID int64) error {
	for _, d := range defaultShortcutCommands {
		cmd := d
		cmd.UserID = userID
		if err := cs.Create(ctx, &cmd); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) runUserCreatedHooks(ctx context.Context, userID int64) {
	for _, h := range s.userCreatedHooks {
		if err := h(ctx, userID); err != nil {
			log.Printf("user-created hook failed for user %d: %v", userID, err)
		}
	}
}

func (s *Server) RegisterUserCreatedHook(h UserCreatedHook) {
	s.userCreatedHooks = append(s.userCreatedHooks, h)
}
