package history

import (
	"testing"
	"time"
)


// TestParseDuration：--last 参数的时长解析（支持 h/m/s 与 d 后缀）。
func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		err  bool
	}{
		{"", 0, false},
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"2d", 48 * time.Hour, false},
		{"90s", 90 * time.Second, false},
		{"xxh", 0, true},
		{"1x", 0, true},
	}
	for _, c := range cases {
		got, err := ParseDuration(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseDuration(%q): expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseDuration(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
