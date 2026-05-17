# 数据库配置

go-owl 支持两种嵌入式数据库，通过 Go 构建标签在编译时选择：

## 编译选择

### DuckDB（默认）

DuckDB 是一个高性能分析型数据库，适合复杂查询和数据分析。

```bash
# 默认编译，使用 DuckDB
go build -o owl ./cmd/cli
```

### SQLite3（备选）

SQLite3 是一个成熟的轻量级数据库，兼容性更好，适合受限环境。

```bash
# 使用构建标签编译 SQLite3 版本
go build -tags sqlite3 -o owl ./cmd/cli
```

## 自动降级策略

虽然我们使用构建标签在编译时选择，也可以通过 Makefile 或脚本实现自动降级：

```bash
#!/bin/bash

# 先尝试编译 DuckDB 版本
echo "尝试编译 DuckDB 版本..."
if go build -o owl ./cmd/cli 2>/dev/null; then
    echo "✓ DuckDB 版本编译成功"
    exit 0
fi

# DuckDB 编译失败，降级到 SQLite3
echo "⚠️  DuckDB 编译失败，降级到 SQLite3..."
if go build -tags sqlite3 -o owl ./cmd/cli; then
    echo "✓ SQLite3 版本编译成功"
    exit 0
fi

echo "✗ 编译失败"
exit 1
```

## 数据库文件

- **DuckDB**: `~/.owl/history.db`
- **SQLite3**: `~/.owl/history.db.sqlite3`

## 差异对比

| 特性 | DuckDB | SQLite3 |
|-----|--------|---------|
| 查询性能 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| 兼容性 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 二进制体积 | 较大 | 较小 |
| 类型支持 | 原生 JSON | JSON 存为 TEXT |
| 构建时间 | 较慢 | 较快 |

## 推荐使用场景

**DuckDB**:
- 需要复杂历史查询
- 数据量较大
- 分析型需求

**SQLite3**:
- 受限环境（例如需要 CGO 兼容性问题）
- 简单的 CRUD 操作
- 追求最小体积
