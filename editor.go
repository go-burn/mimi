package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// EditorOpener 编辑器打开器
type EditorOpener struct {
}

// NewEditorOpener 创建编辑器打开器
func NewEditorOpener() (*EditorOpener, error) {
	return &EditorOpener{}, nil
}

// OpenWithEditor 使用编辑器打开文件
func (e *EditorOpener) OpenWithEditor(filePath string) error {
	if runtime.GOOS == "darwin" || os.Geteuid() == 0 {
		MLog.Info("尝试使用本机GUI编辑器打开文件", "file", filePath)
		// 尝试寻找并使用GUI编辑器
		if err := e.openWithGUIEditorAsUser(filePath); err == nil {
			return nil
		} else {
			MLog.Warn("无法打开GUI编辑器,回退到文本编辑器", "error", err)
		}
		return e.openWithTextEditor(filePath)
	}

	// 非root用户,使用系统默认应用打开(根据文件后缀)
	MLog.Info("使用系统默认方式打开文件", "file", filePath)
	if err := e.openWithSystemDefault(filePath); err == nil {
		return nil
	} else {
		MLog.Warn("系统默认方式打开失败,尝试文本编辑器", "error", err)
	}

	// 最后尝试文本编辑器
	return e.openWithTextEditor(filePath)
}

// openWithGUIEditorAsUser 以普通用户身份打开GUI编辑器(用于root环境)
func (e *EditorOpener) openWithGUIEditorAsUser(filePath string) error {
	if runtime.GOOS == "darwin" {
		appDirs := []string{"/Applications"}
		if home, err := os.UserHomeDir(); err == nil {
			appDirs = append(appDirs, filepath.Join(home, "Applications"))
		}
		for _, name := range []string{"Cursor", "Visual Studio Code", "Sublime Text", "CotEditor", "BBEdit", "Zed", "GoLand", "IntelliJ IDEA", "Atom"} {
			for _, dir := range appDirs {
				appPath := filepath.Join(dir, name+".app")
				if info, err := os.Stat(appPath); err != nil || !info.IsDir() {
					continue
				}
				MLog.Info("使用本机编辑器打开文件", "app", appPath, "file", filePath)
				if output, err := exec.Command("open", "-a", appPath, filePath).CombinedOutput(); err == nil {
					return nil
				} else {
					MLog.Warn("启动编辑器失败,尝试下一个", "app", appPath, "error", err, "output", string(output))
				}
			}
		}
		return fmt.Errorf("未找到可用的本机GUI编辑器")
	}

	guiEditors := []string{"cursor", "code", "goland", "idea", "subl", "atom"}
	var foundPath string
	for _, editor := range guiEditors {
		if path, err := exec.LookPath(editor); err == nil {
			foundPath = path
			MLog.Info("找到GUI编辑器", "editor", editor, "path", path)
			break
		}
	}

	if foundPath == "" {
		return fmt.Errorf("未找到可用的GUI编辑器")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		// Linux: 尝试以原始用户身份运行
		originalUser := os.Getenv("SUDO_USER")
		if originalUser != "" && originalUser != "root" {
			MLog.Info("以普通用户身份打开编辑器", "user", originalUser, "editor", foundPath)
			// 设置DISPLAY环境变量
			cmd = exec.Command("sudo", "-u", originalUser, "DISPLAY=:0", foundPath, filePath)
		} else {
			cmd = exec.Command(foundPath, filePath)
		}

	default:
		// Windows 或其他平台
		cmd = exec.Command(foundPath, filePath)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动编辑器失败: %w", err)
	}

	return nil
}

// openWithSystemDefault 使用系统默认应用打开文件(根据文件后缀)
func (e *EditorOpener) openWithSystemDefault(filePath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		return e.openWithGUIEditorAsUser(filePath)
	case "windows":
		// Windows: 直接使用 notepad 强制以文本方式打开,避免执行可执行文件
		cmd = exec.Command("notepad.exe", filePath)
	default: // linux
		// Linux: 使用 xdg-open,系统会根据 MIME 类型打开
		cmd = exec.Command("xdg-open", filePath)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("使用系统默认应用打开失败: %w", err)
	}
	return nil
}

// openWithTextEditor 使用文本编辑器打开文件(兜底方案)
func (e *EditorOpener) openWithTextEditor(filePath string) error {
	MLog.Info("使用文本编辑器打开文件", "file", filePath)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		// macOS: 使用 TextEdit 或 open -e
		cmd = exec.Command("open", "-e", filePath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("打开 TextEdit 失败: %w: %s", err, output)
		}
		return nil
	case "windows":
		// Windows: 使用 notepad
		cmd = exec.Command("notepad.exe", filePath)
	default: // linux
		// Linux: 尝试常见的文本编辑器
		editors := []string{"nano", "vim", "vi", "gedit", "kate"}
		for _, editor := range editors {
			if _, err := exec.LookPath(editor); err == nil {
				cmd = exec.Command(editor, filePath)
				break
			}
		}
		// 如果都没有,尝试 xdg-open
		if cmd == nil {
			cmd = exec.Command("xdg-open", filePath)
		}
	}

	if cmd == nil {
		return fmt.Errorf("未找到可用的文本编辑器")
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	return nil
}
