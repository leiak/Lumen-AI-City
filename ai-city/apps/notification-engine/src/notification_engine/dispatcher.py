"""通知派发器：APNs / FCM / 用户偏好 + 频控。"""
from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass

logger = logging.getLogger(__name__)


@dataclass
class Notification:
    user_id: str
    title: str
    body: str
    data: dict
    priority: int = 5  # 1-10
    ttl_seconds: int = 3600


class NotificationDispatcher:
    def __init__(self, redis_url: str) -> None:
        self.redis_url = redis_url
        # TODO: APNs + FCM 客户端初始化

    async def send(self, notif: Notification) -> bool:
        """发送通知（含频控）。"""
        if await self._is_throttled(notif.user_id):
            logger.info(f"notification throttled for {notif.user_id}")
            return False

        # TODO: 根据设备类型分派 APNs / FCM
        logger.info(f"sending notification to {notif.user_id}: {notif.title}")
        await self._mark_sent(notif.user_id)
        return True

    async def _is_throttled(self, user_id: str) -> bool:
        # 默认 5min 内最多 1 条（频控策略可在用户偏好中配置）
        return False

    async def _mark_sent(self, user_id: str) -> None:
        # TODO: Redis 写入时间戳
        pass
