// ai-city/apps/a2a-gateway/internal/router/router.go
//
// 跨城 NPC 路由：包级 GlobalTable 单例 + 加载入口 LoadRoutes。
//
// 注意：本包与 internal/httpgw/router.go（gin HTTP 路由）同名但语义不同：
//   - httpgw/router.go：HTTP method/path → handler 映射；
//   - router/router.go：NPC ID → 城市 endpoint 映射（联邦投递目标解析）。
package router

// GlobalTable 包级单例：启动期 LoadRoutes 一次性装载，HTTP/gRPC handlers
// 通过 Lookup / All 读取。后续 Task 引入 Reload() 时仍共用此变量。
var GlobalTable = NewTable()

// LoadRoutes 从 yaml 文件加载路由到 GlobalTable。
// 入口处 fail-fast：加载失败应直接 log.Fatal，路由表为空 = a2a 联邦瘫痪。
func LoadRoutes(path string) error {
	return GlobalTable.LoadFromYAML(path)
}
