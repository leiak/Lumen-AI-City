# ============================================
# a2a-gateway (Go) —— 构建上下文 = ai-city/
# ============================================
# 必须整体拷 apps/ + packages/：go.work 列了 6 个模块，目录缺一个 go 命令就报错；
# 且本模块依赖 github.com/aicity/proto 与 github.com/aicity/sdk-go
# （靠 go.work + go.mod replace 解析）。

FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY go.work go.work.sum ./
COPY apps apps
COPY packages packages

# 不跑 go mod download：a2a-gateway 没有 go.sum，单模块 download 会失败；
# workspace 模式下用 go.work.sum 校验。
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/a2a-gateway ./apps/a2a-gateway/cmd \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/a2a_smoke ./apps/a2a-gateway/cmd/a2a_smoke \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/http_smoke ./apps/a2a-gateway/cmd/http_smoke \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" \
      -o /out/inbox_smoke ./apps/a2a-gateway/cmd/inbox_smoke

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/a2a-gateway /app/a2a-gateway
# E2E 冒烟二进制：原生工作流已废弃，这是唯一的验收手段
# docker compose exec a2a-gateway /app/a2a_smoke  (12 检查)
# docker compose exec a2a-gateway /app/http_smoke (8 检查)
# docker compose exec a2a-gateway /app/inbox_smoke (6 检查)
COPY --from=builder /out/a2a_smoke /app/a2a_smoke
COPY --from=builder /out/http_smoke /app/http_smoke
COPY --from=builder /out/inbox_smoke /app/inbox_smoke

EXPOSE 50061 8083
ENTRYPOINT ["/app/a2a-gateway"]
