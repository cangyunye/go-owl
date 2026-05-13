package history

import (
	"database/sql"
)

// globalDB 全局数据库实例
var globalDB DBInterface

// DBInterface 定义历史记录数据库的统一接口
type DBInterface interface {
	Connection() *sql.DB
	InitSchema() error
	Close() error
	Cleanup(retentionDays int) error
}

// NewDBFunc 定义创建数据库实例的函数类型
type NewDBFunc func(config *Config) (DBInterface, error)

// SetGlobalDB 设置全局数据库实例
func SetGlobalDB(db DBInterface) {
	globalDB = db
}

// GetGlobalDB 获取全局数据库实例
func GetGlobalDB() DBInterface {
	return globalDB
}

// GetDB 获取全局数据库包装器（兼容旧代码）
func GetDB() *DB {
	if globalDB == nil {
		return nil
	}
	return &DB{impl: globalDB}
}
