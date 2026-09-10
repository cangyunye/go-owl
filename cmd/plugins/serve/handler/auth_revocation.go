package handler

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// revokedKeyPrefix 是 settings 表中令牌撤销记录的键前缀。
const revokedKeyPrefix = "auth_revoked_at."

// authRevocations 记录"某用户名何时被撤销令牌"。
//
// JWT 内的角色是签发时的快照，服务端不查库校验，因此改角色、改密码或删除
// 账号后，旧 token 在到期前（默认 24h）仍持有原权限。这里记录撤销时刻，
// 中间件只需比较 token 的签发时间，无需每请求查库。
// 记录持久化在 settings 表，进程重启后依然生效。
type authRevocations struct {
	mu sync.RWMutex
	db *sql.DB
	at map[string]int64 // username → 撤销时刻(unix 秒)
}

// newAuthRevocations 创建撤销登记表并从 settings 载入已有记录。
func newAuthRevocations(ctx context.Context, db *sql.DB) *authRevocations {
	r := &authRevocations{db: db, at: make(map[string]int64)}
	if err := r.load(ctx); err != nil {
		log.Printf("serve: 加载令牌撤销记录失败: %v", err)
	}
	return r
}

func (r *authRevocations) load(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key LIKE ?`, revokedKeyPrefix+"%")
	if err != nil {
		return err
	}
	defer rows.Close()

	r.mu.Lock()
	defer r.mu.Unlock()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		ts, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			continue
		}
		r.at[strings.TrimPrefix(key, revokedKeyPrefix)] = ts
	}
	return rows.Err()
}

// Revoke 撤销某用户名下所有已签发 token（写库失败也不影响内存生效）。
func (r *authRevocations) Revoke(ctx context.Context, username string) error {
	if r == nil || username == "" {
		return nil
	}
	now := time.Now().Unix()
	r.mu.Lock()
	r.at[username] = now
	r.mu.Unlock()

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		revokedKeyPrefix+username, strconv.FormatInt(now, 10))
	if err != nil {
		log.Printf("serve: 持久化令牌撤销记录失败(%s): %v", username, err)
	}
	return err
}

// isRevoked 判断 token 是否早于该用户最近一次撤销。
//
// JWT 的 IssuedAt 只有秒级精度，因此采用"签发时刻不晚于撤销时刻即失效"的
// 从紧判定：撤销与旧 token 落在同一秒时也会被拒，不留同秒绕过窗口。
// 代价是撤销当秒内重新登录可能拿到一张立即失效的 token，重试即可。
// nil 接收者表示未启用撤销，一律返回 false。
func (r *authRevocations) isRevoked(username string, issuedAt time.Time) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	at, ok := r.at[username]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	if issuedAt.IsZero() {
		return true // 无签发时间，无法证明在撤销之后，从严拒绝
	}
	return issuedAt.Unix() <= at
}
