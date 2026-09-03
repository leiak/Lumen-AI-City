"""配置管理。"""
from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    service_name: str = "agent-os"
    log_level: str = "info"

    # LLM
    anthropic_api_key: str = ""
    litellm_base_url: str = "http://localhost:4000"
    primary_model: str = "claude-sonnet-4-6"
    fallback_model: str = "claude-haiku-4-5-20251001"
    daily_budget_usd: float = 5000.0

    # 数据
    redis_url: str = "redis://localhost:6379/0"
    database_url: str = "postgresql://aicity:aicity_dev@localhost:5432/aicity"
    kafka_brokers: str = "localhost:9092"

    # Tick（按 §19.9）
    cbd_tick_seconds: float = 2.0
    residential_tick_seconds: float = 5.0
    suburb_tick_seconds: float = 10.0

    # LOD
    lod0_distance_m: float = 50.0  # < 50m 升级到 L1
    lod1_distance_m: float = 0.0   # 玩家选中升级到 L2

    # 决策日志
    log_to_loki: bool = True
    log_to_kafka: bool = True


settings = Settings()
