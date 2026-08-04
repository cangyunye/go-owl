package nodeselect

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cangyunye/go-owl/internal/node"
)

func TestResolverSource_List(t *testing.T) {
	resolver := node.NewNodeResolver()
	src := NewResolverSource(resolver)
	rows, err := src.List(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, rows)
}
