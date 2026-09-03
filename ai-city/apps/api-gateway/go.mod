module github.com/aicity/api-gateway

go 1.23

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/jackc/pgx/v5 v5.6.0
	github.com/prometheus/client_golang v1.20.0
	github.com/redis/go-redis/v9 v9.6.1
	github.com/sony/gobreaker v1.0.0
	go.opentelemetry.io/otel v1.30.0
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.27.0
)

require github.com/google/uuid v1.6.0 // indirect
