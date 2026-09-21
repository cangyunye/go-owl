package common

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want func(t *testing.T, got string)
	}{
		{"short-untouched", "abc", 10, func(t *testing.T, got string) {
			if got != "abc" {
				t.Fatalf("got %q", got)
			}
		}},
		{"ascii-exact", "abcdefghij", 7, func(t *testing.T, got string) {
			if got != "abcd..." {
				t.Fatalf("got %q", got)
			}
		}},
		{"tiny-budget", "abcdef", 2, func(t *testing.T, got string) {
			if len(got) > 2 || !utf8.ValidString(got) {
				t.Fatalf("got %q", got)
			}
		}},
		{"zero", "abc", 0, func(t *testing.T, got string) {
			if got != "" {
				t.Fatalf("got %q", got)
			}
		}},
		{"cjk-boundary", strings.Repeat("运", 40), 100, func(t *testing.T, got string) {
			if len(got) > 100 || !utf8.ValidString(got) {
				t.Fatalf("got %q", got)
			}
			if !strings.HasSuffix(got, "...") {
				t.Fatalf("missing ellipsis: %q", got)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.want(t, Truncate(c.in, c.max))
		})
	}
}
