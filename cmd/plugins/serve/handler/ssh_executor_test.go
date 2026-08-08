package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNodeInfo_WithProxyJump(t *testing.T) {
	db := nodeSelectTestDB(t)
	db.Exec(`UPDATE nodes SET proxy_jump = 'jump:2222', password = 'pw' WHERE id = 'n1'`)
	e := &sshExecutor{db: db}
	info, err := e.getNodeInfo("n1")
	require.NoError(t, err)
	assert.Equal(t, "jump:2222", info.ProxyJump)
	assert.Equal(t, "pw", info.Password)
	assert.Equal(t, "root", info.User)
}

func TestGetNodeInfo_NotFound(t *testing.T) {
	db := nodeSelectTestDB(t)
	e := &sshExecutor{db: db}
	_, err := e.getNodeInfo("ghost")
	require.Error(t, err)
}

func TestDialNode_BadAddress(t *testing.T) {
	db := nodeSelectTestDB(t)
	db.Exec(`UPDATE nodes SET address = '10.255.255.1' WHERE id = 'n1'`)
	e := &sshExecutor{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := e.dialNode(ctx, "n1")
	require.Error(t, err)
}
