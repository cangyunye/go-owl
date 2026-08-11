package store

import (
	"context"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandStore_CreateAppendsPosition(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	c1 := &model.UserCommand{UserID: 1, Name: "磁盘", Command: "df -h"}
	require.NoError(t, s.Create(ctx, c1))
	assert.Equal(t, 0, c1.Position)

	c2 := &model.UserCommand{UserID: 1, Name: "内存", Command: "free -h"}
	require.NoError(t, s.Create(ctx, c2))
	assert.Equal(t, 1, c2.Position)

	c3 := &model.UserCommand{UserID: 2, Name: "x", Command: "echo x"}
	require.NoError(t, s.Create(ctx, c3))
	assert.Equal(t, 0, c3.Position, "position 按用户独立计数")
}

func TestCommandStore_ListByUser_OrdersByPosition(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 1, Name: "a", Command: "echo a"}))
	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 2, Name: "other", Command: "echo o"}))
	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 1, Name: "c", Command: "echo c"}))

	list, err := s.ListByUser(ctx, 1)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"a", "c"}, []string{list[0].Name, list[1].Name})
}

func TestCommandStore_UpdateDelete_RespectOwnership(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	cmd := &model.UserCommand{UserID: 1, Name: "a", Command: "echo a"}
	require.NoError(t, s.Create(ctx, cmd))

	// 其他用户改不了
	affected, err := s.Update(ctx, &model.UserCommand{ID: cmd.ID, UserID: 2, Name: "hack", Command: "hack"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
	affected, err = s.Delete(ctx, cmd.ID, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(0), affected)
	list, _ := s.ListByUser(ctx, 1)
	require.Len(t, list, 1)
	assert.Equal(t, "a", list[0].Name)

	// 本人可改可删
	affected, err = s.Update(ctx, &model.UserCommand{ID: cmd.ID, UserID: 1, Name: "改名", Command: "echo b"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	list, _ = s.ListByUser(ctx, 1)
	require.Len(t, list, 1)
	assert.Equal(t, "改名", list[0].Name)

	affected, err = s.Delete(ctx, cmd.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	list, _ = s.ListByUser(ctx, 1)
	assert.Empty(t, list)
}

func TestCommandStore_Reorder(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	c1 := &model.UserCommand{UserID: 1, Name: "1", Command: "c1"}
	c2 := &model.UserCommand{UserID: 1, Name: "2", Command: "c2"}
	c3 := &model.UserCommand{UserID: 1, Name: "3", Command: "c3"}
	for _, c := range []*model.UserCommand{c1, c2, c3} {
		require.NoError(t, s.Create(ctx, c))
	}

	require.NoError(t, s.Reorder(ctx, 1, []int64{c3.ID, c1.ID, c2.ID}))
	list, err := s.ListByUser(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []string{"3", "1", "2"}, []string{list[0].Name, list[1].Name, list[2].Name})
}

func TestCommandStore_CountByUser(t *testing.T) {
	db := openTestDB(t)
	s := NewCommandStore(db)
	ctx := context.Background()
	require.NoError(t, s.Init(ctx))

	n, err := s.CountByUser(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: 7, Name: "a", Command: "echo a"}))
	n, _ = s.CountByUser(ctx, 7)
	assert.Equal(t, 1, n)
}

func TestCommandStore_CascadeOnUserDelete(t *testing.T) {
	db := openTestDB(t)
	_, err := db.Exec("PRAGMA foreign_keys=ON")
	require.NoError(t, err)
	ctx := context.Background()

	us := NewUserStore(db)
	require.NoError(t, us.Init(ctx))
	user := &model.User{Username: "u1", PasswordHash: "h", Role: "viewer"}
	require.NoError(t, us.Create(ctx, user))

	s := NewCommandStore(db)
	require.NoError(t, s.Init(ctx))
	require.NoError(t, s.Create(ctx, &model.UserCommand{UserID: user.ID, Name: "a", Command: "echo a"}))

	require.NoError(t, us.Delete(ctx, user.ID))
	n, err := s.CountByUser(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}
