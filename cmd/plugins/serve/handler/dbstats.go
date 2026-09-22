package handler

import (
	"database/sql"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// DBStatsHandler 数据库可观测性与空间回收（admin）：
// owl.db 为监控指标、告警、任务历史等共用的单文件库，管理员需要
// 直观看到各表行数/占用并能在膨胀后手动回收。
type DBStatsHandler struct {
	db     *sql.DB
	dbPath string
}

func NewDBStatsHandler(db *sql.DB, dbPath string) *DBStatsHandler {
	return &DBStatsHandler{db: db, dbPath: dbPath}
}

type dbTableStat struct {
	Name      string `json:"name"`
	Rows      int64  `json:"rows"`
	SizeBytes int64  `json:"size_bytes"`
}

type dbFileInfo struct {
	SizeBytes     int64 `json:"size_bytes"`
	WALSizeBytes  int64 `json:"wal_size_bytes"`
	PageCount     int64 `json:"page_count"`
	PageSize      int64 `json:"page_size"`
	FreelistPages int64 `json:"freelist_pages"`
	FreelistBytes int64 `json:"freelist_bytes"`
}

type dbStatsResponse struct {
	Tables          []dbTableStat `json:"tables"`
	File            dbFileInfo    `json:"file"`
	TotalTableBytes int64         `json:"total_size_bytes"`
}

// Stats GET /api/v1/db/stats：全库每张表的行数与字节占用 + 文件级信息。
// 大表 COUNT(*) 为全表扫描，仅 admin 低频调用。
func (h *DBStatsHandler) Stats(c *gin.Context) {
	resp, err := h.collectStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "collect stats failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Vacuum POST /api/v1/db/vacuum：立即空间回收（VACUUM + WAL 截断），
// 返回回收前后的文件大小。与引擎的定期清理互不冲突（busy_timeout 排队）。
func (h *DBStatsHandler) Vacuum(c *gin.Context) {
	before, err := h.fileSize()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "stat db file failed"})
		return
	}
	if _, err := h.db.Exec("VACUUM"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "vacuum failed"})
		return
	}
	if _, err := h.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "wal checkpoint failed"})
		return
	}
	after, _ := h.fileSize()
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"size_bytes_before": before,
		"size_bytes_after":  after,
	}})
}

func (h *DBStatsHandler) collectStats() (*dbStatsResponse, error) {
	names, err := h.tableNames()
	if err != nil {
		return nil, err
	}
	sizes := h.tableSizes()
	resp := &dbStatsResponse{Tables: make([]dbTableStat, 0, len(names))}
	for _, n := range names {
		var count int64
		if err := h.db.QueryRow("SELECT COUNT(*) FROM " + n).Scan(&count); err != nil {
			return nil, err
		}
		resp.Tables = append(resp.Tables, dbTableStat{Name: n, Rows: count, SizeBytes: sizes[n]})
		resp.TotalTableBytes += sizes[n]
	}
	resp.File = h.fileInfo()
	return resp, nil
}

func (h *DBStatsHandler) tableNames() ([]string, error) {
	rows, err := h.db.Query(
		`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// tableSizes 经 dbstat 虚表取每表字节；虚表不可用时返回空映射（0 = 未知）。
func (h *DBStatsHandler) tableSizes() map[string]int64 {
	sizes := map[string]int64{}
	rows, err := h.db.Query(`SELECT name, SUM(pgsize) FROM dbstat GROUP BY name`)
	if err != nil {
		return sizes
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		var size int64
		if err := rows.Scan(&name, &size); err != nil {
			return sizes
		}
		sizes[name] = size
	}
	return sizes
}

func (h *DBStatsHandler) fileInfo() dbFileInfo {
	var info dbFileInfo
	_ = h.db.QueryRow("PRAGMA page_count").Scan(&info.PageCount)
	_ = h.db.QueryRow("PRAGMA page_size").Scan(&info.PageSize)
	_ = h.db.QueryRow("PRAGMA freelist_count").Scan(&info.FreelistPages)
	info.FreelistBytes = info.FreelistPages * info.PageSize
	info.SizeBytes, _ = h.fileSize()
	if st, err := os.Stat(h.dbPath + "-wal"); err == nil {
		info.WALSizeBytes = st.Size()
	}
	return info
}

func (h *DBStatsHandler) fileSize() (int64, error) {
	st, err := os.Stat(h.dbPath)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
