module github.com/aicity/a2a-gateway

go 1.23

require (
	github.com/aicity/sdk-go v0.0.0
	github.com/gin-gonic/gin v1.10.0
	github.com/jackc/pgx/v5 v5.6.0
	google.golang.org/grpc v1.66.0
)

require (
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/redis/go-redis/v9 v9.6.1 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.27.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240604185151-ef581f913117 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

// Sprint 7+：本地 sdk-go（packages/sdk-go）通过 go.work 已自动解析，
// 但 v0.0.0 版本号导致部分工具链（如 go test）尝试从远程 repo fetch go.mod。
// 加 replace 强制指向本地路径，消除网络依赖。
replace github.com/aicity/sdk-go => ../../packages/sdk-go
