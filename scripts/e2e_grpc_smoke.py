"""Sprint 3 E2E gRPC smoke: Move / GetTile / ComputePath against world-engine:50051.

Requires world-engine running (REDIS_URL set, gRPC on 0.0.0.0:50051).

Generated stubs go to a temp dir, not into packages/proto/, so the source tree
stays clean.
"""
import sys
import tempfile
from pathlib import Path

import grpc
from grpc_tools import protoc

ROOT = Path(__file__).resolve().parent.parent
PROTO_DIR = ROOT / "ai-city" / "packages" / "proto"

with tempfile.TemporaryDirectory() as tmp:
    out = Path(tmp)
    protoc.main(  # type: ignore[attr-defined]
        [
            "protoc",
            f"-I{PROTO_DIR}",
            f"--python_out={out}",
            f"--grpc_python_out={out}",
            str(PROTO_DIR / "world.proto"),
        ]
    )
    sys.path.insert(0, str(out))
    import world_pb2 as pb  # noqa: E402
    import world_pb2_grpc as pb_grpc  # noqa: E402


def main() -> int:
    ch = grpc.insecure_channel("127.0.0.1:50051")
    stub = pb_grpc.WorldEngineStub(ch)

    # 1) Move → (50,50) → tile_0_0
    mv = stub.Move(
        pb.MoveRequest(
            entity_id="e2e_player_001",
            target=pb.Vec2(x=50.0, y=50.0),
            sequence=42,
            predicted=True,
            ts_ms=0,
        )
    )
    print(f"[1] Move: accepted={mv.accepted} seq={mv.sequence} "
          f"corrected=({mv.corrected_position.x:.1f},{mv.corrected_position.y:.1f}) "
          f"ts={mv.server_ts_ms}")
    assert mv.accepted and mv.sequence == 42

    # 2) GetTile → tile_0_0（应包含 e2e_player_001）
    tile = stub.GetTile(pb.GetTileRequest(tile_id="tile_0_0"))
    print(f"[2] GetTile: id={tile.id} size={tile.size} lod={tile.lod_level} "
          f"buildings={len(tile.buildings)} players={list(tile.player_ids)}")
    assert tile.id == "tile_0_0"
    assert "e2e_player_001" in tile.player_ids

    # 3) ComputePath → 直线桩
    path = stub.ComputePath(
        pb.PathRequest(
            entity_id="e2e_player_001",
            start=pb.Vec2(x=0.0, y=0.0),
            end=pb.Vec2(x=30.0, y=40.0),
        )
    )
    print(f"[3] ComputePath: waypoints={len(path.waypoints)} distance={path.distance_m:.3f}")
    assert len(path.waypoints) == 2
    assert abs(path.distance_m - 50.0) < 0.001

    # 4) GetTile 不存在 → NOT_FOUND
    try:
        stub.GetTile(pb.GetTileRequest(tile_id="tile_999_999"))
        raise AssertionError("should have raised")
    except grpc.RpcError as e:
        print(f"[4] GetTile missing: code={e.code().name} (expected NOT_FOUND)")
        assert e.code() == grpc.StatusCode.NOT_FOUND

    print("\n[OK] All 4 E2E gRPC checks passed against 127.0.0.1:50051")
    return 0


if __name__ == "__main__":
    sys.exit(main())
