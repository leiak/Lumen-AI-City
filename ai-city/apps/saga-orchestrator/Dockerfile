FROM golang:1.23-alpine
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /app/saga-orchestrator ./cmd
EXPOSE 9100
CMD ["/app/saga-orchestrator"]