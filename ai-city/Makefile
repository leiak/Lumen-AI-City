# AI City - 常用命令
# 用法: make <target>

.PHONY: help install dev test lint build clean seed proto logs stop

help:                   ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install:                ## 安装工具链
	./scripts/bootstrap.sh

dev:                    ## 启动所有服务（开发模式）
	docker compose -f docker-compose.dev.yml up -d
	./scripts/seed-data.sh
	turbo run dev --parallel

stop:                   ## 停止所有服务
	docker compose -f docker-compose.dev.yml down
	turbo run stop 2>/dev/null || true

test:                   ## 运行所有测试
	turbo run test

lint:                   ## 运行 lint
	turbo run lint

build:                  ## 构建所有包
	turbo run build

clean:                  ## 清理构建产物
	turbo run clean
	rm -rf node_modules .turbo

seed:                   ## 初始化种子数据
	./scripts/seed-data.sh

proto:                  ## 重新生成 proto 代码
	cd packages/proto && buf generate

logs:                   ## 查看所有服务日志
	docker compose -f docker-compose.dev.yml logs -f

load-test:              ## 运行压测
	./scripts/load-test.sh

chaos:                  ## 故障注入
	./scripts/chaos.sh
