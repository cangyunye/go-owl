package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stagingTestDB(t *testing.T) (*sql.DB, *StagingHandler, string) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	require.NoError(t, err)

	dir := t.TempDir()
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_dir', ?)`, dir)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('staging_min_free', '1')`)
	require.NoError(t, err)

	return db, NewStagingHandler(db), dir
}

func stagingRouter(t *testing.T, h *StagingHandler, uploadRoles, deleteRoles, listRoles []model.Role) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware())

	op := auth.Group("", ah.RBACMiddleware(uploadRoles...))
	op.POST("/staging/upload", h.Upload)

	adm := auth.Group("", ah.RBACMiddleware(deleteRoles...))
	adm.DELETE("/staging/:name", h.Delete)

	rd := auth.Group("", ah.RBACMiddleware(listRoles...))
	rd.GET("/staging/files", h.List)
	rd.GET("/staging/disk", h.DiskInfo)

	return r
}

func testTokenOp(t *testing.T) string {
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, err := as.GenerateToken("operator", "operator")
	require.NoError(t, err)
	return token
}

func testTokenViewer(t *testing.T) string {
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, err := as.GenerateToken("viewer", "viewer")
	require.NoError(t, err)
	return token
}

func testTokenAdmin(t *testing.T) string {
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	token, err := as.GenerateToken("admin", "admin")
	require.NoError(t, err)
	return token
}

func multipartUpload(t *testing.T, router *gin.Engine, token string, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader(content))
	require.NoError(t, err)
	w.Close()

	req, _ := http.NewRequest("POST", "/api/v1/staging/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// Case 1: 正常上传
func TestStagingUpload_Basic(t *testing.T) {
	_, h, dir := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenOp(t)

	rec := multipartUpload(t, router, token, "test.txt", "hello world")
	assert.Equal(t, 202, rec.Code)

	var resp struct {
		Name string `json:"name"`
		Size int    `json:"size"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Equal(t, "test.txt", resp.Name)
	assert.Equal(t, 11, resp.Size)

	_, err := os.Stat(filepath.Join(dir, "test.txt"))
	assert.NoError(t, err)
}

// Case 3: 文件名已存在自动重命名
func TestStagingUpload_DuplicateFilename(t *testing.T) {
	_, h, dir := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenOp(t)

	// Upload first
	rec1 := multipartUpload(t, router, token, "test.txt", "first")
	assert.Equal(t, 202, rec1.Code)

	// Upload same name
	rec2 := multipartUpload(t, router, token, "test.txt", "second")
	assert.Equal(t, 202, rec2.Code)

	var resp struct {
		Name string `json:"name"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &resp)
	assert.Equal(t, "test_1.txt", resp.Name)

	_, err := os.Stat(filepath.Join(dir, "test.txt"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "test_1.txt"))
	assert.NoError(t, err)
}

// Case 4: viewer 角色不能上传
func TestStagingUpload_ForbiddenForViewer(t *testing.T) {
	_, h, _ := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenViewer(t) // viewer token won't have upload perms

	rec := multipartUpload(t, router, token, "test.txt", "data")
	assert.Equal(t, 403, rec.Code)
}

// Case 5: 列出中转站文件
func TestStagingList_Basic(t *testing.T) {
	_, h, _ := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenViewer(t)
	opToken := testTokenOp(t)

	// Upload 3 files
	multipartUpload(t, router, opToken, "a.txt", "aaa")
	multipartUpload(t, router, opToken, "b.log", "bbbb")
	multipartUpload(t, router, opToken, "c.tar", "ccccc")

	req, _ := http.NewRequest("GET", "/api/v1/staging/files", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp struct {
		Data []StagingFile `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 3)
	assert.Equal(t, "a.txt", resp.Data[0].Name)
	assert.Equal(t, int64(3), resp.Data[0].Size)
}

// Case 6: 空中转站
func TestStagingList_Empty(t *testing.T) {
	_, h, _ := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenViewer(t)

	req, _ := http.NewRequest("GET", "/api/v1/staging/files", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var resp struct {
		Data []StagingFile `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 0)
}

// Case 7: Admin 删除
func TestStagingDelete_ByAdmin(t *testing.T) {
	_, h, dir := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	opToken := testTokenOp(t)
	adminToken := testTokenAdmin(t)

	multipartUpload(t, router, opToken, "delete-me.txt", "content")

	req, _ := http.NewRequest("DELETE", "/api/v1/staging/delete-me.txt", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)

	_, err := os.Stat(filepath.Join(dir, "delete-me.txt"))
	assert.True(t, os.IsNotExist(err))
}

// Case 8: Operator 不能删除
func TestStagingDelete_ForbiddenForOperator(t *testing.T) {
	_, h, dir := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	opToken := testTokenOp(t)

	multipartUpload(t, router, opToken, "keep.txt", "data")

	req, _ := http.NewRequest("DELETE", "/api/v1/staging/keep.txt", nil)
	req.Header.Set("Authorization", "Bearer "+opToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, 403, rec.Code)

	_, err := os.Stat(filepath.Join(dir, "keep.txt"))
	assert.NoError(t, err)
}

// Case 9: 删除不存在
func TestStagingDelete_NotFound(t *testing.T) {
	_, h, _ := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	adminToken := testTokenAdmin(t)

	req, _ := http.NewRequest("DELETE", "/api/v1/staging/nonexistent.zip", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, 404, rec.Code)
}

// Case 10: 磁盘空间信息
func TestStagingDiskInfo(t *testing.T) {
	_, h, _ := stagingTestDB(t)
	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenViewer(t)

	req, _ := http.NewRequest("GET", "/api/v1/staging/disk", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	var info DiskInfo
	json.Unmarshal(rec.Body.Bytes(), &info)
	assert.Equal(t, uint64(1*1024*1024*1024), info.Threshold)
	assert.NotEmpty(t, info.StagingDir)
	assert.Equal(t, info.Total, info.Used+info.Free)
}

// Case 2: 磁盘空间不足
func TestStagingUpload_InsufficientSpace(t *testing.T) {
	db, h, _ := stagingTestDB(t)
	// Override with impossibly high threshold
	db.Exec(`UPDATE settings SET value = '999999999' WHERE key = 'staging_min_free'`)

	router := stagingRouter(t, h, []model.Role{model.RoleOperator}, []model.Role{model.RoleAdmin}, []model.Role{model.RoleViewer})
	token := testTokenOp(t)

	rec := multipartUpload(t, router, token, "bigfile.dat", "some data")
	assert.Equal(t, 507, rec.Code)
}
