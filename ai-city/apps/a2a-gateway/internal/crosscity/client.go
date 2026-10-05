// ai-city/apps/a2a-gateway/internal/crosscity/client.go
//
// Sprint 12 (2.0 stage 1) — Task 50: 跨城 gRPC client（mTLS 拨号）。
//
// 联邦投递（Task 52+）：a2a-gateway 收到别城 NPC 的 inbox 消息后，从
// `internal/router.GlobalTable` 解析目标 NPC 所在城市的 world-engine gRPC endpoint，
// 再用本包的 `Dial()` 建立 mTLS 连接、调用 `Move / SubscribePosition` 等跨城 RPC。
//
// mTLS 要求：
//   - 客户端出示由 aicity-CA 签发的 cert
//   - 客户端校验服务端 cert（同样由 aicity-CA 签发）
//   - 所有 3 个 PEM 文件由 compose 挂到 /certs，路径通过 env 注入

package crosscity

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Dial 创建带 mTLS 的 gRPC 连接。
//
// 环境变量（必须全部设置；缺一返错，fail-fast）：
//   - GRPC_TLS_CERT —— 客户端 cert（PEM）
//   - GRPC_TLS_KEY  —— 客户端 private key（PEM）
//   - GRPC_TLS_CA   —— CA bundle（PEM），用于校验 server cert
//
// 调用方负责在不用时 `conn.Close()`。
func Dial(addr string) (*grpc.ClientConn, error) {
	certFile := os.Getenv("GRPC_TLS_CERT")
	keyFile := os.Getenv("GRPC_TLS_KEY")
	caFile := os.Getenv("GRPC_TLS_CA")
	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("crosscity.Dial: GRPC_TLS_CERT/KEY/CA env vars required for mTLS")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("crosscity.Dial: load client cert/key: %w", err)
	}
	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("crosscity.Dial: read CA bundle %s: %w", caFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("crosscity.Dial: failed to append CA certs from %s", caFile)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		MinVersion:   tls.VersionTLS12,
	}

	// 跨城 RPC 期望高延迟（公网/内网跨域），用阻塞 dial 让 caller 用 ctx 控制超时；
	// grpc.Dial 默认 non-blocking + 懒连接，首个 RPC 才建链 —— 调用方通常会立即
	// 用 ctx.WithTimeout 包一次。
	return grpc.Dial(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
}
