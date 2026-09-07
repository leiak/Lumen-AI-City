# ============================================
# ws-gateway (Go) —— 构建上下文 = ai-city/
# ============================================
# 与 api-gateway/Dockerfile 同构：必须整体拷 apps/ + packages/，
# go.work 列了 6 个模块，目录缺一个 go 命令就报错。

FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY go.work go.work.sum ./
COPY apps apps
COPY packages packages

# 不跑 go mod download：workspace 模式下用 go.work.sum 校验；单模块 download
# 会因 a2a-gateway 缺 go.sum 而失败。
# ./cmd 与 ./cmd/ws_smoke 是独立 package，两个 main 不冲突。
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/ws-gateway ./apps/ws-gateway/cmd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/ws_smoke ./apps/ws-gateway/cmd/ws_smoke

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/ws-gateway /app/ws-gateway
# E2E 冒烟二进制：docker compose exec ws-gateway /app/ws_smoke
COPY --from=builder /out/ws_smoke /app/ws_smoke

EXPOSE 8082
ENTRYPOINT ["/app/ws-gateway"]
