package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// seedNodeSpec 单个种子节点定义。
type seedNodeSpec struct {
	id      string
	name    string
	address string
	user    string
	groups  []string
	labels  map[string]string
}

// seedNodeSpecs 生成 50 个多样化测试节点，分组覆盖 web/db/cache/worker/monitor/gateway/kafka，
// 标签覆盖 env/app/owner/region/role/engine/task。
func seedNodeSpecs() []seedNodeSpec {
	var specs []seedNodeSpec

	// web: 10 台（10.0.1.x）
	webUsers := []string{"nginx", "apache"}
	webApps := []string{"nginx", "httpd"}
	webOwners := []string{"zhangwei", "wangfang", "zhaoming", "lisong", "chenyu"}
	webRegions := []string{"us-east", "us-west", "ap-east", "ap-south"}
	for i := 1; i <= 10; i++ {
		env := "prod"
		if i >= 9 {
			env = "dev"
		} else if i == 4 || i == 5 || i == 7 {
			env = "staging"
		}
		u := webUsers[(i-1)%len(webUsers)]
		app := webApps[(i-1)%len(webApps)]
		specs = append(specs, seedNodeSpec{
			id:      "web-0" + strconv.Itoa(i),
			name:    "web-" + app + "-0" + strconv.Itoa(i),
			address: "10.0.1." + strconv.Itoa(10+i),
			user:    u,
			groups:  []string{"web", env},
			labels: map[string]string{
				"env": env, "app": app,
				"owner": webOwners[(i-1)%len(webOwners)], "region": webRegions[(i-1)%len(webRegions)],
			},
		})
	}

	// db: 10 台（10.0.2.x）
	dbEngines := []string{"mysql", "postgres"}
	dbOwners := []string{"huangli", "zhangwei", "zhaoming", "chenyu", "liuyi"}
	for i := 1; i <= 10; i++ {
		env := "prod"
		if i >= 9 {
			env = "dev"
		} else if i >= 7 {
			env = "staging"
		}
		engine := dbEngines[(i-1)%len(dbEngines)]
		role := "master"
		if i%2 == 0 {
			role = "slave"
		}
		specs = append(specs, seedNodeSpec{
			id:      "db-0" + strconv.Itoa(i),
			name:    "db-" + engine + "-" + role + "-0" + strconv.Itoa(i),
			address: "10.0.2." + strconv.Itoa(10+i),
			user:    "oracle",
			groups:  []string{"db", env},
			labels: map[string]string{
				"env": env, "engine": engine, "role": role, "owner": dbOwners[(i-1)%len(dbOwners)],
			},
		})
	}

	// cache: 6 台（10.0.3.x）
	cacheEngines := []string{"redis", "memcache"}
	cacheOwners := []string{"wangfang", "lisong", "huangli", "zhaoming"}
	for i := 1; i <= 6; i++ {
		env := "prod"
		if i == 6 {
			env = "dev"
		}
		specs = append(specs, seedNodeSpec{
			id:      "cache-0" + strconv.Itoa(i),
			name:    "cache-" + cacheEngines[(i-1)%len(cacheEngines)] + "-0" + strconv.Itoa(i),
			address: "10.0.3." + strconv.Itoa(10+i),
			user:    "redis",
			groups:  []string{"cache", env},
			labels: map[string]string{
				"env": env, "engine": cacheEngines[(i-1)%len(cacheEngines)],
				"role": "main", "owner": cacheOwners[(i-1)%len(cacheOwners)],
			},
		})
	}

	// worker: 7 台（10.0.4.x）
	workerTasks := []string{"job", "etl", "flume"}
	workerOwners := []string{"zhangwei", "chenyu", "wangfang", "lisong", "liuyi"}
	for i := 1; i <= 7; i++ {
		env := "prod"
		if i == 7 {
			env = "dev"
		}
		specs = append(specs, seedNodeSpec{
			id:      "worker-0" + strconv.Itoa(i),
			name:    "worker-" + workerTasks[(i-1)%len(workerTasks)] + "-0" + strconv.Itoa(i),
			address: "10.0.4." + strconv.Itoa(10+i),
			user:    "ttadm",
			groups:  []string{"worker", env},
			labels: map[string]string{
				"env": env, "task": workerTasks[(i-1)%len(workerTasks)], "owner": workerOwners[(i-1)%len(workerOwners)],
			},
		})
	}

	// monitor: 7 台（10.0.5.x）
	monitorApps := []string{"prometheus", "grafana", "elasticsearch", "alertmanager"}
	monitorOwners := []string{"zhaoming", "huangli", "zhangwei", "chenyu", "liuyi"}
	for i := 1; i <= 7; i++ {
		env := "prod"
		if i == 7 {
			env = "dev"
		} else if i == 6 {
			env = "staging"
		}
		specs = append(specs, seedNodeSpec{
			id:      "monitor-0" + strconv.Itoa(i),
			name:    "monitor-" + monitorApps[(i-1)%len(monitorApps)] + "-0" + strconv.Itoa(i),
			address: "10.0.5." + strconv.Itoa(10+i),
			user:    "ttadm",
			groups:  []string{"monitor", env},
			labels: map[string]string{
				"env": env, "app": monitorApps[(i-1)%len(monitorApps)], "owner": monitorOwners[(i-1)%len(monitorOwners)],
			},
		})
	}

	// gateway: 7 台（10.0.6.x）
	gatewayApps := []string{"haproxy", "nginx", "metallb"}
	gatewayOwners := []string{"wangfang", "zhangwei", "lisong", "chenyu", "liuyi"}
	for i := 1; i <= 7; i++ {
		env := "prod"
		if i == 7 {
			env = "dev"
		} else if i == 6 {
			env = "staging"
		}
		specs = append(specs, seedNodeSpec{
			id:      "gateway-0" + strconv.Itoa(i),
			name:    "gw-" + gatewayApps[(i-1)%len(gatewayApps)] + "-0" + strconv.Itoa(i),
			address: "10.0.6." + strconv.Itoa(10+i),
			user:    "ttadm",
			groups:  []string{"gateway", env},
			labels: map[string]string{
				"env": env, "app": gatewayApps[(i-1)%len(gatewayApps)], "owner": gatewayOwners[(i-1)%len(gatewayOwners)],
			},
		})
	}

	// kafka: 3 台（10.0.7.x）
	kafkaOwners := []string{"huangli", "zhangwei"}
	for i := 1; i <= 3; i++ {
		specs = append(specs, seedNodeSpec{
			id:      "kafka-0" + strconv.Itoa(i),
			name:    "kafka-broker-0" + strconv.Itoa(i),
			address: "10.0.7." + strconv.Itoa(10+i),
			user:    "kafka",
			groups:  []string{"kafka", "production"},
			labels: map[string]string{
				"env": "prod", "role": "broker", "owner": kafkaOwners[(i-1)%len(kafkaOwners)],
			},
		})
	}

	return specs
}

// Seed 批量插入 50 个多样化测试节点（已存在的 ID 跳过），便于开发调试。
// 幂等：重复调用只插入尚未存在的节点。
func (h *NodeHandler) Seed(c *gin.Context) {
	specs := seedNodeSpecs()
	now := time.Now().UTC().Format(time.RFC3339)

	created := 0
	skipped := 0

	for _, spec := range specs {
		groupsJSON, _ := json.Marshal(spec.groups)
		labelsJSON, _ := json.Marshal(spec.labels)

		res, err := h.db.Exec(
			`INSERT OR IGNORE INTO nodes (id, name, address, port, user, status, groups, labels, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'offline', ?, ?, ?, ?)`,
			spec.id, spec.name, spec.address, 22, spec.user, string(groupsJSON), string(labelsJSON), now, now,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "seed failed: " + err.Error()})
			return
		}
		if n, _ := res.RowsAffected(); n > 0 {
			created++
		} else {
			skipped++
		}
	}

	if created > 0 {
		ids := make([]string, 0, len(specs))
		for _, spec := range specs {
			ids = append(ids, spec.id)
		}
		h.recordNodeManage(c, "node seed", ids)
	}
	c.JSON(http.StatusOK, gin.H{"created": created, "skipped": skipped, "total": len(specs)})
}
