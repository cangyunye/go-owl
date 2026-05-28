# nodes.json 密码加密方案

## 需求分析

当前 `~/.owl/nodes.json` 中的密码以明文形式存储，存在安全风险。需要实现加密存储，同时确保不影响 `exec`、`file` 等依赖节点信息的命令。

## 当前节点存储机制

```
┌─────────────────────────────────────────────────────────────┐
│                     节点数据流向                            │
├─────────────────────────────────────────────────────────────┤
│  nodes.json ──→ LocalSource.loadFromFile() ──→ LocalNode  │
│                                                      │     │
│                                                      ▼     │
│  NodeResolver.Resolve() ←─── ResolvedNode               │     │
│         │                                                  │
│         ▼                                                  │
│  SSH连接建立 (exec, file等命令) ←─── SSHPassword          │
└─────────────────────────────────────────────────────────────┘
```

## 方案选择

采用 **AES-GCM 对称加密 + 用户主密码 + 系统密钥环回退** 的方案：

| 方案 | 优点 | 缺点 |
|------|------|------|
| 系统密钥环 | 安全，密钥不落地 | 平台依赖 |
| 用户主密码 | 跨平台，可控 | 用户需记住密码 |
| 机器标识符 | 自动无需交互 | 安全性较低 |

**最终方案**：优先使用系统密钥环存储加密密钥，失败则回退到用户输入主密码派生密钥。

## 加密方案设计

### 1. 加密算法
- **算法**: AES-GCM-256
- **密钥派生**: PBKDF2-HMAC-SHA256（迭代次数 100000）
- **盐长度**: 16 bytes
- **Nonce长度**: 12 bytes (GCM推荐)

### 2. 密钥管理

```
┌─────────────────────────────────────────────────────────────┐
│                     密钥管理流程                            │
├─────────────────────────────────────────────────────────────┤
│  用户主密码 ──→ PBKDF2 ──→ 加密密钥(32字节)                │
│                          │                                │
│                          ▼                                │
│              加密 nodes.json 中的 SSHPassword              │
│                          │                                │
│                          ▼                                │
│              存储加密后的密码 + 盐 + nonce                   │
└─────────────────────────────────────────────────────────────┘
```

### 3. 数据格式

**加密后的密码存储格式**:
```json
{
  "id": "node1",
  "name": "Node 1",
  "address": "192.168.1.10",
  "port": 22,
  "user": "root",
  "password": "encrypted:<salt>:<nonce>:<ciphertext>",
  "ssh_key": "",
  "groups": [],
  "labels": {}
}
```

### 4. 兼容性设计

支持自动检测和迁移：
- 检测密码是否以 `encrypted:` 前缀开头
- 无前缀：视为明文，加载时自动加密保存
- 有前缀：按加密格式解密

## 修改的文件

### 1. 新增加密工具模块
**文件**: `internal/crypto/encryption.go`
```go
package crypto

// Encrypt 加密字符串
func Encrypt(plaintext, passphrase string) (string, error)

// Decrypt 解密字符串
func Decrypt(ciphertext, passphrase string) (string, error)

// IsEncrypted 检查是否已加密
func IsEncrypted(s string) bool
```

### 2. 修改 LocalSource
**文件**: `internal/node/local_source.go`
- 修改 `loadFromFile()`：加载时解密密码
- 修改保存逻辑：保存时加密密码

### 3. 新增密钥管理模块
**文件**: `internal/crypto/keyring.go`
- 封装系统密钥环操作
- 提供统一接口，自动回退到主密码模式

### 4. 修改节点添加/更新命令
**文件**: `cmd/cli/cmd/node/add.go`, `cmd/cli/cmd/node/update.go`
- 添加密码加密逻辑

## 透明性保证

修改仅在 `LocalSource` 层进行，上层调用无需改动：

```go
// 修改前
node, err := resolver.Resolve("node1")
sshPassword := node.SSHPassword  // 明文

// 修改后（完全相同的接口）
node, err := resolver.Resolve("node1")
sshPassword := node.SSHPassword  // 自动解密后的明文
```

## 实施步骤

1. **创建加密工具模块** (`internal/crypto/`)
2. **修改 LocalSource** 支持透明加解密
3. **实现密钥管理**（密钥环 + 主密码回退）
4. **添加迁移逻辑**（首次加载时加密现有明文密码）
5. **单元测试**覆盖加密/解密/迁移场景

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 密钥丢失 | 密码无法恢复 | 提示用户备份密钥 |
| 性能开销 | 加载/保存变慢 | 加密仅针对密码字段 |
| 兼容性破坏 | 旧版本无法读取 | 保留明文格式识别 |
| 用户体验 | 需要输入密码 | 优先使用密钥环，缓存主密码 |

## 文件清单

| 文件路径 | 操作 | 说明 |
|----------|------|------|
| `internal/crypto/encryption.go` | 新增 | 加密解密核心逻辑 |
| `internal/crypto/keyring.go` | 新增 | 系统密钥环封装 |
| `internal/node/local_source.go` | 修改 | 添加加解密逻辑 |
| `cmd/cli/cmd/node/add.go` | 修改 | 添加密码加密 |
| `cmd/cli/cmd/node/update.go` | 修改 | 添加密码加密 |
| `internal/crypto/encryption_test.go` | 新增 | 加密单元测试 |
| `internal/node/local_source_test.go` | 修改 | 添加加密测试 |