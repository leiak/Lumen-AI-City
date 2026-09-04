//! Sprint 3: 从 packages/proto/world.proto 生成 tonic gRPC 代码
//!
//! 由 `tonic-build` 在 cargo build 时跑，结果写到 OUT_DIR。
//! src/grpc.rs 用 `tonic::include_proto!("aicity.world.v1")` 引入。

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 相对路径：apps/world-engine/build.rs → ../../packages/proto
    let proto_dir = std::path::Path::new("../..").join("packages/proto");
    let world_proto = proto_dir.join("world.proto");
    println!("cargo:rerun-if-changed={}", world_proto.display());

    tonic_build::configure()
        .build_server(true)
        .build_client(true) // 单测里会用 client
        .compile_protos(&[world_proto], &[proto_dir])?;

    Ok(())
}