FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /ws-gateway ./cmd

FROM alpine:3.19
COPY --from=builder /ws-gateway /app/ws-gateway
WORKDIR /app
EXPOSE 8082
ENTRYPOINT ["/app/ws-gateway"]
