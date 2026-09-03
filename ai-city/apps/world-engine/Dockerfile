FROM rust:1.82-alpine AS builder
RUN apk add --no-cache musl-dev cmake build-base openssl-dev
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release && rm -rf src
COPY . .
RUN cargo build --release --bin world-engine

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/target/release/world-engine /app/world-engine
WORKDIR /app
EXPOSE 50051
ENTRYPOINT ["/app/world-engine"]
