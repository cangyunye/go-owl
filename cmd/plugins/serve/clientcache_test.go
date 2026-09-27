package serve

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// 客户端减负：全量节点在浏览器侧共享缓存 + 并发去重，写路径负责失效。
// 没有这层缓存时，8 个标签各自打开选择器就会把同一份全量节点拉 8 遍。
func TestClientCache_NodesAllShared(t *testing.T) {
	api := readWebFile(t, "web/js/api.js")

	assert.Contains(t, api, "NODES_ALL_TTL", "全量节点缓存需要 TTL")
	assert.Contains(t, api, "inflight", "并发请求要去重（多个标签同时打开选择器）")
	assert.Contains(t, api, "invalidateNodesAll", "写路径必须能立即失效缓存")

	// 写路径的每个入口都要失效，漏一个就会出现「改了节点但选择器还是旧数据」
	for _, call := range []string{
		"api.invalidateNodesAll(); return r; }",   // create/update/delete/batchGroup
	} {
		assert.GreaterOrEqual(t, strings.Count(api, call), 4,
			"节点写路径（创建/更新/删除/批量分组）都要失效缓存")
	}
	assert.Contains(t, api, "api.invalidateNodesAll();", "导入节点后也要失效缓存")
}
