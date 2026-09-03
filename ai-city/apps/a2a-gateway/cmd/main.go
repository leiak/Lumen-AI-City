// Package main A2A Gateway 入口
// 详细设计见 docs/06-A2A协议.md 全文
package main

import (
	"log"
	"os"
)

func main() {
	port := os.Getenv("A2A_GATEWAY_PORT")
	if port == "" {
		port = "8083"
	}
	log.Printf("a2a-gateway starting on :%s", port)
	// TODO: 实现 Agent Card 服务 + Envelope 路由 + 适配器
	select {}
}
