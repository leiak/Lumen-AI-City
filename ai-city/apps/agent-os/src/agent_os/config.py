"""配置管理。Sprint 12 减面：只保留本范围需要的字段。"""
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    service_name: str = "agent-os"
    log_level: str = "info"

    # HTTP（沿用 Sprint 11 T04 决定：8084 平行于 8080/8082/8083）
    http_port: int = 8084

    # Redis（沿用 world-engine 模式：手写 RESP，URL 形如 redis://host:port[/db]）
    redis_url: str = "redis://localhost:6379/0"
    redis_channel_npc_dialogue: str = "aicity:npc_dialogue"

    # NPC 模板（dev 走 monorepo 相对路径；容器化时由 Dockerfile COPY 注入 /etc/aicity/npc-templates）
    npc_templates_dir: str = "./packages/npc-templates"

    # Say 调度（5s 触发一轮；Sprint 13+ 接 player listener 后改成事件驱动）
    say_tick_seconds: float = 5.0


# Alias used by app factory:  spec 文档统一写 `Config()`。
Config = Settings
settings = Settings()
