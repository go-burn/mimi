package main

import (
	"os"
	"os/signal"
	"syscall"
)

// SetupSignalHandler 设置信号处理器,确保优雅退出
func SetupSignalHandler(cleanup func()) {
	sigChan := make(chan os.Signal, 1)

	// 监听常见的终止信号
	signal.Notify(sigChan,
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // 系统kill
		syscall.SIGQUIT, // Ctrl+\
	)

	go func() {
		sig := <-sigChan
		MLog.Info("收到退出信号,开始清理资源", "signal", sig.String())

		// 执行清理函数
		if cleanup != nil {
			cleanup()
		}

		MLog.Info("清理完成,退出程序")
		os.Exit(0)
	}()
}
