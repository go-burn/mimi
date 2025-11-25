package main

import (
	"fmt"
	appConfig "mimi/config"
	"os"
	"path/filepath"
	"sort"

	"github.com/dop251/goja"
)

type OverwriteVm struct {
	*goja.Runtime
}

var OVM *OverwriteVm

func NewOverwriteVm() (*OverwriteVm, error) {
	// 使用应用数据目录下的 config.js
	appDataDir, _ := appConfig.GetAppDataDir()
	overwriteJsPath := filepath.Join(appDataDir, ConfigJS)

	// 不存在就创建overwrite.js
	if _, err := os.Stat(overwriteJsPath); os.IsNotExist(err) {
		defaultOverwriteJS := `// MIMI 配置覆写文件
// 此文件用于自定义 mihomo 配置,会在默认配置基础上进行覆写

// 订阅节点配置
// 注意: 可以通过菜单选择使用哪个订阅或全部订阅
const subscriptions = {
    // "sub1": "https://your-subscription-url-1",
    // "sub2": "https://your-subscription-url-2",
};

/**
 * main 函数: 生成或覆写 mihomo 配置
 * @param {Object} params - 默认配置对象
 * @returns {Object} 最终的 mihomo 配置
 *
 * 全局变量 selectedSubscription:
 * - 从菜单选择的订阅名称
 * - 空字符串表示"全部订阅"
 * - 非空字符串表示选中的具体订阅名称
 */
function main(params) {
    // 默认情况下,使用传入的默认配置
    // 你可以修改 params 中的任何字段来自定义配置

    // 处理订阅
    processProxyProviders(params);

    // 示例: 修改代理端口
    // params["mixed-port"] = 7891;

    // 示例: 启用局域网访问
    // params["allow-lan"] = true;

    // 示例: 添加代理组
    // params["proxy-groups"] = [
    //     {
    //         "name": "🚀 节点选择",
    //         "type": "select",
    //         "proxies": ["DIRECT"]
    //     }
    // ];

    // 示例: 添加规则
    // params["rules"] = [
    //     "DOMAIN-SUFFIX,google.com,🚀 节点选择",
    //     "GEOIP,CN,DIRECT",
    //     "MATCH,🚀 节点选择"
    // ];

    return params;
}

/**
 * processProxyProviders 函数: 处理订阅配置
 * 根据 selectedSubscription 变量筛选需要加载的订阅
 */
function processProxyProviders(params) {
    let providers = {};

    // selectedSubscription 是从 Go 代码注入的全局变量
    // 空字符串表示"全部订阅",否则为具体订阅名称
    const selected = typeof selectedSubscription !== 'undefined' ? selectedSubscription : '';

    for (let key in subscriptions) {
        // 如果选中了特定订阅,只处理该订阅
        if (selected !== '' && key !== selected) {
            continue;
        }

        providers[key] = {
            "type": "http",
            "url": subscriptions[key],
            "interval": 86400, // 24小时更新一次
            "health-check": {
                "enable": true,
                "interval": 300,
                "url": "https://www.google.com/generate_204"
            },
            "override": {
                "additional-prefix": "[" + key + "] ",
            },
        };
    }

    params["proxy-providers"] = providers;
}

/**
 * transformProxiesConfig 函数: 处理代理节点配置
 * @returns {Array} 代理节点数组
 */
function transformProxiesConfig() {
    return [];
}

/**
 * transformBypassConfig 函数: 配置系统代理绕过列表
 * @returns {Array<string>} 绕过域名/IP列表
 */
function transformBypassConfig() {
    return [
        "localhost",
        "127.*",
        "10.*",
        "172.16.*",
        "172.17.*",
        "172.18.*",
        "172.19.*",
        "172.20.*",
        "172.21.*",
        "172.22.*",
        "172.23.*",
        "172.24.*",
        "172.25.*",
        "172.26.*",
        "172.27.*",
        "172.28.*",
        "172.29.*",
        "172.30.*",
        "172.31.*",
        "192.168.*"
    ];
}
`
		if err := os.WriteFile(overwriteJsPath, []byte(defaultOverwriteJS), 0644); err != nil {
			return nil, fmt.Errorf("创建覆写文件失败: %w", err)
		}
	}

	// 读取 JavaScript 文件
	jsContent, err := os.ReadFile(overwriteJsPath)
	if err != nil {
		return nil, fmt.Errorf("读取覆写文件失败: %w", err)
	}

	// 创建 goja 运行时环境
	vm := goja.New()
	// 注册 console.log
	vm.Set("console", map[string]interface{}{
		"log": func(args ...interface{}) {
			MLog.Info("JS console.log", "args", args)
		},
	})

	// 执行 JS 文件内容
	_, err = vm.RunString(string(jsContent))
	if err != nil {
		return nil, fmt.Errorf("执行 config.js 失败: %w", err)
	}
	OVM = &OverwriteVm{vm}
	return OVM, nil
}

func (vm *OverwriteVm) Main(params map[string]interface{}) (map[string]interface{}, error) {
	// 获取 main 函数
	mainFunc, ok := goja.AssertFunction(vm.Get("main"))
	if !ok {
		return nil, fmt.Errorf("未找到 main 函数")
	}

	// 注入选中的订阅信息到 JavaScript 环境
	vm.Set("selectedSubscription", selectedSubscription)

	// 调用 main 函数并传入参数
	result, err := mainFunc(goja.Undefined(), vm.ToValue(params))
	if err != nil {
		return nil, fmt.Errorf("调用 main 函数失败: %w", err)
	}

	// 将 JavaScript 返回值转换为 Go map
	resultMap := result.Export()
	if resultMap == nil {
		return nil, fmt.Errorf("JavaScript 返回值为 null 或 undefined")
	}

	// 类型断言确保返回值是 map
	finalMap, ok := resultMap.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("JavaScript 返回值不是对象类型")
	}

	return finalMap, nil
}

func (vm *OverwriteVm) Proxies() ([]interface{}, error) {
	// 获取 main 函数
	f, ok := goja.AssertFunction(vm.Get("transformProxiesConfig"))
	if !ok {
		return nil, fmt.Errorf("未找到 transformProxiesConfig 函数")
	}

	// 调用 main 函数并传入参数
	result, err := f(goja.Undefined())
	if err != nil {
		return nil, fmt.Errorf("调用 transformProxiesConfig 函数失败: %w", err)
	}

	// 将 JavaScript 返回值转换为 Go array
	resultMap := result.Export()
	if resultMap == nil {
		return nil, fmt.Errorf("JavaScript 返回值为 null 或 undefined")
	}

	// 类型断言确保返回值是 array
	finalMap, ok := resultMap.([]interface{})
	if !ok {
		return nil, fmt.Errorf("JavaScript 返回值不是对象类型")
	}

	return finalMap, nil
}

func (vm *OverwriteVm) ByPass() ([]string, error) {
	// 获取 main 函数
	f, ok := goja.AssertFunction(vm.Get("transformBypassConfig"))
	if !ok {
		return nil, fmt.Errorf("未找到 transformBypassConfig 函数")
	}

	// 调用 main 函数并传入参数
	result, err := f(goja.Undefined())
	if err != nil {
		return nil, fmt.Errorf("调用 transformBypassConfig 函数失败: %w", err)
	}

	// 将 JavaScript 返回值转换为 Go array
	resultMap := result.Export()
	if resultMap == nil {
		return nil, fmt.Errorf("JavaScript 返回值为 null 或 undefined")
	}

	// 类型断言确保返回值是 array
	final, ok := resultMap.([]interface{})
	if !ok {
		return nil, fmt.Errorf("JavaScript 返回值不是数组类型")
	}
	bypassList := make([]string, len(final))
	for i, v := range final {
		bypassList[i], _ = v.(string)
	}

	return bypassList, nil
}

// Subscriptions 获取订阅列表
func (vm *OverwriteVm) Subscriptions() ([]string, error) {
	// 获取 subscriptions 对象
	subscriptionsValue := vm.Get("subscriptions")
	if subscriptionsValue == nil || goja.IsUndefined(subscriptionsValue) || goja.IsNull(subscriptionsValue) {
		return []string{}, nil // 返回空 map,不报错
	}

	// 将 JavaScript 对象转换为 Go map
	subscriptionsExport := subscriptionsValue.Export()
	if subscriptionsExport == nil {
		return []string{}, nil
	}

	// 类型断言为 map[string]interface{}
	subscriptionsMap, ok := subscriptionsExport.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("subscriptions 不是对象类型")
	}

	// 转换为 []string 并排序
	var result []string
	for key := range subscriptionsMap {
		result = append(result, key)
	}

	// 排序确保顺序稳定
	sort.Strings(result)

	return result, nil
}
