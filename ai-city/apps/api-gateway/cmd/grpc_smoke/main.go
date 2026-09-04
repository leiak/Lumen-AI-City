// Sprint 3.5 E2E smoke：直接用 internal/worldgrpc.Client 调运行中的 world-engine。
//
// 用法：
//   world-engine.exe & （REDIS_URL 已设，gRPC 在 50051）
//   go run ./cmd/grpc_smoke  # 或 build 后 ./bin/grpc_smoke.exe
//
// 退出码 0 = 成功；非 0 = 失败（CI / 手动验收用）
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aicity/api-gateway/internal/worldgrpc"
	worldv1 "github.com/aicity/proto/gen/go"
)

func main() {
	addr := os.Getenv("WORLD_ENGINE_GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:50051"
	}

	fmt.Printf("[grpc_smoke] dial %s ...\n", addr)
	c, err := worldgrpc.NewClient(addr)
	if err != nil {
		fmt.Printf("[FAIL] dial: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1) Move → (150, 50) 落在 tile_1_0
	mv, err := c.Move(ctx, &worldv1.MoveRequest{
		EntityId: "grpc_smoke_player_001",
		Target:   &worldv1.Vec2{X: 150.0, Y: 50.0},
	})
	if err != nil {
		fmt.Printf("[FAIL] Move: %v\n", err)
		os.Exit(2)
	}
	if !mv.GetAccepted() {
		fmt.Printf("[FAIL] Move accepted=false\n")
		os.Exit(3)
	}
	fmt.Printf("[OK]   Move accepted, corrected=(%.1f,%.1f) ts=%d\n",
		mv.GetCorrectedPosition().GetX(),
		mv.GetCorrectedPosition().GetY(),
		mv.GetServerTsMs())

	// 2) GetTile → tile_1_0 应该有 grpc_smoke_player_001
	tile, err := c.GetTile(ctx, "tile_1_0")
	if err != nil {
		fmt.Printf("[FAIL] GetTile: %v\n", err)
		os.Exit(4)
	}
	if tile.GetId() != "tile_1_0" {
		fmt.Printf("[FAIL] tile id want tile_1_0 got %s\n", tile.GetId())
		os.Exit(5)
	}
	hasPlayer := false
	for _, p := range tile.GetPlayerIds() {
		if p == "grpc_smoke_player_001" {
			hasPlayer = true
			break
		}
	}
	if !hasPlayer {
		fmt.Printf("[FAIL] player not in tile_1_0 (got %v)\n", tile.GetPlayerIds())
		os.Exit(6)
	}
	fmt.Printf("[OK]   GetTile tile_1_0 size=%.0f players=%v\n", tile.GetSize(), tile.GetPlayerIds())

	// 3) ComputePath → 直线 stub
	path, err := c.ComputePath(ctx, "grpc_smoke_player_001",
		&worldv1.Vec2{X: 0, Y: 0}, &worldv1.Vec2{X: 30, Y: 40})
	if err != nil {
		fmt.Printf("[FAIL] ComputePath: %v\n", err)
		os.Exit(7)
	}
	if len(path.GetWaypoints()) != 2 {
		fmt.Printf("[FAIL] waypoints want 2 got %d\n", len(path.GetWaypoints()))
		os.Exit(8)
	}
	fmt.Printf("[OK]   ComputePath waypoints=2 distance=%.3f\n", path.GetDistanceM())

	fmt.Println("\n[OK] all 3 grpc_smoke checks passed against", addr)
}
