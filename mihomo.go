package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub"
	"github.com/metacubex/mihomo/hub/executor"
)

const ConfigJS = "config.js"

var mcfg *config.Config

// InitMihomo 初始化 mihomo 配置目录
// 必须在应用目录初始化之后调用
func InitMihomo(homeDir string) error {
	// 设置 mihomo 的工作目录
	constant.SetHomeDir(homeDir)
	constant.SetConfig(constant.Path.Config())

	if err := os.Chdir(homeDir); err != nil {
		return fmt.Errorf("切换到 mihomo 工作目录失败: %w", err)
	}

	// 初始化 mihomo 配置
	if err := config.Init(homeDir); err != nil {
		return fmt.Errorf("初始化 mihomo 配置目录失败: %w", err)
	}

	MLog.Info("Mihomo 配置目录", "path", constant.Path.HomeDir())
	MLog.Info("Mihomo 配置文件路径", "path", constant.Path.Resolve(constant.Path.Config()))
	return nil
}

func Parse(configBytes []byte, options ...hub.Option) (*config.Config, error) {
	var cfg *config.Config
	var err error

	if len(configBytes) != 0 {
		cfg, err = executor.ParseWithBytes(configBytes)
	} else {
		// 读取配置文件路径
		configPath := constant.Path.Resolve(constant.Path.Config())
		MLog.Info("读取 Mihomo 配置文件", "path", configPath)

		// 读取配置文件内容
		configBytes, err = os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}

		// 使用 ParseWithBytes 解析配置
		cfg, err = executor.ParseWithBytes(configBytes)
	}

	if err != nil {
		return nil, err
	}

	for _, option := range options {
		option(cfg)
	}

	hub.ApplyConfig(cfg)
	return cfg, nil
}

func apply() {
	cfg, err := Parse([]byte{})
	if cfg == nil || err != nil {
		MLog.Error("Parse configuration error", "error", err)
		return
	}
	mcfg = cfg
	setWindowHost(mcfg.Controller.ExternalController)

	// 如果系统代理已启用,则更新代理配置(端口可能变化)
	isProxyEnabled := systemProxyService.StateProxy()
	MLog.Info("apply 检查系统代理状态", "已启用", isProxyEnabled)
	if isProxyEnabled {
		byPass, _ := OVM.ByPass()
		if err = systemProxyService.EnableProxy(fmt.Sprintf("127.0.0.1:%d", mcfg.General.MixedPort), byPass...); err != nil {
			MLog.Warn("更新系统代理配置失败", "error", err)
		} else {
			MLog.Info("系统代理配置已更新", "端口", mcfg.General.MixedPort)
		}
	}
}

func shutdown() {
	executor.Shutdown()
}

// WriteConfigYAML 将配置写入 config.yaml 文件
func WriteConfigYAML(config map[string]interface{}) error {
	// 获取完整的配置文件路径
	configPath := constant.Path.Resolve(constant.Path.Config())

	// 创建文件
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	// 使用 yaml.Encoder
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf, yaml.Indent(2), yaml.IndentSequence(true))

	if err = encoder.Encode(config); err != nil {
		return fmt.Errorf("编码配置失败: %w", err)
	}

	if err = encoder.Close(); err != nil {
		return fmt.Errorf("关闭编码器失败: %w", err)
	}

	// 写入文件
	if _, err = file.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// isValidConfig 检查配置是否有效(至少包含必需字段)
func isValidConfig(config map[string]interface{}) bool {
	// 检查是否为空或只有空字段
	if len(config) == 0 {
		return false
	}

	// 检查必需的基础字段
	requiredFields := []string{"mixed-port", "mode", "external-controller"}
	for _, field := range requiredFields {
		if _, exists := config[field]; !exists {
			return false
		}
	}

	return true
}

// 读取配置、执行 JavaScript 处理、写入结果
func ProcessOverwrite() error {
	// 1.解析config
	vm, err := NewOverwriteVm()
	if err != nil {
		return fmt.Errorf("解析 config.js 失败: %w", err)
	}

	// 2. 获取默认配置作为基础
	configData := make(map[string]interface{})

	// 3. 调用main 函数进行覆写
	processedConfig, err := vm.Main(configData)
	if err != nil {
		MLog.Warn("执行 config.js main 函数失败,使用默认配置", "error", err)
		processedConfig = configData
	}

	// 5. 写入处理后的配置
	if err := WriteConfigYAML(processedConfig); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 6. 调用Proxies函数
	proxiesConfig, err := vm.Proxies()
	if err != nil {
		MLog.Warn("执行 transformProxiesConfig 函数失败", "error", err)
	} else {
		n := NewProxyProcessService(vm)
		n.SetConfig(proxiesConfig)
	}

	MLog.Info("配置处理完成", "path", constant.Path.Resolve(constant.Path.Config()))
	return nil
}
