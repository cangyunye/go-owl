package nodeselect

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSource struct {
	rows []NodeRow
	err  error
}

func (f *fakeSource) List(ctx context.Context) ([]NodeRow, error) {
	return f.rows, f.err
}

func sampleRows() []NodeRow {
	return []NodeRow{
		{ID: "n1", Name: "web-01", Groups: []string{"web", "prod"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "web-k8s-01", Groups: []string{"web-k8s"}, Labels: map[string]string{"env": "prod", "zone": "a"}},
		{ID: "n3", Name: "db-01", Groups: []string{"db"}, Labels: map[string]string{"env": "staging"}},
		{ID: "n4", Name: "web-02", Groups: []string{"web"}, Labels: map[string]string{}, Status: "offline"},
	}
}

func TestSelect_GroupExactMatch(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{Groups: []string{"web"}})
	require.NoError(t, err)
	var ids []string
	for _, n := range got {
		ids = append(ids, n.ID)
	}
	assert.ElementsMatch(t, []string{"n1", "n4"}, ids)
}

func TestSelect_LabelsAND(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{
		Labels: map[string]string{"env": "prod", "zone": "a"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n2", got[0].ID)
}

func TestSelect_LabelKeyOnly(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{
		Labels: map[string]string{"zone": ""},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n2", got[0].ID)
}

func TestSelect_IDAndName(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{NodeIDs: []string{"n1", "db-01"}})
	require.NoError(t, err)
	var ids []string
	for _, n := range got {
		ids = append(ids, n.ID)
	}
	assert.ElementsMatch(t, []string{"n1", "n3"}, ids)
}

func TestSelect_UnknownIDErrors(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	_, err := s.Select(context.Background(), SelectOptions{NodeIDs: []string{"n1", "ghost"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestSelect_PriorityNodesOverGroups(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{
		NodeIDs: []string{"n3"},
		Groups:  []string{"web"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n3", got[0].ID)
}

func TestSelect_Status(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{Status: "offline"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n4", got[0].ID)
}

func TestSelect_EmptyReturnsAll(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.Select(context.Background(), SelectOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 4)
}

func TestSelectIntersect_GroupAndLabel(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	// groups 与 labels 取交集:web 组 + env=prod -> n1
	got, err := s.SelectIntersect(context.Background(), SelectOptions{
		Groups: []string{"web"},
		Labels: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n1", got[0].ID)
}

func TestSelectIntersect_EmptyReturnsAll(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.SelectIntersect(context.Background(), SelectOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 4)
}

func TestSelectIntersect_GroupOnly(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.SelectIntersect(context.Background(), SelectOptions{Groups: []string{"web"}})
	require.NoError(t, err)
	var ids []string
	for _, n := range got {
		ids = append(ids, n.ID)
	}
	assert.ElementsMatch(t, []string{"n1", "n4"}, ids)
}

func TestSelectIntersect_NodeIDsTakePrecedence(t *testing.T) {
	s := NewSelector(&fakeSource{rows: sampleRows()})
	got, err := s.SelectIntersect(context.Background(), SelectOptions{
		NodeIDs: []string{"n2"},
		Groups:  []string{"web"},
		Labels:  map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "n2", got[0].ID)
}
