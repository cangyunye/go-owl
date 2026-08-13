package nodes

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

func TestParseFilterQuery_Groups(t *testing.T) {
	fq := ParseFilterQuery("g:web,db")
	if len(fq.Groups) != 2 || fq.Groups[0] != "web" || fq.Groups[1] != "db" {
		t.Fatalf("unexpected groups: %#v", fq.Groups)
	}
	if fq.Empty() {
		t.Fatal("expected not empty")
	}
}

func TestParseFilterQuery_Labels(t *testing.T) {
	fq := ParseFilterQuery("l:env=prod,os=debian")
	if len(fq.Labels) != 2 || fq.Labels["env"] != "prod" || fq.Labels["os"] != "debian" {
		t.Fatalf("unexpected labels: %#v", fq.Labels)
	}
}

func TestParseFilterQuery_Search(t *testing.T) {
	fq := ParseFilterQuery("web-1")
	if fq.Search != "web-1" {
		t.Fatalf("unexpected search: %q", fq.Search)
	}
}

func TestParseFilterQuery_Mixed(t *testing.T) {
	fq := ParseFilterQuery("g:web l:env=prod cache")
	if len(fq.Groups) != 1 || fq.Groups[0] != "web" {
		t.Fatalf("groups: %#v", fq.Groups)
	}
	if fq.Labels["env"] != "prod" {
		t.Fatalf("labels: %#v", fq.Labels)
	}
	if fq.Search != "cache" {
		t.Fatalf("search: %q", fq.Search)
	}
}

func TestParseFilterQuery_Empty(t *testing.T) {
	if !ParseFilterQuery("").Empty() {
		t.Fatal("empty query should be Empty")
	}
}

func TestApplyFilter_Groups(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "n1", Groups: []string{"web"}},
		{ID: "n2", Groups: []string{"db"}},
		{ID: "n3", Groups: []string{"cache", "web"}},
	}
	fq := ParseFilterQuery("g:web")
	got := applyFilter(nodes, fq)
	if len(got) != 2 || got[0].ID != "n1" || got[1].ID != "n3" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Labels(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "n1", Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Labels: nil},
	}
	fq := ParseFilterQuery("l:env=prod")
	got := applyFilter(nodes, fq)
	if len(got) != 1 || got[0].ID != "n1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Search(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "web-1", Name: "prod-web", Address: "10.0.0.1"},
		{ID: "db-1", Name: "prod-db", Address: "10.0.0.2"},
	}
	got := applyFilter(nodes, ParseFilterQuery("10.0.0.1"))
	if len(got) != 1 || got[0].ID != "web-1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Empty_ReturnsAll(t *testing.T) {
	nodes := []*common.NodeInfo{{ID: "n1"}, {ID: "n2"}}
	if got := applyFilter(nodes, FilterQuery{}); len(got) != 2 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestApplyFilter_Empty_ReturnsNothing(t *testing.T) {
	var nodes []*common.NodeInfo
	if got := applyFilter(nodes, ParseFilterQuery("g:web")); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestApplyFilter_Search_MultipleTermsAND(t *testing.T) {
	nodes := []*common.NodeInfo{
		{ID: "web-1", Name: "cache-srv", Address: "10.0.0.1"},
		{ID: "web-2", Name: "web-2", Address: "10.0.0.2"},
	}
	got := applyFilter(nodes, ParseFilterQuery("web cache"))
	if len(got) != 1 || got[0].ID != "web-1" {
		t.Fatalf("expected AND per-term match, got: %+v", got)
	}
}

func TestGroupsIntersect(t *testing.T) {
	if !groupsIntersect([]string{"a", "b"}, []string{"b", "c"}) {
		t.Fatal("expected true")
	}
	if groupsIntersect([]string{"a"}, []string{"b"}) {
		t.Fatal("expected false")
	}
}
