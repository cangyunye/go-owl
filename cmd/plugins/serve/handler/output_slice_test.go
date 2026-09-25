package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 增量取输出的切片语义：客户端游标落后/超前都不该报错，limit 要有上限。
func TestSliceOutput(t *testing.T) {
	out := strings.Repeat("x", 1000)

	cases := []struct {
		name         string
		offset, limit int
		wantLen      int
		wantNext     int
	}{
		{"从头取", 0, 100, 100, 100},
		{"从中间取", 900, 100, 100, 1000},
		{"偏移正好在末尾", 1000, 100, 0, 1000},
		{"偏移越界按末尾处理", 5000, 100, 0, 1000},
		{"负偏移归零", -10, 50, 50, 50},
		{"limit 为 0 走默认", 0, 0, 1000, 1000},
		{"limit 超上限被钳到 1MB", 0, 5 << 20, 1000, 1000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, next := sliceOutput(out, c.offset, c.limit)
			assert.Equal(t, c.wantLen, len(data))
			assert.Equal(t, c.wantNext, next)
		})
	}
}
