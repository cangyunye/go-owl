package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	globalLogger *zap.Logger
)

// Config 日志配置
type Config struct {
	Level        string // 日志级别: debug, info, warn, error
	Console      bool   // 是否输出到控制台
	ConsoleLevel string // 控制台日志级别
	File         bool   // 是否输出到文件
	FilePath     string // 日志文件路径
	MaxSize      int    // 单个日志文件最大大小(MB)
	MaxBackups   int    // 保留的备份文件数量
	MaxAge       int    // 保留天数
	Compress     bool   // 是否压缩
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDir, ".owl", "logs", "owl.log")

	return &Config{
		Level:        "info",
		Console:      true,
		ConsoleLevel: "info",
		File:         true,
		FilePath:     logPath,
		MaxSize:      100,
		MaxBackups:   10,
		MaxAge:       30,
		Compress:     true,
	}
}

// Init 初始化日志系统
func Init(config *Config) error {
	if config == nil {
		config = DefaultConfig()
	}

	cores := []zapcore.Core{}

	// 文件输出
	if config.File {
		ensureLogDir(config.FilePath)

		fileWriter := &lumberjack.Logger{
			Filename:   config.FilePath,
			MaxSize:    config.MaxSize,
			MaxBackups: config.MaxBackups,
			MaxAge:     config.MaxAge,
			Compress:   config.Compress,
		}

		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(zapcore.EncoderConfig{
				TimeKey:        "timestamp",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				MessageKey:     "msg",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.LowercaseLevelEncoder,
				EncodeTime:     zapcore.ISO8601TimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			}),
			zapcore.AddSync(fileWriter),
			parseLevel(config.Level),
		)
		cores = append(cores, fileCore)
	}

	// 控制台输出
	if config.Console {
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
				TimeKey:        "timestamp",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				MessageKey:     "msg",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.CapitalColorLevelEncoder,
				EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
				EncodeDuration: zapcore.StringDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			}),
			zapcore.AddSync(os.Stdout),
			parseLevel(config.ConsoleLevel),
		)
		cores = append(cores, consoleCore)
	}

	globalLogger = zap.New(zapcore.NewTee(cores...), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	return nil
}

func ensureLogDir(path string) {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0755)
	}
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// GetLogger 获取全局 logger
func GetLogger() *zap.Logger {
	if globalLogger == nil {
		Init(nil)
	}
	return globalLogger
}

// WithOperation 添加操作类型字段
func WithOperation(opType string) zap.Field {
	return zap.String("op_type", opType)
}

// WithTaskID 添加任务ID字段
func WithTaskID(taskID string) zap.Field {
	return zap.String("task_id", taskID)
}

// WithNodeID 添加节点ID字段
func WithNodeID(nodeID string) zap.Field {
	return zap.String("node_id", nodeID)
}

// 快捷函数
func Debug(msg string, fields ...zap.Field) {
	GetLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	GetLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetLogger().Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	GetLogger().Fatal(msg, fields...)
}

func Sync() {
	if globalLogger != nil {
		globalLogger.Sync()
	}
}

// WithField 添加通用字段添加函数
func WithField(key string, value interface{}) zap.Field {
	return zap.Any(key, value)
}

// WithError 添加错误字段
func WithError(err error) zap.Field {
	return zap.Error(err)
}
