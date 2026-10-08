import json

from asyncpg import Pool


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

    async def take_down_npc_template(self, template_id: int, admin_id: str) -> None:
        async with self.pool.acquire() as conn:
            await conn.execute(
                """
                UPDATE npc_template
                SET status = 'taken_down', updated_at = NOW()
                WHERE id = $1
                """,
                template_id,
            )
