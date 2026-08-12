package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openPlaybookTestDB(t *testing.T) *PlaybookStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := NewPlaybookStore(db)
	require.NoError(t, s.Init(context.Background()))
	return s
}

func makePlaybook(id, name, filePath, category string) *model.Playbook {
	return &model.Playbook{
		ID:         id,
		Name:       name,
		FilePath:   filePath,
		Category:   category,
		FileExists: true,
		TaskNames:  []string{},
	}
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestPlaybookID(t *testing.T) {
	h := sha256.Sum256([]byte("/some/path/test.yaml"))
	expected := fmt.Sprintf("%x", h[:6])
	assert.Equal(t, expected, playbookID("/some/path/test.yaml"))
	assert.Len(t, playbookID("/some/path/test.yaml"), 12)
}

func TestPlaybookStore_UpsertAndGet(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	pb := makePlaybook("id1", "test-pb", "/tmp/test.yaml", "")
	require.NoError(t, s.Upsert(ctx, pb))

	got, err := s.Get(ctx, "id1")
	require.NoError(t, err)
	assert.Equal(t, "id1", got.ID)
	assert.Equal(t, "test-pb", got.Name)
	assert.Equal(t, "/tmp/test.yaml", got.FilePath)
}

func TestPlaybookStore_GetNotFound(t *testing.T) {
	s := openPlaybookTestDB(t)
	_, err := s.Get(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestPlaybookStore_List(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, makePlaybook("id1", "pb1", "/a.yaml", "")))
	require.NoError(t, s.Upsert(ctx, makePlaybook("id2", "pb2", "/b.yaml", "")))

	list, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestPlaybookStore_Delete(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, makePlaybook("id1", "pb1", "/a.yaml", "")))
	require.NoError(t, s.Delete(ctx, "id1"))

	_, err := s.Get(ctx, "id1")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestPlaybookStore_ListByCategory(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, makePlaybook("id1", "pb1", "/a.yaml", "web")))
	require.NoError(t, s.Upsert(ctx, makePlaybook("id2", "pb2", "/b.yaml", "db")))
	require.NoError(t, s.Upsert(ctx, makePlaybook("id3", "pb3", "/c.yaml", "web")))

	webPBs, err := s.ListByCategory(ctx, "web")
	require.NoError(t, err)
	assert.Len(t, webPBs, 2)

	dbPBs, err := s.ListByCategory(ctx, "db")
	require.NoError(t, err)
	assert.Len(t, dbPBs, 1)
}

func TestPlaybookStore_GetCategoryCounts(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, makePlaybook("id1", "pb1", "/a.yaml", "web")))
	require.NoError(t, s.Upsert(ctx, makePlaybook("id2", "pb2", "/b.yaml", "db")))
	require.NoError(t, s.Upsert(ctx, makePlaybook("id3", "pb3", "/c.yaml", "web")))

	counts, err := s.GetCategoryCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, counts["web"])
	assert.Equal(t, 1, counts["db"])
}

func TestPlaybookStore_SyncFromDir(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, filepath.Join(dir, "ping.yaml"), `name: ping
description: Ping test
tasks:
  - name: ping localhost
    command: ping -c 1 127.0.0.1
`)
	writeYAML(t, filepath.Join(dir, "web", "deploy.yaml"), `name: deploy
description: Deploy web app
tasks:
  - name: deploy app
    command: echo deploy
`)

	s := openPlaybookTestDB(t)
	ctx := context.Background()

	results, errs, err := s.SyncFromDir(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, errs)
	assert.Len(t, results, 2)

	list, _ := s.List(ctx)
	assert.Len(t, list, 2)

	var deployPB *model.Playbook
	for _, pb := range list {
		if pb.Name == "deploy" {
			deployPB = pb
		}
	}
	require.NotNil(t, deployPB)
	assert.Equal(t, "web", deployPB.Category)
}

func TestPlaybookStore_SyncFromDir_MarksMissing(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, "a.yaml"), `name: alpha
tasks:
  - name: task1
    command: echo hi
`)

	s := openPlaybookTestDB(t)
	ctx := context.Background()

	_, _, err := s.SyncFromDir(ctx, dir)
	require.NoError(t, err)

	os.Remove(filepath.Join(dir, "a.yaml"))

	_, _, err = s.SyncFromDir(ctx, dir)
	require.NoError(t, err)

	list, _ := s.List(ctx)
	require.Len(t, list, 1)
	assert.False(t, list[0].FileExists)
}

func TestPlaybookStore_SyncFromDir_InvalidDir(t *testing.T) {
	s := openPlaybookTestDB(t)
	_, _, err := s.SyncFromDir(context.Background(), "/nonexistent/dir")
	assert.Error(t, err)
}

func TestPlaybookStore_UpsertUpdatesCache(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	pb := makePlaybook("id1", "original", "/a.yaml", "")
	require.NoError(t, s.Upsert(ctx, pb))

	got, _ := s.Get(ctx, "id1")
	assert.Equal(t, "original", got.Name)

	pb.Name = "updated"
	require.NoError(t, s.Upsert(ctx, pb))

	got, _ = s.Get(ctx, "id1")
	assert.Equal(t, "updated", got.Name)
}

func TestPlaybookStore_DeleteRemovesFromCache(t *testing.T) {
	s := openPlaybookTestDB(t)
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, makePlaybook("id1", "pb1", "/a.yaml", "")))
	require.NoError(t, s.Delete(ctx, "id1"))

	_, err := s.Get(ctx, "id1")
	assert.ErrorIs(t, err, sql.ErrNoRows)

	list, _ := s.List(ctx)
	assert.Empty(t, list)
}

func TestPlaybookStore_SyncFromDir_NoCollisionFalsePositives(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, filepath.Join(dir, "alpha.yaml"), `name: alpha
tasks:
  - name: task1
    command: echo alpha
`)
	writeYAML(t, filepath.Join(dir, "beta.yaml"), `name: beta
tasks:
  - name: task1
    command: echo beta
`)
	writeYAML(t, filepath.Join(dir, "sub", "gamma.yaml"), `name: gamma
tasks:
  - name: task1
    command: echo gamma
`)
	writeYAML(t, filepath.Join(dir, "sub", "delta.yaml"), `name: delta
tasks:
  - name: task1
    command: echo delta
`)

	s := openPlaybookTestDB(t)
	ctx := context.Background()

	results, errs, err := s.SyncFromDir(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, errs, "no hash collisions should be reported for distinct file paths")
	assert.Len(t, results, 4)

	list, err := s.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 4)

	ids := make(map[string]bool)
	for _, pb := range list {
		assert.False(t, ids[pb.ID], "each playbook should have a unique ID")
		ids[pb.ID] = true
	}
}

func TestPlaybookStore_SyncFromDir_CategoryFromYAML(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, "tagged.yaml"), `name: tagged
category: web
tasks:
  - name: task1
    command: echo hi
`)

	s := openPlaybookTestDB(t)
	ctx := context.Background()

	_, errs, err := s.SyncFromDir(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, errs)

	list, _ := s.List(ctx)
	require.Len(t, list, 1)
	assert.Equal(t, "web", list[0].Category, "root-level playbook must keep YAML-embedded category")
}

func TestPlaybookStore_SyncFromDir_SubdirCategoryWins(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, "web", "deploy.yaml"), `name: deploy
category: db
tasks:
  - name: task1
    command: echo hi
`)

	s := openPlaybookTestDB(t)
	ctx := context.Background()

	_, errs, err := s.SyncFromDir(ctx, dir)
	require.NoError(t, err)
	assert.Empty(t, errs)

	list, _ := s.List(ctx)
	require.Len(t, list, 1)
	assert.Equal(t, "web", list[0].Category, "path-derived category must win over YAML for subdir playbooks")
}

func TestPlaybookStore_SyncFromDir_FilePathReturnsError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte("name: test\n"), 0644))

	s := openPlaybookTestDB(t)
	_, _, err := s.SyncFromDir(context.Background(), filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}
