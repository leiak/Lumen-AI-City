import os, psycopg
from .embeddings import embed_text


def get_pg_conn():
    return psycopg.connect(
        host=os.getenv("POSTGRES_HOST", "postgres"),
        port=os.getenv("POSTGRES_PORT", "5432"),
        dbname=os.getenv("POSTGRES_DB", "aicity"),
        user=os.getenv("POSTGRES_USER", "aicity"),
        password=os.getenv("POSTGRES_PASSWORD", "aicity_dev"),
    )


async def recall_from_pg(req):
    """Milvus 不可达时降级到 PG short-term memory"""
    with get_pg_conn() as conn:
        with conn.cursor() as cur:
            cur.execute("""
                SELECT message->>'text', 1.0 - (embedding <=> %s::real[]) AS score
                FROM memory_player_session
                WHERE npc_id = %s AND player_id = %s
                ORDER BY created_at DESC LIMIT %s
            """, (embed_text(req.query), req.npc_id, req.player_id, req.top_k))
            rows = cur.fetchall()
    if not rows:
        from .main import RecallResponse
        return RecallResponse(messages=[], scores=[])
    from .main import RecallResponse
    return RecallResponse(messages=[r[0] for r in rows], scores=[r[1] for r in rows])


def write_to_pg(req):
    """PG fallback 写入"""
    with get_pg_conn() as conn:
        with conn.cursor() as cur:
            cur.execute("""
                INSERT INTO memory_player_session (npc_id, player_id, message, embedding, created_at)
                VALUES (%s, %s, %s::jsonb, %s::real[], NOW())
            """, (
                req.npc_id, req.player_id,
                f'{{"text": "{req.text}"}}',
                embed_text(req.text),
            ))
        conn.commit()