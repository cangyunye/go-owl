package serve

import (
	"path/filepath"
	"testing"
)

// 回归:主库连接池必须封顶。modernc sqlite 每个连接约 2MB 页缓存常驻
// Go 堆,不设 MaxOpenConns 时并发(WS 推送 + 前端轮询 + shell 流式落库)
// 会创建任意多连接,RSS 随之无界增长且回落慢。
func TestOpenMainDB_PoolLimit(t *testing.T) {
	db, err := openMainDB(filepath.Join(t.TempDir(), "owl.db"))
	if err != nil {
		t.Fatalf("openMainDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := db.Stats().MaxOpenConnections; got != 8 {
		t.Errorf("main pool MaxOpenConnections = %d, want 8", got)
	}
}
