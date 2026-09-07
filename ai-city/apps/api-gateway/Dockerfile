# ============================================
# api-gateway (Go) —— 构建上下文 = ai-city/
# ============================================
# 必须整体拷 apps/ + packages/：go.work 列了 6 个模块，目录缺一个 go 命令就报错；
# 且 internal/worldgrpc/client.go import github.com/aicity/proto/gen/go
# （无 require/replace，只能靠 workspace 解析）。

FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY go.work go.work.sum ./
COPY apps apps
COPY packages packages

# 不跑 go mod download：workspace 模式下用 go.work.sum 校验；单模块 download
# 会因 a2a-gateway 缺 go.sum 而失败。
# ./cmd 与 ./cmd/<sub> 是独立 package，多个 main 不冲突。
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/api-gateway ./apps/api-gateway/cmd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/grpc_smoke ./apps/api-gateway/cmd/grpc_smoke

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/api-gateway /app/api-gateway
# E2E 冒烟二进制：原生工作流已废弃，这是唯一的验收手段
# docker compose exec api-gateway /app/grpc_smoke
COPY --from=builder /out/grpc_smoke /app/grpc_smoke

EXPOSE 8080
ENTRYPOINT ["/app/api-gateway"]
