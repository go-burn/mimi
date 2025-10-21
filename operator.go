package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appConfig "mimi/config"

	"github.com/dop251/goja"
)

// ProcessProxyScript 代理处理脚本配置
type ProcessProxyScript struct {
	Name     string        // 脚本名称
	URL      string        // 远程脚本URL (可选,如果有则下载)
	Operator goja.Callable // 内联operator函数 (可选,如果有则直接调用)
}

// ProxyProcessService 代理处理服务
type ProxyProcessService struct {
	scripts    []ProcessProxyScript
	cacheMutex sync.RWMutex // 用于并发访问磁盘缓存时的互斥锁
	httpClient *http.Client
	cacheDir   string // 缓存目录路径

	vm *OverwriteVm
}

var proxyProcessService *ProxyProcessService

// NewProxyProcessService 创建代理处理服务
func NewProxyProcessService(vm *OverwriteVm) *ProxyProcessService {
	if proxyProcessService != nil {
		return proxyProcessService
	}
	// 获取缓存目录
	appDataDir, _ := appConfig.GetAppDataDir()

	cacheDir := filepath.Join(appDataDir, "operator_script")
	// 确保缓存目录存在
	_ = os.MkdirAll(cacheDir, 0755)

	proxyProcessService = &ProxyProcessService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cacheDir: cacheDir,
		vm:       vm,
	}
	return proxyProcessService
}

func (s *ProxyProcessService) SetConfig(exported []interface{}) {
	// 解析每个脚本配置
	s.scripts = make([]ProcessProxyScript, 0, len(exported))

	for i, cfg := range exported {
		cfgMap, ok := cfg.(map[string]interface{})
		if !ok {
			MLog.Warn("跳过无效的脚本配置", "index", i)
			continue
		}

		script := ProcessProxyScript{}

		// 解析name字段
		if name, ok := cfgMap["name"].(string); ok {
			script.Name = name
		} else {
			script.Name = fmt.Sprintf("script-%d", i)
		}

		// 解析url字段
		if urlStr, ok := cfgMap["url"].(string); ok {
			script.URL = urlStr
		}

		// 解析operator函数 - 直接从goja.Value转换为Callable
		if operatorVal := cfgMap["operator"]; operatorVal != nil && script.URL == "" {
			// operatorVal 是从 goja.Runtime.Export() 导出的,需要转换回 goja.Value
			gojaVal := s.vm.ToValue(operatorVal)
			if operatorFunc, ok := goja.AssertFunction(gojaVal); ok {
				script.Operator = operatorFunc
			} else {
				MLog.Warn("脚本配置的operator不是函数", "index", i)
				continue
			}
		}

		// 验证配置有效性
		if script.URL == "" && script.Operator == nil {
			MLog.Warn("跳过无效的脚本配置: 既没有url也没有operator", "index", i)
			continue
		}

		s.scripts = append(s.scripts, script)
	}
}

// getCacheFilePath 根据URL生成缓存文件路径
func (s *ProxyProcessService) getCacheFilePath(scriptURL string) string {
	// 使用MD5哈希URL作为文件名，避免特殊字符问题
	hash := md5.Sum([]byte(scriptURL))
	filename := hex.EncodeToString(hash[:]) + ".js"
	return filepath.Join(s.cacheDir, filename)
}

// loadCacheFromDisk 从磁盘加载缓存
func (s *ProxyProcessService) loadCacheFromDisk(scriptURL string) (string, error) {
	cachePath := s.getCacheFilePath(scriptURL)

	// 检查cachePath文件是否过期
	fileInfo, err := os.Stat(cachePath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("缓存文件不存在")
	}
	if time.Now().Sub(fileInfo.ModTime()) > 24*time.Hour {
		return "", fmt.Errorf("缓存已过期")
	}
	code, err := os.ReadFile(cachePath)
	if err != nil {
		return "", err
	}
	return string(code), nil
}

// saveCacheToDisk 保存缓存到磁盘
func (s *ProxyProcessService) saveCacheToDisk(scriptURL, code string) error {
	cachePath := s.getCacheFilePath(scriptURL)
	return os.WriteFile(cachePath, []byte(code), 0644)
}

// ParseURLParams 解析URL中的参数 (如 url#param1=value1&param2=value2)
func (s *ProxyProcessService) ParseURLParams(rawURL string) (string, map[string]interface{}, error) {
	// 分离URL和参数部分
	parts := strings.SplitN(rawURL, "#", 2)
	baseURL := parts[0]
	params := make(map[string]interface{})

	if len(parts) > 1 {
		// 解析参数
		paramStr := parts[1]
		values, err := url.ParseQuery(paramStr)
		if err != nil {
			return baseURL, params, fmt.Errorf("解析URL参数失败: %w", err)
		}

		// 转换为map[string]interface{}
		for key, vals := range values {
			if len(vals) == 1 {
				// 尝试解析为布尔值
				if vals[0] == "true" {
					params[key] = true
				} else if vals[0] == "false" {
					params[key] = false
				} else {
					params[key] = vals[0]
				}
			} else {
				params[key] = vals
			}
		}
	}

	return baseURL, params, nil
}

// FetchRemoteScript 下载远程脚本(带磁盘缓存)
func (s *ProxyProcessService) FetchRemoteScript(scriptURL string) (string, error) {
	// 解析URL,去掉参数部分用作缓存key
	baseURL, _, err := s.ParseURLParams(scriptURL)
	if err != nil {
		return "", err
	}

	// 检查磁盘缓存
	s.cacheMutex.RLock()
	cached, err := s.loadCacheFromDisk(baseURL)
	s.cacheMutex.RUnlock()
	if err == nil {
		return cached, nil
	}

	resp, err := s.httpClient.Get(baseURL)
	if err != nil {
		return "", fmt.Errorf("下载脚本失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载脚本失败: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取脚本内容失败: %w", err)
	}

	code := string(body)

	// 保存到磁盘缓存
	s.cacheMutex.Lock()
	if err = s.saveCacheToDisk(baseURL, code); err != nil {
		MLog.Warn("保存脚本缓存失败", "error", err)
	}
	s.cacheMutex.Unlock()

	return code, nil
}

// ExecuteScript 执行单个脚本
func (s *ProxyProcessService) ExecuteScript(script ProcessProxyScript, proxies []interface{}) ([]interface{}, error) {
	var operator goja.Callable

	// 情况1: 有内联operator函数 - 直接使用
	if script.Operator != nil {
		operator = script.Operator
	} else if script.URL != "" {
		// 情况2: 有URL - 下载并执行远程脚本
		// 为每个远程脚本创建独立的VM,避免变量冲突
		scriptVM := goja.New()

		// 下载脚本
		scriptCode, err := s.FetchRemoteScript(script.URL)
		if err != nil {
			return proxies, fmt.Errorf("下载脚本失败: %w", err)
		}

		// 解析URL参数
		_, urlParams, err := s.ParseURLParams(script.URL)
		if err != nil {
			MLog.Warn("解析URL参数失败", "error", err)
			urlParams = make(map[string]interface{})
		}

		// 注入参数到独立VM
		scriptVM.Set("inArg", urlParams)
		scriptVM.Set("$arguments", urlParams)

		// 执行脚本
		_, err = scriptVM.RunString(scriptCode)
		if err != nil {
			return proxies, fmt.Errorf("执行脚本失败: %w", err)
		}

		// 获取operator函数
		operatorVal := scriptVM.Get("operator")
		if operatorVal == nil || goja.IsUndefined(operatorVal) {
			return proxies, fmt.Errorf("脚本中未定义operator函数")
		}

		var ok bool
		operator, ok = goja.AssertFunction(operatorVal)
		if !ok {
			return proxies, fmt.Errorf("operator不是函数")
		}

		// 使用独立VM执行operator
		result, err := operator(goja.Undefined(), scriptVM.ToValue(proxies))
		if err != nil {
			return proxies, fmt.Errorf("调用operator失败: %w", err)
		}

		// 转换结果
		exported := result.Export()
		resultProxies, ok := exported.([]interface{})
		if !ok {
			return proxies, fmt.Errorf("operator返回值不是数组")
		}

		return resultProxies, nil
	} else {
		return proxies, fmt.Errorf("脚本配置无效: 既没有url也没有operator")
	}

	// 执行内联operator(proxies) - 使用共享VM
	result, err := operator(goja.Undefined(), s.vm.ToValue(proxies))
	if err != nil {
		return proxies, fmt.Errorf("调用operator失败: %w", err)
	}

	// 转换结果
	exported := result.Export()
	resultProxies, ok := exported.([]interface{})
	if !ok {
		return proxies, fmt.Errorf("operator返回值不是数组")
	}

	return resultProxies, nil
}

// ProcessProxies 处理所有代理节点
func (s *ProxyProcessService) ProcessProxies(proxies []interface{}) ([]interface{}, error) {
	if len(s.scripts) == 0 {
		MLog.Info("没有配置脚本")
		return proxies, nil
	}

	if len(proxies) == 0 {
		return proxies, nil
	}

	currentProxies := proxies

	// 处理每个脚本
	for _, script := range s.scripts {
		// 更新proxies变量
		s.vm.Set("proxies", currentProxies)
		result, err := s.ExecuteScript(script, currentProxies)
		if err != nil {
			// 记录错误但继续使用原来的proxies
			MLog.Warn("执行脚本失败", "script", script.Name, "error", err)
			continue
		}
		currentProxies = result
	}

	return currentProxies, nil
}

// GetScriptCount 获取脚本数量
func (s *ProxyProcessService) GetScriptCount() int {
	return len(s.scripts)
}
