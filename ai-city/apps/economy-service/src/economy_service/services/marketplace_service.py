import json

from asyncpg import Pool

from economy_service.clients.kafka_producer import KafkaProducer
from economy_service.errors import (
    InsufficientBalance,
    NoWithdrawableRevenueError,
    RevenueWithdrawalDuplicateError,
    SelfPurchaseError,
    TemplateNotFoundError,
    TemplateTakenDownError,
    WalletNotFound,
)

_kafka: KafkaProducer | None = None
_TOPIC_MARKET_PURCHASED = "econ.market.purchased"


def set_clients(kafka: KafkaProducer | None = None) -> None:
    global _kafka
    _kafka = kafka


class MarketplaceService:
    def __init__(self, pool: Pool):
        self.pool = pool

    async def create_npc_template(
        self,
        creator_id: str,
        name: str,
        ocean_json: dict,
        price_gold: int,
        avatar_url: str | None = None,
        bt_skeleton: str | None = None,
        product_catalog: list[dict] | None = None,
    ) -> int:
        async with self.pool.acquire() as conn:
            row = await conn.fetchrow(
                """
                INSERT INTO npc_template (
                    creator_id, name, avatar_url, ocean_json,
                    bt_skeleton, product_catalog, price_gold
                )
                VALUES ($1, $2, $3, $4, $5, $6, $7)
                RETURNING id
                """,
                creator_id,
                name,
                avatar_url,
                json.dumps(ocean_json),
                bt_skeleton,
                json.dumps(product_catalog) if product_catalog is not None else None,
                price_gold,
            )
        return row["id"]

    async def list_npc_templates(
        self,
        status: str = "live",
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict]:
        async with self.pool.acquire() as conn:
            rows = await conn.fetch(
                """
                SELECT * FROM npc_template
                WHERE status = $1
                ORDER BY created_at DESC
                LIMIT $2 OFFSET $3
                """,
                status,
                limit,
                offset,
            )
        return [dict(row) for row in rows]

    async def get_npc_template(self, template_id: int) -> dict | None:
        async with self.pool.acquire() as conn:
            row = await conn.fetchrow(
                "SELECT * FROM npc_template WHERE id = $1",
                template_id,
            )
        return dict(row) if row is not None else None


    async def take_down_npc_template(self, template_id: int, actor_id: str) -> None:
        async with self.pool.acquire() as conn:
            await conn.execute(
                """
                UPDATE npc_template
                SET status = 'taken_down', updated_at = NOW()
                WHERE id = $1
                """,
                template_id,
            )

    async def create_saga_template(
        self,
        creator_id: str,
        name: str,
        price_gold: int,
        yaml_content: str,
        semantic_version: str,
        icon_url: str | None = None,
        description: str | None = None,
        npc_deps: list[str] | None = None,
    ) -> int:
        async with self.pool.acquire() as conn:
            row = await conn.fetchrow(
                """
                INSERT INTO saga_template (
                    creator_id, name, price_gold, icon_url, description,
                    yaml_content, npc_deps, semantic_version
                )
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                RETURNING id
                """,
                creator_id,
                name,
                price_gold,
                icon_url,
                description,
                yaml_content,
                npc_deps or [],
                semantic_version,
            )
        return row["id"]

    async def purchase_template(
        self,
        user_id: str,
        template_kind: str,
        template_id: int,
        idempotency_key: str,
    ) -> int:
        async with self.pool.acquire() as conn, conn.transaction():
            existing = await conn.fetchrow(
                "SELECT id FROM template_purchase WHERE idempotency_key = $1",
                idempotency_key,
            )
            if existing is not None:
                return existing["id"]

            if template_kind == "npc":
                template = await conn.fetchrow(
                    "SELECT creator_id, price_gold, status FROM npc_template "
                    "WHERE id = $1 FOR UPDATE",
                    template_id,
                )
            elif template_kind == "saga":
                template = await conn.fetchrow(
                    "SELECT creator_id, price_gold, status FROM saga_template "
                    "WHERE id = $1 FOR UPDATE",
                    template_id,
                )
            else:
                raise TemplateNotFoundError("unsupported template kind")

            if template is None:
                raise TemplateNotFoundError(f"template not found: {template_id}")
            if template["status"] != "live":
                raise TemplateTakenDownError(f"template taken down: {template_id}")

            creator_id = str(template["creator_id"])
            price = template["price_gold"]
            if creator_id == user_id:
                raise SelfPurchaseError(f"creator cannot purchase template: {template_id}")

            buyer = await conn.fetchrow(
                "SELECT gold_balance FROM wallet WHERE user_id = $1 FOR UPDATE",
                user_id,
            )
            if buyer is None:
                raise WalletNotFound(f"wallet not found: {user_id}")
            if buyer["gold_balance"] < price:
                raise InsufficientBalance(
                    f"gold balance {buyer['gold_balance']} < {price}"
                )

            purchase = await conn.fetchrow(
                """
                INSERT INTO template_purchase (
                    user_id, template_kind, template_id,
                    price_paid_gold, idempotency_key
                )
                VALUES ($1, $2, $3, $4, $5)
                ON CONFLICT (idempotency_key) DO NOTHING
                RETURNING id
                """,
                user_id,
                template_kind,
                template_id,
                price,
                idempotency_key,
            )
            if purchase is None:
                existing = await conn.fetchrow(
                    "SELECT id FROM template_purchase WHERE idempotency_key = $1",
                    idempotency_key,
                )
                if existing is None:
                    raise TemplateNotFoundError("purchase lost after idempotency conflict")
                return existing["id"]
            purchase_id = purchase["id"]

            new_buyer_balance = buyer["gold_balance"] - price
            await conn.execute(
                "UPDATE wallet SET gold_balance = $1 WHERE user_id = $2",
                new_buyer_balance,
                user_id,
            )
            await conn.execute(
                "INSERT INTO wallet (user_id) VALUES ($1) "
                "ON CONFLICT (user_id) DO NOTHING",
                creator_id,
            )
            creator = await conn.fetchrow(
                "SELECT gold_balance FROM wallet WHERE user_id = $1 FOR UPDATE",
                creator_id,
            )
            if creator is None:
                raise WalletNotFound(f"wallet not found: {creator_id}")
            new_creator_balance = creator["gold_balance"] + price
            await conn.execute(
                "UPDATE wallet SET gold_balance = $1 WHERE user_id = $2",
                new_creator_balance,
                creator_id,
            )

            await conn.execute(
                """
                INSERT INTO creator_revenue (
                    creator_id, purchase_id, amount_gold, platform_cut_gold
                )
                VALUES ($1, $2, $3, 0)
                """,
                creator_id,
                purchase_id,
                price,
            )

            await conn.execute(
                """
                INSERT INTO transaction (
                    tx_type, user_id, counterparty_id, currency, amount,
                    balance_after, trace_id
                )
                VALUES ('player_transfer', $1, $2, 'gold', $3, $4, $5)
                """,
                user_id,
                creator_id,
                -price,
                new_buyer_balance,
                f"market:{purchase_id}",
            )
            await conn.execute(
                """
                INSERT INTO transaction (
                    tx_type, user_id, counterparty_id, currency, amount,
                    balance_after, trace_id
                )
                VALUES ('player_transfer', $1, $2, 'gold', $3, $4, $5)
                """,
                creator_id,
                user_id,
                price,
                new_creator_balance,
                f"market:{purchase_id}",
            )

        if _kafka is not None:
            await _kafka.send(
                _TOPIC_MARKET_PURCHASED,
                {
                    "purchase_id": purchase_id,
                    "user_id": user_id,
                    "creator_id": creator_id,
                    "template_kind": template_kind,
                    "template_id": template_id,
                    "price_paid_gold": price,
                },
            )
        return purchase_id

    async def list_inventory(
        self,
        user_id: str,
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict]:
        async with self.pool.acquire() as conn:
            rows = await conn.fetch(
                """
                SELECT
                    p.id AS purchase_id,
                    p.template_kind,
                    p.template_id,
                    p.price_paid_gold,
                    p.created_at
                FROM template_purchase AS p
                WHERE p.user_id = $1
                ORDER BY p.created_at DESC
                LIMIT $2 OFFSET $3
                """,
                user_id,
                limit,
                offset,
            )
        return [dict(row) for row in rows]

    async def take_down_saga_template(self, template_id: int, actor_id: str) -> None:
        async with self.pool.acquire() as conn:
            await conn.execute(
                """
                UPDATE saga_template
                SET status = 'taken_down', updated_at = NOW()
                WHERE id = $1
                """,
                template_id,
            )

    async def list_creator_revenue(
        self,
        creator_id: str,
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict]:
        async with self.pool.acquire() as conn:
            rows = await conn.fetch(
                """
                SELECT
                    r.purchase_id,
                    r.amount_gold,
                    r.platform_cut_gold,
                    r.created_at
                FROM creator_revenue AS r
                WHERE r.creator_id = $1
                ORDER BY r.created_at DESC
                LIMIT $2 OFFSET $3
                """,
                creator_id,
                limit,
                offset,
            )
        return [dict(row) for row in rows]

    async def list_saga_templates(
        self,
        status: str = "live",
        limit: int = 50,
        offset: int = 0,
    ) -> list[dict]:
        async with self.pool.acquire() as conn:
            rows = await conn.fetch(
                """
                SELECT * FROM saga_template
                WHERE status = $1
                ORDER BY created_at DESC
                LIMIT $2 OFFSET $3
                """,
                status,
                limit,
                offset,
            )
        return [dict(row) for row in rows]

    async def get_saga_template(self, template_id: int) -> dict | None:
        async with self.pool.acquire() as conn:
            row = await conn.fetchrow(
                "SELECT * FROM saga_template WHERE id = $1",
                template_id,
            )
        return dict(row) if row is not None else None

    async def get_creator_revenue_summary(self, creator_id: str) -> dict:
        async with self.pool.acquire() as conn:
            earned = await conn.fetchrow(
                """
                SELECT COALESCE(SUM(amount_gold), 0)::BIGINT AS total
                FROM creator_revenue
                WHERE creator_id = $1
                """,
                creator_id,
            )
            withdrawn = await conn.fetchrow(
                """
                SELECT COALESCE(SUM(amount_gold), 0)::BIGINT AS total
                FROM creator_withdrawal
                WHERE creator_id = $1
                """,
                creator_id,
            )

        earned_gold = int(earned["total"]) if earned else 0
        withdrawn_gold = int(withdrawn["total"]) if withdrawn else 0
        return {
            "earned_gold": earned_gold,
            "withdrawn_gold": withdrawn_gold,
            "available_gold": earned_gold - withdrawn_gold,
        }

    async def withdraw_creator_revenue(self, creator_id: str, idempotency_key: str) -> dict:
        async with self.pool.acquire() as conn:
            existing = await conn.fetchrow(
                """
                SELECT id, creator_id, amount_gold, balance_after
                FROM creator_withdrawal
                WHERE idempotency_key = $1
                """,
                idempotency_key,
            )
            if existing is not None:
                if str(existing["creator_id"]) != creator_id:
                    raise RevenueWithdrawalDuplicateError(
                        "revenue withdrawal idempotency key belongs to another creator"
                    )
                return {
                    "withdrawal_id": existing["id"],
                    "amount_gold": existing["amount_gold"],
                    "balance_after": existing["balance_after"],
                    "status": "already_settled",
                }

            wallet = await conn.fetchrow(
                "SELECT gold_balance FROM wallet WHERE user_id = $1 FOR UPDATE",
                creator_id,
            )
            if wallet is None:
                raise WalletNotFound(f"wallet not found: {creator_id}")

            earned = await conn.fetchrow(
                """
                SELECT COALESCE(SUM(amount_gold), 0)::BIGINT AS total
                FROM creator_revenue
                WHERE creator_id = $1
                """,
                creator_id,
            )
            withdrawn = await conn.fetchrow(
                """
                SELECT COALESCE(SUM(amount_gold), 0)::BIGINT AS total
                FROM creator_withdrawal
                WHERE creator_id = $1
                """,
                creator_id,
            )
            amount = int(earned["total"]) - int(withdrawn["total"])
            if amount <= 0:
                raise NoWithdrawableRevenueError("creator revenue has already been settled")

            balance = int(wallet["gold_balance"])
            if balance < amount:
                raise InsufficientBalance(
                    f"gold balance {balance} < withdrawable revenue {amount}"
                )
            new_balance = balance - amount
            await conn.execute(
                "UPDATE wallet SET gold_balance = $1 WHERE user_id = $2",
                new_balance,
                creator_id,
            )
            withdrawal = await conn.fetchrow(
                """
                INSERT INTO creator_withdrawal (
                    creator_id, amount_gold, balance_after, idempotency_key
                )
                VALUES ($1, $2, $3, $4)
                RETURNING id, amount_gold, balance_after
                """,
                creator_id,
                amount,
                new_balance,
                idempotency_key,
            )
            await conn.execute(
                """
                INSERT INTO transaction (
                    tx_type, user_id, currency, amount, balance_after, trace_id
                )
                VALUES ('creator_withdrawal', $1, 'gold', $2, $3, $4)
                """,
                creator_id,
                -amount,
                new_balance,
                f"creator_withdrawal:{withdrawal['id']}",
            )

        return {
            "withdrawal_id": withdrawal["id"],
            "amount_gold": withdrawal["amount_gold"],
            "balance_after": withdrawal["balance_after"],
            "status": "settled",
        }
