package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// IsRunningAsRoot 检查当前程序是否以 root 权限运行
func IsRunningAsRoot() bool {
	if runtime.GOOS == "windows" {
		// Windows 系统检测管理员权限
		cmd := exec.Command("net", "session")
		err := cmd.Run()
		return err == nil
	}
	// Unix-like 系统检测 root 权限
	return os.Geteuid() == 0
}

// RequestAdminPrivilege 请求管理员权限并重启应用
// enableTun: 是否在获得权限后立即启用 TUN 模式
func RequestAdminPrivilege(enableTun bool) error {
	if IsRunningAsRoot() {
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		// macOS 使用 osascript 请求权限
		MLog.Info("请求管理员权限以启动 TUN 模式", "execPath", execPath)

		// 获取当前用户信息
		currentUID := os.Getuid()

		// 构建启动命令：通过环境变量传递 TUN 启用意图
		// 使用 env 命令设置环境变量，避免文件系统 I/O
		launchCmd := fmt.Sprintf("'%s'", execPath)
		if enableTun {
			launchCmd = fmt.Sprintf("env MIMI_ENABLE_TUN=1 '%s'", execPath)
			MLog.Info("将通过环境变量 MIMI_ENABLE_TUN=1 启动新进程")
		}

		// 关键组合：
		// 1. osascript "with administrator privileges" 让命令以 root 执行
		// 2. launchctl asuser 让进程运行在用户会话中（GUI 可访问）
		// 3. env 命令设置环境变量传递启动意图
		// 结果：root 权限 + GUI 可见 + 无文件残留
		script := fmt.Sprintf(
			`do shell script "launchctl asuser %d %s &" with administrator privileges`,
			currentUID, launchCmd,
		)
		cmd := exec.Command("osascript", "-e", script)

		// 异步执行权限提升命令，不等待完成
		// 这样可以立即退出用户进程，避免阻塞
		if err := cmd.Start(); err != nil {
			MLog.Error("启动权限提升命令失败", "error", err)
			return fmt.Errorf("启动权限提升命令失败: %w", err)
		}

		GracefulExit()

	case "linux":
		// Linux 使用 pkexec 或 sudo
		if _, err := exec.LookPath("pkexec"); err == nil {
			// 使用 env 命令设置环境变量
			var cmd *exec.Cmd
			if enableTun {
				cmd = exec.Command("pkexec", "env", "MIMI_ENABLE_TUN=1", execPath)
				MLog.Info("将通过环境变量 MIMI_ENABLE_TUN=1 启动新进程")
			} else {
				cmd = exec.Command("pkexec", execPath)
			}
			err := cmd.Start()
			if err != nil {
				return fmt.Errorf("使用 pkexec 启动失败: %w", err)
			}
			GracefulExit()
		} else if _, err := exec.LookPath("sudo"); err == nil {
			// 回退到 sudo
			return fmt.Errorf("需要管理员权限,请使用: sudo %s", execPath)
		}
		return fmt.Errorf("未找到权限提升工具(pkexec/sudo)")

	case "windows":
		// Windows 使用 runas
		// 注意: 在 Windows 上需要使用 UAC 提示
		return fmt.Errorf("Windows TUN 模式需要以管理员身份运行应用")

	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	return nil
}

// CheckTunPrivilege 检查 TUN 模式所需权限
func CheckTunPrivilege() error {
	if !IsRunningAsRoot() {
		return fmt.Errorf("TUN 模式需要管理员权限")
	}
	return nil
}

// EnsureTunPrivilege 确保具有 TUN 模式所需的权限
func EnsureTunPrivilege() error {
	if err := CheckTunPrivilege(); err != nil {
		MLog.Warn("需要管理员权限", "error", err)

		// 如果是自动模式,则尝试请求权限
		if err := RequestAdminPrivilege(false); err != nil {
			return fmt.Errorf("无法获取管理员权限: %w", err)
		}
	}
	return nil
}

// GetPrivilegeStatus 获取当前权限状态描述
func GetPrivilegeStatus() string {
	if IsRunningAsRoot() {
		return "✓ 已具有管理员权限"
	}

	var methods []string
	switch runtime.GOOS {
	case "darwin":
		methods = append(methods, "将提示输入管理员密码")
	case "linux":
		if _, err := exec.LookPath("pkexec"); err == nil {
			methods = append(methods, "将使用 pkexec 请求权限")
		}
		if _, err := exec.LookPath("sudo"); err == nil {
			methods = append(methods, fmt.Sprintf("或使用命令: sudo %s", os.Args[0]))
		}
	case "windows":
		methods = append(methods, "请右键以管理员身份运行")
	}

	if len(methods) > 0 {
		return "✗ 需要管理员权限 - " + strings.Join(methods, ", ")
	}
	return "✗ 需要管理员权限"
}

// GracefulExit 优雅退出程序，确保所有资源被正确清理
// 与 os.Exit(0) 不同，此函数会：
// 1. 执行所有注册的清理函数
// 2. 关闭所有网络监听器（释放端口）
// 3. 清理 TUN 设备
// 4. 恢复系统代理设置
// 5. 刷新并关闭日志文件
func GracefulExit() {
	// 1. 关闭 mihomo 核心（释放所有端口和网络资源）
	MLog.Info("正在关闭 mihomo 核心...")
	shutdown()

	// 3. 关闭日志系统（刷新缓冲区）
	MLog.Info("正在关闭日志系统...")
	CloseLogger()

	// 4. 最后退出（此时所有资源已清理）
	os.Exit(0)
}
