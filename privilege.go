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

	// 构建环境变量
	var envVars []string
	if enableTun {
		envVars = append(envVars, "MIMI_ENABLE_TUN=1")
		MLog.Info("将通过环境变量 MIMI_ENABLE_TUN=1 启动新进程")
	}

	MLog.Info("请求管理员权限重启应用")

	// 使用统一的重启函数，以管理员权限启动
	if err := RestartApplication(true, envVars...); err != nil {
		return err
	}

	// 优雅退出当前进程
	GracefulExit()

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

// RestartApplication 重启应用程序
// asAdmin: 是否以管理员权限重启
// envVars: 传递给新进程的环境变量 (格式: "KEY=VALUE")
func RestartApplication(asAdmin bool, envVars ...string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		wd = "" // 如果获取失败，使用空字符串，命令会使用默认目录
	}

	switch runtime.GOOS {
	case "darwin":
		// macOS 特殊处理
		if asAdmin {
			// 以管理员权限重启
			return restartMacOSWithAdmin(execPath, wd, envVars...)
		}
		// 普通重启
		return restartMacOS(execPath, wd, envVars...)

	case "linux":
		if asAdmin {
			// 以管理员权限重启
			return restartLinuxWithAdmin(execPath, envVars...)
		}
		// 普通重启
		return restartLinux(execPath, wd, envVars...)

	case "windows":
		if asAdmin {
			return fmt.Errorf("Windows 需要手动以管理员身份运行")
		}
		// 普通重启
		return restartWindows(execPath, wd, envVars...)

	default:
		return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// restartMacOS macOS 普通重启
func restartMacOS(execPath, workDir string, envVars ...string) error {
	// 如果是 .app 包，使用 open 命令
	if strings.Contains(execPath, ".app/Contents/MacOS/") {
		appPath := execPath[:strings.Index(execPath, ".app/")+4]

		// 使用 shell 脚本延迟启动，避免端口冲突
		// sleep 1 秒后启动新进程，给旧进程足够时间释放端口
		script := fmt.Sprintf("sleep 1 && open '%s'", appPath)

		if len(envVars) > 0 {
			// open 命令不支持直接设置环境变量，需要通过其他方式
			MLog.Warn("macOS .app 重启不支持环境变量传递", "envVars", envVars)
		}

		cmd := exec.Command("sh", "-c", script)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("启动 .app 失败: %w", err)
		}

		MLog.Info("已调度延迟启动 .app (1秒后)")
		return nil
	}

	// 普通可执行文件 - 也使用延迟启动
	envPrefix := ""
	if len(envVars) > 0 {
		envPrefix = "env " + strings.Join(envVars, " ") + " "
	}

	script := fmt.Sprintf("sleep 1 && %s'%s' %s", envPrefix, execPath, strings.Join(os.Args[1:], " "))
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = workDir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新进程失败: %w", err)
	}

	MLog.Info("已调度延迟启动新进程 (1秒后)")
	return nil
}

// restartMacOSWithAdmin macOS 以管理员权限重启
func restartMacOSWithAdmin(execPath, workDir string, envVars ...string) error {
	currentUID := os.Getuid()

	// 构建启动命令，添加 sleep 避免端口冲突
	launchCmd := fmt.Sprintf("'%s'", execPath)
	if len(envVars) > 0 {
		envPrefix := "env " + strings.Join(envVars, " ")
		launchCmd = fmt.Sprintf("%s '%s'", envPrefix, execPath)
		MLog.Info("将通过环境变量启动新进程", "envVars", envVars)
	}

	// 使用 osascript + launchctl 保持 GUI 会话
	// 添加 sleep 1，让旧进程先退出释放端口
	script := fmt.Sprintf(
		`do shell script "sleep 1 && launchctl asuser %d %s &" with administrator privileges`,
		currentUID, launchCmd,
	)
	cmd := exec.Command("osascript", "-e", script)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动权限提升命令失败: %w", err)
	}

	MLog.Info("已请求管理员权限重启 (1秒后启动)")
	return nil
}

// restartLinux Linux 普通重启
func restartLinux(execPath, workDir string, envVars ...string) error {
	// 使用 shell 脚本延迟启动，避免端口冲突
	envPrefix := ""
	if len(envVars) > 0 {
		envPrefix = strings.Join(envVars, " ") + " "
	}

	args := strings.Join(os.Args[1:], " ")
	script := fmt.Sprintf("sleep 1 && %s'%s' %s", envPrefix, execPath, args)

	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = workDir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新进程失败: %w", err)
	}

	MLog.Info("已调度延迟启动新进程 (1秒后)")
	return nil
}

// restartLinuxWithAdmin Linux 以管理员权限重启
func restartLinuxWithAdmin(execPath string, envVars ...string) error {
	if _, err := exec.LookPath("pkexec"); err == nil {
		// 使用 shell 脚本延迟启动，避免端口冲突
		args := []string{"sh", "-c"}

		envPrefix := ""
		if len(envVars) > 0 {
			envPrefix = strings.Join(envVars, " ") + " "
		}

		cmdArgs := strings.Join(os.Args[1:], " ")
		script := fmt.Sprintf("sleep 1 && %s'%s' %s", envPrefix, execPath, cmdArgs)
		args = append(args, script)

		cmd := exec.Command("pkexec", args...)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("使用 pkexec 启动失败: %w", err)
		}

		MLog.Info("已使用 pkexec 请求权限重启 (1秒后启动)")
		return nil
	}

	if _, err := exec.LookPath("sudo"); err == nil {
		return fmt.Errorf("需要管理员权限，请使用: sudo %s", execPath)
	}

	return fmt.Errorf("未找到权限提升工具(pkexec/sudo)")
}

// restartWindows Windows 普通重启
func restartWindows(execPath, workDir string, envVars ...string) error {
	// Windows 使用 timeout 命令延迟启动，避免端口冲突
	// timeout /t 1 /nobreak > nul 等待1秒
	args := strings.Join(os.Args[1:], " ")
	script := fmt.Sprintf("timeout /t 1 /nobreak > nul && \"%s\" %s", execPath, args)

	cmd := exec.Command("cmd", "/C", script)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), envVars...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动新进程失败: %w", err)
	}

	MLog.Info("已调度延迟启动新进程 (1秒后)")
	return nil
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
