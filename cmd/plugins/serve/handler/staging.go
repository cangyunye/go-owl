package handler

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type StagingHandler struct {
	db *sql.DB
}

type StagingFile struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	CreateTime time.Time `json:"create_time"`
}

type DiskInfo struct {
	Total       uint64 `json:"total"`
	Used        uint64 `json:"used"`
	Free        uint64 `json:"free"`
	Threshold   uint64 `json:"threshold"`
	StagingDir  string `json:"staging_dir"`
}

func NewStagingHandler(db *sql.DB) *StagingHandler {
	return &StagingHandler{db: db}
}

func (h *StagingHandler) stagingDir() string {
	var dir string
	err := h.db.QueryRow(`SELECT value FROM settings WHERE key = 'staging_dir'`).Scan(&dir)
	if err != nil || dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".owl", "staging")
	}
	return dir
}

func (h *StagingHandler) minFreeBytes() uint64 {
	var val string
	err := h.db.QueryRow(`SELECT value FROM settings WHERE key = 'staging_min_free'`).Scan(&val)
	if err != nil || val == "" {
		return 10 * 1024 * 1024 * 1024 // 10GB default
	}
	gb, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 10 * 1024 * 1024 * 1024
	}
	return gb * 1024 * 1024 * 1024
}

func (h *StagingHandler) diskFree() (uint64, error) {
	dir := h.stagingDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}
	_, free, err := fsStat(dir)
	return free, err
}

func (h *StagingHandler) Upload(c *gin.Context) {
	dir := h.stagingDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot create staging directory"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "file is required"})
		return
	}
	defer file.Close()

	name := header.Filename
	size := header.Size

	if size == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "empty file"})
		return
	}

	free, err := h.diskFree()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot check disk space"})
		return
	}

	minFree := h.minFreeBytes()
	if free < minFree {
		c.JSON(http.StatusInsufficientStorage, gin.H{
			"code":    507,
			"message": fmt.Sprintf("insufficient disk space: %d bytes free, %d bytes required", free, minFree),
		})
		return
	}

	if uint64(size) > free-minFree {
		c.JSON(http.StatusInsufficientStorage, gin.H{
			"code":    507,
			"message": fmt.Sprintf("file too large: %d bytes exceeds available space", size),
		})
		return
	}

	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err == nil {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		ext := filepath.Ext(name)
		for i := 1; ; i++ {
			dest = filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
			if _, err := os.Stat(dest); os.IsNotExist(err) {
				break
			}
		}
	}

	out, err := os.Create(dest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot write file"})
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		os.Remove(dest)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "write failed"})
		return
	}

	fi, _ := os.Stat(dest)
	c.JSON(http.StatusAccepted, gin.H{
		"name": filepath.Base(dest),
		"size": fi.Size(),
		"path": dest,
	})
}

func (h *StagingHandler) List(c *gin.Context) {
	dir := h.stagingDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []StagingFile{}})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": []StagingFile{}})
		return
	}

	files := make([]StagingFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, StagingFile{
			Name:       e.Name(),
			Size:       fi.Size(),
			ModTime:    fi.ModTime(),
			CreateTime: getBirthTime(fi),
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": files})
}

func (h *StagingHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid filename"})
		return
	}

	dir := h.stagingDir()
	path := filepath.Join(dir, name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "file not found"})
		return
	}

	if err := os.Remove(path); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "delete failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *StagingHandler) DiskInfo(c *gin.Context) {
	dir := h.stagingDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot access staging directory"})
		return
	}

	total, free, err := fsStat(dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cannot get disk info"})
		return
	}

	used := total - free

	c.JSON(http.StatusOK, DiskInfo{
		Total:      total,
		Used:       used,
		Free:       free,
		Threshold:  h.minFreeBytes(),
		StagingDir: dir,
	})
}
