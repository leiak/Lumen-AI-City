# AI City - 常用命令
# 用法: make <target>

.PHONY: help install dev test lint build clean seed proto logs stop ps reset

help:                   ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

install:                ## 安装工具链
	./scripts/bootstrap.sh

dev:                    ## 启动 aitown 全栈（6 个容器）
	docker compose up -d --build

stop:                   ## 停止所有服务
	docker compose down

ps:                     ## 查看容器状态
	docker compose ps

reset:                  ## 重建（含删卷 → 重跑 initdb 种子）
	docker compose down -v
	docker compose up -d --build

test:                   ## 运行所有测试
	turbo run test

lint:                   ## 运行 lint
	turbo run lint

build:                  ## 构建所有包
	turbo run build

clean:                  ## 清理构建产物
	turbo run clean
	rm -rf node_modules .turbo

seed:                   ## （已废弃）schema + 种子由 postgres initdb 自动应用
	@echo "schema/种子已由 docker compose 的 /docker-entrypoint-initdb.d 自动应用。"
	@echo "需重跑种子请用: make reset （会删除 pg_data 卷）"

proto:                  ## 重新生成 proto 代码
	cd packages/proto && buf generate

logs:                   ## 查看所有服务日志
	docker compose logs -f

load-test:              ## 运行压测
	./scripts/load-test.sh

chaos:                  ## 故障注入
	./scripts/chaos.sh
