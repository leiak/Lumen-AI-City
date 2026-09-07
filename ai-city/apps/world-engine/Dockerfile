# ============================================
# world-engine (Rust) —— 构建上下文 = ai-city/
# ============================================
# 布局刻意还原仓库结构：crate 在 /src/apps/world-engine，proto 在
# /src/packages/proto，这样 build.rs:8 的相对路径 ../../packages/proto 成立。

FROM rust:1.82-alpine AS builder

# protobuf-dev 提供 /usr/bin/protoc —— Cargo.lock 里 prost-build 0.13.5
# 不含 vendored protoc。
# 不装 openssl-dev：Cargo.lock 无 ring / openssl-sys / rustls，无 C TLS 依赖，
# musl 全静态安全。
RUN apk add --no-cache musl-dev build-base protobuf-dev
ENV PROTOC=/usr/bin/protoc

WORKDIR /src

# world.proto 零 import，只拷这一个文件即可
COPY packages/proto/world.proto packages/proto/world.proto
COPY apps/world-engine apps/world-engine

WORKDIR /src/apps/world-engine

# 不做 dummy-src 预热层：Cargo.toml 同时声明 [lib] src/lib.rs 与
# [[bin]] src/main.rs，只造 main.rs 的假 src 会因缺 lib.rs 报错。
# 改用 BuildKit cache mount 缓存 cargo registry 与 target/；因 target/ 是
# cache mount（构建结束即卸载），必须在同一个 RUN 里 cp 出二进制。
# --locked 让 Cargo.lock 过期变成硬报错而非静默重解析。
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/src/apps/world-engine/target \
    cargo build --release --locked --bin world-engine \
 && cp target/release/world-engine /world-engine

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /world-engine /app/world-engine
EXPOSE 50051 50052
ENTRYPOINT ["/app/world-engine"]
