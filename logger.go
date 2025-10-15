package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	appConfig "mimi/config"

	"github.com/sirupsen/logrus"
)

// 日志文件配置
const (
	LogFileName   = "mimi.log"
	MaxLogSize    = 10 * 1024 * 1024 // 10MB
	MaxLogBackups = 5                // 保留最近5个日志文件
)

var (
	logFile      *os.File
	isDevMode    bool
	logFilePath  string
	currentSize  int64
	loggerWriter io.Writer

	// MIMI 应用层日志器
	MLog *slog.Logger
)

// PrefixHandler 为日志添加前缀的 Handler
type PrefixHandler struct {
	handler slog.Handler
	prefix  string
}

func NewPrefixHandler(w io.Writer, prefix string, opts *slog.HandlerOptions) *PrefixHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
	}
	return &PrefixHandler{
		handler: slog.NewTextHandler(w, opts),
		prefix:  prefix,
	}
}

func (h *PrefixHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *PrefixHandler) Handle(ctx context.Context, r slog.Record) error {
	// 在消息前添加前缀
	r.Message = h.prefix + r.Message
	return h.handler.Handle(ctx, r)
}

func (h *PrefixHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrefixHandler{
		handler: h.handler.WithAttrs(attrs),
		prefix:  h.prefix,
	}
}

func (h *PrefixHandler) WithGroup(name string) slog.Handler {
	return &PrefixHandler{
		handler: h.handler.WithGroup(name),
		prefix:  h.prefix,
	}
}

// InitLogger 初始化日志系统
// devMode: 是否为开发模式(true=仅输出到控制台, false=同时输出到文件和控制台)
func InitLogger(devMode bool) error {
	isDevMode = devMode

	var opts *slog.HandlerOptions
	if isDevMode {
		// 开发模式: DEBUG 级别
		opts = &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
		loggerWriter = os.Stdout
	} else {
		// 生产模式: INFO 级别
		opts = &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}

		// 获取应用数据目录
		appDataDir, err := appConfig.GetAppDataDir()
		if err != nil {
			return fmt.Errorf("获取应用数据目录失败: %w", err)
		}

		// 创建logs子目录
		logsDir := filepath.Join(appDataDir, "logs")
		if err := os.MkdirAll(logsDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %w", err)
		}

		// 日志文件路径
		logFilePath = filepath.Join(logsDir, LogFileName)

		// 检查是否需要轮转
		if err := rotateLogIfNeeded(); err != nil {
			return fmt.Errorf("日志轮转失败: %w", err)
		}

		// 打开日志文件(追加模式)
		logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %w", err)
		}

		// 获取当前文件大小
		fileInfo, err := logFile.Stat()
		if err == nil {
			currentSize = fileInfo.Size()
		}

		// 创建多路输出:同时写入文件和控制台
		loggerWriter = io.MultiWriter(os.Stdout, &rotatingWriter{file: logFile})
	}

	// 创建 MIMI 应用层日志器
	MLog = slog.New(NewPrefixHandler(loggerWriter, "[MIMI] ", opts))

	if isDevMode {
		MLog.Info("日志模式: 开发模式(仅控制台输出)")
	} else {
		MLog.Info("日志模式: 生产模式(文件+控制台输出)")
		MLog.Info("日志文件", "path", logFilePath)
	}

	return nil
}

// rotatingWriter 支持日志轮转的Writer
type rotatingWriter struct {
	file *os.File
}

func (w *rotatingWriter) Write(p []byte) (n int, err error) {
	n, err = w.file.Write(p)
	if err != nil {
		return
	}

	currentSize += int64(n)

	// 检查是否需要轮转
	if currentSize >= MaxLogSize {
		if err := rotateLog(); err != nil {
			MLog.Error("日志轮转失败", "error", err)
		}
	}

	return
}

// rotateLogIfNeeded 启动时检查是否需要轮转
func rotateLogIfNeeded() error {
	fileInfo, err := os.Stat(logFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在,无需轮转
		}
		return err
	}

	// 如果文件超过限制,进行轮转
	if fileInfo.Size() >= MaxLogSize {
		return rotateLog()
	}

	return nil
}

// rotateLog 执行日志轮转
func rotateLog() error {
	// 关闭当前日志文件
	if logFile != nil {
		logFile.Close()
	}

	// 生成备份文件名:mimi_20250111_150405.log
	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(
		filepath.Dir(logFilePath),
		fmt.Sprintf("mimi_%s.log", timestamp),
	)

	// 重命名当前日志文件
	if err := os.Rename(logFilePath, backupPath); err != nil {
		return fmt.Errorf("重命名日志文件失败: %w", err)
	}

	// 清理旧的备份文件
	if err := cleanOldBackups(); err != nil {
		MLog.Warn("清理旧备份文件失败", "error", err)
	}

	// 创建新的日志文件
	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("创建新日志文件失败: %w", err)
	}

	currentSize = 0
	MLog.Info("日志轮转完成", "backup", backupPath)

	return nil
}

// cleanOldBackups 清理旧的备份文件,只保留最近的N个
func cleanOldBackups() error {
	logsDir := filepath.Dir(logFilePath)

	// 读取目录中的所有日志备份文件
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return err
	}

	// 收集所有备份文件
	var backups []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".log" && entry.Name() != LogFileName {
			backups = append(backups, entry)
		}
	}

	// 如果备份文件数量超过限制,删除最旧的
	if len(backups) > MaxLogBackups {
		// 按修改时间排序(旧的在前)
		// 注意:DirEntry不包含修改时间,需要使用FileInfo
		type backupFile struct {
			name    string
			modTime time.Time
		}
		var backupFiles []backupFile

		for _, backup := range backups {
			info, err := backup.Info()
			if err != nil {
				continue
			}
			backupFiles = append(backupFiles, backupFile{
				name:    backup.Name(),
				modTime: info.ModTime(),
			})
		}

		// 简单排序:保留最新的MaxLogBackups个
		// 删除多余的文件
		for i := 0; i < len(backupFiles)-MaxLogBackups; i++ {
			oldestFile := backupFiles[0]
			for _, bf := range backupFiles {
				if bf.modTime.Before(oldestFile.modTime) {
					oldestFile = bf
				}
			}

			// 删除最旧的文件
			oldPath := filepath.Join(logsDir, oldestFile.name)
			if err := os.Remove(oldPath); err != nil {
				MLog.Warn("删除旧日志文件失败", "error", err)
			} else {
				MLog.Info("已删除旧日志文件", "path", oldPath)
			}

			// 从列表中移除
			for j, bf := range backupFiles {
				if bf.name == oldestFile.name {
					backupFiles = append(backupFiles[:j], backupFiles[j+1:]...)
					break
				}
			}
		}
	}

	return nil
}

// CloseLogger 关闭日志系统
func CloseLogger() {
	if logFile != nil {
		MLog.Info("关闭日志文件")
		logFile.Close()
		logFile = nil
	}
}

// ConfigureMihomoLogger 配置 mihomo 使用的 logrus 输出
// 必须在 mihomo 初始化之后调用,以覆盖其默认配置
func ConfigureMihomoLogger() {
	if isDevMode {
		// 开发模式:logrus 已在 InitLogger 中配置
		return
	}

	// 生产模式:重新配置 logrus 输出到文件+控制台
	if loggerWriter != nil {
		logrus.SetOutput(loggerWriter)
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
		MLog.Info("mihomo 日志已配置为写入文件")
	}
}
