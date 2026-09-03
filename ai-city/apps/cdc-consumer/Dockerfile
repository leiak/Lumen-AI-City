FROM rust:1.82-alpine AS builder
RUN apk add --no-cache musl-dev cmake build-base openssl-dev
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release && rm -rf src
COPY . .
RUN cargo build --release --bin cdc-consumer
FROM alpine:3.19
COPY --from=builder /app/target/release/cdc-consumer /app/cdc-consumer
WORKDIR /app
ENTRYPOINT ["/app/cdc-consumer"]
