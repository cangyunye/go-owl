# API 节点源设计文档

## 概述

go-owl 支持通过外部 API 获取节点连接信息，实现与 CMDB、资产管理系统等外部系统的集成。当配置了 API 源时，系统会优先从 API 获取节点信息。

## 环境变量配置

### OWL_API_ENDPOINT

指定 API 服务地址，用于获取节点信息。

```bash
export OWL_API_ENDPOINT="https://cmdb.example.com/api/v1/nodes"
```

### OWL_API_TOKEN

指定 API 认证密钥，用于请求鉴权。

```bash
export OWL_API_TOKEN="your-api-key-here"
```

### OWL_API_TIMEOUT

API 请求超时时间（秒），默认 30 秒。

```bash
export OWL_API_TIMEOUT="60"
```

## 节点源优先级

系统按以下优先级获取节点信息：

```
1. API 源 (OWL_API_ENDPOINT + OWL_API_TOKEN)
      ↓ 未配置或失败
2. 本地数据库 (DuckDB/SQLite3)
      ↓ 未找到
3. SSH 配置 (~/.ssh/config)
      ↓ 未找到
4. 命令行参数
```

## API 接口规范

### 1. 获取节点列表

**请求：**

```http
GET /api/v1/nodes HTTP/1.1
Host: cmdb.example.com
Authorization: Bearer {OWL_API_TOKEN}
Content-Type: application/json
```

**查询参数：**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `page` | int | 否 | 页码，默认 1 |
| `page_size` | int | 否 | 每页数量，默认 100 |
| `name` | string | 否 | 按名称过滤 |
| `group` | string | 否 | 按分组过滤 |
| `label` | string | 否 | 按标签过滤 |
| `status` | string | 否 | 按状态过滤 (active/inactive) |

**响应格式：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 100,
    "page": 1,
    "page_size": 100,
    "items": [
      {
        "id": "node-001",
        "name": "web-server-01",
        "hostname": "web01.example.com",
        "address": "192.168.1.10",
        "port": 22,
        "user": "root",
        "status": "active",
        "groups": ["web", "production"],
        "labels": {
          "env": "prod",
          "region": "cn-east-1",
          "os": "ubuntu22"
        },
        "ssh_key": "/path/to/private/key",
        "ssh_password": "",
        "proxy_jump": "",
        "created_at": "2024-01-01T00:00:00Z",
        "updated_at": "2024-12-01T00:00:00Z"
      }
    ]
  }
}
```

### 2. 获取单个节点

**请求：**

```http
GET /api/v1/nodes/{node_id} HTTP/1.1
Host: cmdb.example.com
Authorization: Bearer {OWL_API_TOKEN}
Content-Type: application/json
```

**响应格式：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "node-001",
    "name": "web-server-01",
    "hostname": "web01.example.com",
    "address": "192.168.1.10",
    "port": 22,
    "user": "root",
    "status": "active",
    "groups": ["web", "production"],
    "labels": {
      "env": "prod",
      "region": "cn-east-1"
    },
    "ssh_key": "/path/to/private/key",
    "ssh_password": "",
    "proxy_jump": "bastion.example.com",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-12-01T00:00:00Z"
  }
}
```

### 3. 按名称查询节点

**请求：**

```http
GET /api/v1/nodes?name=web-server-01 HTTP/1.1
Host: cmdb.example.com
Authorization: Bearer {OWL_API_TOKEN}
Content-Type: application/json
```

## 数据结构定义

### Node 节点信息

```go
type APINode struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Hostname    string            `json:"hostname"`
    Address     string            `json:"address"`
    Port        int               `json:"port"`
    User        string            `json:"user"`
    Status      string            `json:"status"`
    Groups      []string          `json:"groups"`
    Labels      map[string]string `json:"labels"`
    SSHKey      string            `json:"ssh_key"`
    SSHPassword string            `json:"ssh_password"`
    ProxyJump   string            `json:"proxy_jump"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### APIResponse API 响应

```go
type APIResponse struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data"`
}

type NodeListResponse struct {
    Total    int        `json:"total"`
    Page     int        `json:"page"`
    PageSize int        `json:"page_size"`
    Items    []APINode  `json:"items"`
}
```

## 认证方式

### Bearer Token（推荐）

```http
Authorization: Bearer {OWL_API_TOKEN}
```

### API Key Header

```http
X-API-Key: {OWL_API_TOKEN}
```

### Query Parameter

```http
GET /api/v1/nodes?api_key={OWL_API_TOKEN}
```

## 错误处理

### 错误响应格式

```json
{
  "code": 1001,
  "message": "认证失败",
  "data": null
}
```

### 错误码定义

| 错误码 | 说明 |
|--------|------|
| 0 | 成功 |
| 1001 | 认证失败 |
| 1002 | 权限不足 |
| 1003 | 节点不存在 |
| 2001 | 参数错误 |
| 5001 | 服务器内部错误 |

## 缓存策略

为减少 API 调用，系统实现本地缓存：

- **缓存时间**: 5 分钟（可配置）
- **缓存键**: `owl:node:{node_id}`
- **失效条件**:
  - 超时
  - 手动刷新 (`owl node refresh`)
  - 连接失败时重新获取

## 使用示例

### 配置环境变量

```bash
# 设置 API 源
export OWL_API_ENDPOINT="https://cmdb.example.com/api/v1/nodes"
export OWL_API_TOKEN="sk-xxxxxxxxxxxxxxxx"

# 可选：设置超时
export OWL_API_TIMEOUT="60"
```

### 使用节点

```bash
# 通过名称连接（自动从 API 获取）
owl session attach web-server-01

# 批量执行（从 API 获取分组节点）
owl exec --group web --command "uptime"

# 查看节点列表（从 API 获取）
owl node list

# 强制刷新缓存
owl node refresh
```

## 实现架构

```
┌─────────────────────────────────────────────────────┐
│                    Node Manager                      │
├─────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌────────────┐  │
│  │  API Source │  │ Local DB    │  │ SSH Config │  │
│  │  (Priority) │  │ (Fallback)  │  │ (Fallback) │  │
│  └──────┬──────┘  └──────┬──────┘  └─────┬──────┘  │
│         │                │               │         │
│         └────────────────┴───────────────┘         │
│                          │                          │
│                   Node Resolver                     │
│                          │                          │
│                   Node Cache                        │
└─────────────────────────────────────────────────────┘
```

## 安全考虑

1. **API Key 保护**
   - 不要在代码中硬编码
   - 使用环境变量传递
   - 建议使用密钥管理服务

2. **传输安全**
   - 强制使用 HTTPS
   - 验证服务器证书

3. **权限控制**
   - API Key 应具有最小权限
   - 实现细粒度的访问控制

## 配置文件支持

除环境变量外，也支持在配置文件中设置：

```yaml
# ~/.owl/config.yml
api:
  endpoint: "https://cmdb.example.com/api/v1/nodes"
  key: "sk-xxxxxxxxxxxxxxxx"
  timeout: 60
  cache_ttl: 300
```

## 命令行参数

```bash
# 临时指定 API 源
owl node list --api-endpoint https://cmdb.example.com/api/v1/nodes --api-key sk-xxx

# 禁用 API 源，使用本地数据
owl node list --no-api

# 刷新缓存
owl node refresh
```

## 参考实现

API 服务端可以参考以下实现：

### Python FastAPI 示例（推荐）

```python
# cmdb_server.py
# 安装依赖: pip install fastapi uvicorn pydantic

from fastapi import FastAPI, HTTPException, Header, Query
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import Optional, List, Dict
from datetime import datetime
import os

app = FastAPI(title="CMDB API", version="1.0.0")

# CORS 配置
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# 数据模型
class Node(BaseModel):
    id: str
    name: str
    hostname: Optional[str] = ""
    address: str
    port: int = 22
    user: str = "root"
    status: str = "active"
    groups: List[str] = []
    labels: Dict[str, str] = {}
    ssh_key: Optional[str] = ""
    ssh_password: Optional[str] = ""
    proxy_jump: Optional[str] = ""
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None

class APIResponse(BaseModel):
    code: int
    message: str
    data: Optional[dict] = None

class NodeListResponse(BaseModel):
    total: int
    page: int
    page_size: int
    items: List[Node]

# 模拟数据库
NODES_DB: Dict[str, Node] = {}

# 初始化测试数据
def init_test_data():
    global NODES_DB
    NODES_DB = {
        "node-001": Node(
            id="node-001",
            name="web-server-01",
            hostname="web01.example.com",
            address="192.168.1.10",
            port=22,
            user="root",
            status="active",
            groups=["web", "production"],
            labels={"env": "prod", "region": "cn-east-1"},
            ssh_key="/ssh/keys/web01.pem",
            created_at=datetime.now(),
            updated_at=datetime.now()
        ),
        "node-002": Node(
            id="node-002",
            name="db-server-01",
            hostname="db01.example.com",
            address="192.168.1.20",
            port=22,
            user="ubuntu",
            status="active",
            groups=["database", "production"],
            labels={"env": "prod", "region": "cn-east-1"},
            ssh_key="/ssh/keys/db01.pem",
            created_at=datetime.now(),
            updated_at=datetime.now()
        ),
    }

init_test_data()

# 认证依赖
API_KEY = os.getenv("API_KEY", "sk-secret-key")

async def verify_api_key(authorization: str = Header(None)):
    if not authorization:
        raise HTTPException(status_code=401, detail="缺少认证信息")
    token = authorization.replace("Bearer ", "")
    if token != API_KEY:
        raise HTTPException(status_code=401, detail="认证失败")
    return token

# API 路由
@app.get("/api/v1/nodes", response_model=APIResponse)
async def list_nodes(
    name: Optional[str] = Query(None, description="按名称过滤"),
    group: Optional[str] = Query(None, description="按分组过滤"),
    label: Optional[str] = Query(None, description="按标签过滤"),
    page: int = Query(1, ge=1, description="页码"),
    page_size: int = Query(100, ge=1, le=1000, description="每页数量"),
    authorization: str = Header(None)
):
    await verify_api_key(authorization)

    items = list(NODES_DB.values())

    # 过滤
    if name:
        items = [n for n in items if name.lower() in n.name.lower()]
    if group:
        items = [n for n in items if group in n.groups]
    if label:
        items = [n for n in items if label in n.labels or label in [f"{k}={v}" for k, v in n.labels.items()]]

    total = len(items)
    start = (page - 1) * page_size
    end = start + page_size
    page_items = items[start:end]

    return APIResponse(
        code=0,
        message="success",
        data={
            "total": total,
            "page": page,
            "page_size": page_size,
            "items": [n.dict() for n in page_items]
        }
    )

@app.get("/api/v1/nodes/{node_id}", response_model=APIResponse)
async def get_node(
    node_id: str,
    authorization: str = Header(None)
):
    await verify_api_key(authorization)

    node = NODES_DB.get(node_id)
    if not node:
        raise HTTPException(status_code=404, detail="节点不存在")

    return APIResponse(
        code=0,
        message="success",
        data=node.dict()
    )

@app.post("/api/v1/nodes", response_model=APIResponse)
async def create_node(
    node: Node,
    authorization: str = Header(None)
):
    await verify_api_key(authorization)

    if node.id in NODES_DB:
        raise HTTPException(status_code=400, detail="节点已存在")

    node.created_at = datetime.now()
    node.updated_at = datetime.now()
    NODES_DB[node.id] = node

    return APIResponse(
        code=0,
        message="success",
        data=node.dict()
    )

@app.put("/api/v1/nodes/{node_id}", response_model=APIResponse)
async def update_node(
    node_id: str,
    node: Node,
    authorization: str = Header(None)
):
    await verify_api_key(authorization)

    if node_id not in NODES_DB:
        raise HTTPException(status_code=404, detail="节点不存在")

    node.updated_at = datetime.now()
    NODES_DB[node_id] = node

    return APIResponse(
        code=0,
        message="success",
        data=node.dict()
    )

@app.delete("/api/v1/nodes/{node_id}", response_model=APIResponse)
async def delete_node(
    node_id: str,
    authorization: str = Header(None)
):
    await verify_api_key(authorization)

    if node_id not in NODES_DB:
        raise HTTPException(status_code=404, detail="节点不存在")

    del NODES_DB[node_id]

    return APIResponse(
        code=0,
        message="success",
        data=None
    )

@app.get("/health")
async def health_check():
    return {"status": "ok"}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
```

**启动服务：**

```bash
# 设置 API Key
export API_KEY="sk-xxxxxxxxxxxxxxxx"

# 启动服务
python cmdb_server.py

# 或者使用 uvicorn
uvicorn cmdb_server:app --host 0.0.0.0 --port 8000 --reload
```

**测试 API：**

```bash
# 获取节点列表
curl -X GET "http://localhost:8000/api/v1/nodes" \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx"

# 获取单个节点
curl -X GET "http://localhost:8000/api/v1/nodes/node-001" \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx"

# 按名称过滤
curl -X GET "http://localhost:8000/api/v1/nodes?name=web" \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx"

# 按分组过滤
curl -X GET "http://localhost:8000/api/v1/nodes?group=web" \
  -H "Authorization: Bearer sk-xxxxxxxxxxxxxxxx"
```

### Python Flask 示例

```python
from flask import Flask, jsonify, request

app = Flask(__name__)

@app.route('/api/v1/nodes', methods=['GET'])
def list_nodes():
    api_key = request.headers.get('Authorization', '').replace('Bearer ', '')
    if not validate_api_key(api_key):
        return jsonify({'code': 1001, 'message': '认证失败'}), 401

    nodes = query_nodes_from_db()
    return jsonify({
        'code': 0,
        'message': 'success',
        'data': {
            'total': len(nodes),
            'items': nodes
        }
    })

@app.route('/api/v1/nodes/<node_id>', methods=['GET'])
def get_node(node_id):
    node = query_node_by_id(node_id)
    if not node:
        return jsonify({'code': 1003, 'message': '节点不存在'}), 404
    return jsonify({'code': 0, 'message': 'success', 'data': node})
```

### Go Gin 示例

```go
func ListNodes(c *gin.Context) {
    apiKey := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
    if !validateAPIKey(apiKey) {
        c.JSON(401, gin.H{"code": 1001, "message": "认证失败"})
        return
    }

    nodes := queryNodesFromDB()
    c.JSON(200, gin.H{
        "code": 0,
        "message": "success",
        "data": gin.H{
            "total": len(nodes),
            "items": nodes,
        },
    })
}

func GetNode(c *gin.Context) {
    apiKey := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
    if !validateAPIKey(apiKey) {
        c.JSON(401, gin.H{"code": 1001, "message": "认证失败"})
        return
    }

    nodeID := c.Param("node_id")
    node := queryNodeByID(nodeID)
    if node == nil {
        c.JSON(404, gin.H{"code": 1003, "message": "节点不存在"})
        return
    }

    c.JSON(200, gin.H{"code": 0, "message": "success", "data": node})
}
```
