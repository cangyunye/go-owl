package cache

import (
	"context"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
)

type PlaybookStore interface {
	List(ctx context.Context) ([]*model.Playbook, error)
	Get(ctx context.Context, id string) (*model.Playbook, error)
	ListByCategory(ctx context.Context, category string) ([]*model.Playbook, error)
	Upsert(ctx context.Context, pb *model.Playbook) error
	Delete(ctx context.Context, id string) error
	SyncFromDir(ctx context.Context, dir string) ([]*model.Playbook, []string, error)
	GetCategoryCounts(ctx context.Context) (map[string]int, error)
}
