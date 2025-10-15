//go:build production

package env

// IsProduction 返回是否为生产环境
func IsProduction() bool {
	return true
}
