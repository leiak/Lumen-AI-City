FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /obs-agent ./cmd
FROM alpine:3.19
COPY --from=builder /obs-agent /app/obs-agent
WORKDIR /app
EXPOSE 9091
ENTRYPOINT ["/app/obs-agent"]
