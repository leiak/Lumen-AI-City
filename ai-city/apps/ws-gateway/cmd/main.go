// Package main WebSocket Gateway
// 职责：长连接管理 / 心跳 / 消息分发
// 详细设计见 docs/04-API设计.md §18.3 + docs/11-技术细节与玩法模式.md §E.6
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nhooyr.io/websocket"
)

func main() {
	addr := ":8082"
	if v := os.Getenv("WS_GATEWAY_PORT"); v != "" {
		addr = ":" + v
	}

	http.HandleFunc("/ws", handleWS)

	srv := &http.Server{Addr: addr, Handler: nil}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Printf("ws-gateway starting on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 生产环境应校验 Origin
	})
	if err != nil {
		log.Printf("ws accept failed: %v", err)
		return
	}
	defer c.Close(websocket.StatusInternalError, "internal error")

	// TODO: 连接管理 / 心跳 / 鉴权 / 消息分发
	for {
		_, _, err := c.Read(context.Background())
		if err != nil {
			log.Printf("ws read failed: %v", err)
			return
		}
	}
}
