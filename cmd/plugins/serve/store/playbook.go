package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"gopkg.in/yaml.v3"
)

type PlaybookStore struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]*model.Playbook
}

func NewPlaybookStore(db *sql.DB) *PlaybookStore {
	return &PlaybookStore{db: db, cache: make(map[string]*model.Playbook)}
}

func playbookID(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("%x", h[:6])
}

func (s *PlaybookStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS playbooks (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT DEFAULT '',
			category    TEXT DEFAULT '',
			file_path   TEXT NOT NULL,
			tasks_count INTEGER DEFAULT 0,
			task_names  TEXT DEFAULT '[]',
			file_exists INTEGER DEFAULT 1,
			updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (s *PlaybookStore) scanRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*model.Playbook, error) {
	var id, name, desc, cat, fp string
	var tc int
	var tnJSON string
	var fe int
	var updated string
	if err := scanner.Scan(&id, &name, &desc, &cat, &fp, &tc, &tnJSON, &fe, &updated); err != nil {
		return nil, err
	}
	pb := &model.Playbook{
		ID: id, Name: name, Description: desc, Category: cat,
		FilePath: fp, TasksCount: tc, FileExists: fe == 1, UpdatedAt: updated,
	}
	json.Unmarshal([]byte(tnJSON), &pb.TaskNames)
	if pb.TaskNames == nil {
		pb.TaskNames = []string{}
	}
	return pb, nil
}

func (s *PlaybookStore) buildCache(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, category, file_path, tasks_count, task_names, file_exists, updated_at FROM playbooks`)
	if err != nil {
		return err
	}
	defer rows.Close()
	s.cache = make(map[string]*model.Playbook)
	for rows.Next() {
		pb, err := s.scanRow(rows)
		if err != nil {
			continue
		}
		s.cache[pb.ID] = pb
	}
	return nil
}

func (s *PlaybookStore) ensureCache(ctx context.Context) error {
	s.mu.RLock()
	hasEntries := len(s.cache) > 0
	s.mu.RUnlock()
	if hasEntries {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) == 0 {
		return s.buildCache(ctx)
	}
	return nil
}

func (s *PlaybookStore) List(ctx context.Context) ([]*model.Playbook, error) {
	if err := s.ensureCache(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.Playbook, 0, len(s.cache))
	for _, pb := range s.cache {
		result = append(result, pb)
	}
	return result, nil
}

func (s *PlaybookStore) Get(ctx context.Context, id string) (*model.Playbook, error) {
	if err := s.ensureCache(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if pb, ok := s.cache[id]; ok {
		return pb, nil
	}
	return nil, sql.ErrNoRows
}

func (s *PlaybookStore) ListByCategory(ctx context.Context, category string) ([]*model.Playbook, error) {
	if err := s.ensureCache(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*model.Playbook
	for _, pb := range s.cache {
		if pb.Category == category {
			result = append(result, pb)
		}
	}
	return result, nil
}

func (s *PlaybookStore) GetCategoryCounts(ctx context.Context) (map[string]int, error) {
	if err := s.ensureCache(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int)
	for _, pb := range s.cache {
		counts[pb.Category]++
	}
	return counts, nil
}

func (s *PlaybookStore) Upsert(ctx context.Context, pb *model.Playbook) error {
	tnJSON, _ := json.Marshal(pb.TaskNames)
	fe := 0
	if pb.FileExists {
		fe = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO playbooks (id, name, description, category, file_path, tasks_count, task_names, file_exists, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			file_path = excluded.file_path,
			tasks_count = excluded.tasks_count,
			task_names = excluded.task_names,
			file_exists = excluded.file_exists,
			updated_at = excluded.updated_at`,
		pb.ID, pb.Name, pb.Description, pb.Category, pb.FilePath,
		pb.TasksCount, string(tnJSON), fe, time.Now().UTC())
	if err == nil {
		s.mu.Lock()
		s.cache[pb.ID] = pb
		s.mu.Unlock()
	}
	return err
}

func (s *PlaybookStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM playbooks WHERE id = ?`, id)
	if err == nil {
		s.mu.Lock()
		delete(s.cache, id)
		s.mu.Unlock()
	}
	return err
}

type playbookYAMLMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Tasks       []any  `yaml:"tasks"`
}

func readPlaybookMeta(path string) *model.Playbook {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var meta playbookYAMLMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil
	}
	name := meta.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	var taskNames []string
	if len(meta.Tasks) > 0 {
		for _, t := range meta.Tasks {
			if m, ok := t.(map[string]interface{}); ok {
				for k := range m {
					taskNames = append(taskNames, k)
					break
				}
			}
		}
	}
	return &model.Playbook{
		ID:          playbookID(path),
		Name:        name,
		Description: meta.Description,
		FilePath:    path,
		TasksCount:  len(meta.Tasks),
		TaskNames:   taskNames,
		FileExists:  true,
	}
}

func (s *PlaybookStore) SyncFromDir(ctx context.Context, dir string) ([]*model.Playbook, []string, error) {
	diskMap := make(map[string]*model.Playbook)
	var errors []string

	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("path is not a directory: %s", dir)
	}

	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yml") && !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		pb := readPlaybookMeta(path)
		if pb == nil {
			return nil
		}

		rel, _ := filepath.Rel(dir, filepath.Dir(path))
		if rel != "." {
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			pb.Category = parts[0]
		}

		if existing, ok := diskMap[pb.ID]; ok {
			errors = append(errors, fmt.Sprintf(
				"hash collision: %q and %q have the same ID %q — rename one of them",
				existing.FilePath, pb.FilePath, pb.ID))
			return nil
		}
		diskMap[pb.ID] = pb
		return nil
	})

	var results []*model.Playbook
	for _, pb := range diskMap {
		if err := s.Upsert(ctx, pb); err == nil {
			results = append(results, pb)
		}
	}

	existing, _ := s.List(ctx)
	for _, pb := range existing {
		if _, stillExists := diskMap[pb.ID]; !stillExists {
			pb.FileExists = false
			s.Upsert(ctx, pb)
		}
	}

	return results, errors, nil
}
