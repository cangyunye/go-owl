package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	_ "modernc.org/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserStore_CreateAndFind(t *testing.T) {
	db := openTestDB(t)
	store := NewUserStore(db)
	require.NoError(t, store.Init(context.Background()))

	user := &model.User{
		Username:     "admin",
		PasswordHash: "$2a$10$hash",
		Role:         model.RoleAdmin,
		DisplayName:  "Administrator",
	}

	err := store.Create(context.Background(), user)
	require.NoError(t, err)
	assert.Greater(t, user.ID, int64(0))

	found, err := store.FindByUsername(context.Background(), "admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", found.Username)
	assert.Equal(t, model.RoleAdmin, found.Role)
	assert.Equal(t, "$2a$10$hash", found.PasswordHash)
}

func TestUserStore_FindByUsername_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := NewUserStore(db)
	require.NoError(t, store.Init(context.Background()))

	_, err := store.FindByUsername(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestUserStore_List(t *testing.T) {
	db := openTestDB(t)
	store := NewUserStore(db)
	require.NoError(t, store.Init(context.Background()))

	store.Create(context.Background(), &model.User{Username: "alice", Role: model.RoleViewer})
	store.Create(context.Background(), &model.User{Username: "bob", Role: model.RoleAdmin})

	users, err := store.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

func TestUserStore_ListPaged(t *testing.T) {
	db := openTestDB(t)
	store := NewUserStore(db)
	require.NoError(t, store.Init(context.Background()))

	for i, name := range []string{"alice", "bob", "charlie", "dave", "eve", "carol"} {
		u := &model.User{Username: name, Role: model.RoleViewer}
		if i == 0 {
			u.DisplayName = "Alicia Admin"
		}
		require.NoError(t, store.Create(context.Background(), u))
	}

	total, err := store.Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 6, total)

	users, total, err := store.ListPaged(context.Background(), "", 1, 2)
	require.NoError(t, err)
	assert.Equal(t, 6, total, "total must reflect all users regardless of page size")
	assert.Len(t, users, 2, "page of size 2 must return 2 users")
	assert.Equal(t, "alice", users[0].Username)
	assert.Equal(t, "bob", users[1].Username)

	users, total, err = store.ListPaged(context.Background(), "", 3, 2)
	require.NoError(t, err)
	assert.Equal(t, 6, total)
	assert.Len(t, users, 2, "last page holds the remainder")

	users, total, err = store.ListPaged(context.Background(), "ar", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 2, total, "search must match both username and display_name")
	names := []string{}
	for _, u := range users {
		names = append(names, u.Username)
	}
	assert.Contains(t, names, "carol")
	assert.Contains(t, names, "charlie")

	users, total, err = store.ListPaged(context.Background(), "zzz", 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, users)
}

func TestUserStore_Update(t *testing.T) {
	db := openTestDB(t)
	store := NewUserStore(db)
	require.NoError(t, store.Init(context.Background()))

	store.Create(context.Background(), &model.User{Username: "alice", Role: model.RoleViewer})

	alice, _ := store.FindByUsername(context.Background(), "alice")
	alice.Role = model.RoleAdmin
	err := store.Update(context.Background(), alice)
	require.NoError(t, err)

	updated, _ := store.FindByUsername(context.Background(), "alice")
	assert.Equal(t, model.RoleAdmin, updated.Role)
}

func TestUserStore_Delete(t *testing.T) {
	db := openTestDB(t)
	store := NewUserStore(db)
	require.NoError(t, store.Init(context.Background()))

	store.Create(context.Background(), &model.User{Username: "alice", Role: model.RoleViewer})
	alice, _ := store.FindByUsername(context.Background(), "alice")

	err := store.Delete(context.Background(), alice.ID)
	require.NoError(t, err)

	_, err = store.FindByUsername(context.Background(), "alice")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
