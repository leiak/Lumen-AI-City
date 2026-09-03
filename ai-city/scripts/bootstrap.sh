#!/usr/bin/env bash
# AI City - 工具链引导
# 用法: ./scripts/bootstrap.sh

set -euo pipefail

echo "==> 安装 AI City 开发工具链"

# 检查 OS
if [[ "$OSTYPE" == "darwin"* ]]; then
  echo "检测到 macOS，使用 brew"
  HAS_BREW=$(command -v brew || true)
  if [ -z "$HAS_BREW" ]; then
    echo "请先安装 Homebrew: https://brew.sh"
    exit 1
  fi
  INSTALL="brew install"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
  echo "检测到 Linux"
  INSTALL="sudo apt-get install -y"
else
  echo "不支持的 OS: $OSTYPE"
  exit 1
fi

# Node 22 + pnpm
echo "==> Node.js 22"
if ! command -v node >/dev/null; then
  if [[ "$OSTYPE" == "darwin"* ]]; then
    brew install node@22
  else
    curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
    sudo apt-get install -y nodejs
  fi
fi

echo "==> pnpm"
if ! command -v pnpm >/dev/null; then
  npm install -g pnpm@9
fi

# Python 3.12 + uv
echo "==> Python 3.12"
if ! command -v python3.12 >/dev/null; then
  if [[ "$OSTYPE" == "darwin"* ]]; then
    brew install python@3.12
  else
    sudo apt-get install -y python3.12 python3.12-venv
  fi
fi

echo "==> uv (Python package manager)"
if ! command -v uv >/dev/null; then
  curl -LsSf https://astral.sh/uv/install.sh | sh
fi

# Rust
echo "==> Rust 1.82"
if ! command -v rustc >/dev/null; then
  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.82.0
fi

# Go
echo "==> Go 1.23"
if ! command -v go >/dev/null; then
  if [[ "$OSTYPE" == "darwin"* ]]; then
    brew install go@1.23
  else
    wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz
  fi
fi

# buf
echo "==> buf (protobuf)"
if ! command -v buf >/dev/null; then
  go install github.com/bufbuild/buf/cmd/buf@latest
fi

# Docker
echo "==> Docker"
if ! command -v docker >/dev/null; then
  if [[ "$OSTYPE" == "darwin"* ]]; then
    brew install --cask docker
  else
    curl -fsSL https://get.docker.com -o get-docker.sh
    sh get-docker.sh
  fi
fi

echo ""
echo "==> 工具链安装完成！"
echo ""
echo "下一步："
echo "  cp .env.example .env  # 配置环境变量"
echo "  make dev              # 启动所有服务"
