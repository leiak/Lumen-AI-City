FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /a2a-gateway ./cmd
FROM alpine:3.19
COPY --from=builder /a2a-gateway /app/a2a-gateway
WORKDIR /app
EXPOSE 8083
ENTRYPOINT ["/app/a2a-gateway"]
