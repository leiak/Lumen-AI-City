import time

from fastapi import FastAPI
from pydantic import BaseModel
from typing import List
from .milvus_client import get_milvus_client
from .embeddings import embed_text

app = FastAPI(title="memory-service", version="2.0.0")


class RecallRequest(BaseModel):
    npc_id: str
    player_id: str
    query: str
    top_k: int = 5


class RecallResponse(BaseModel):
    messages: List[str]
    scores: List[float]


class WriteRequest(BaseModel):
    npc_id: str
    player_id: str
    text: str


@app.post("/v1/recall", response_model=RecallResponse)
async def recall(req: RecallRequest):
    """Recall top-K memories for an NPC-player pair (Milvus vector search)"""
    collection_name = f"mem_{req.npc_id.split('_')[1]}_{'_'.join(req.npc_id.split('_')[2:])}"
    client = get_milvus_client()
    try:
        collection = client.Collection(collection_name)
        collection.load()
        query_embedding = embed_text(req.query)
        results = collection.search(
            data=[query_embedding],
            anns_field="embedding",
            param={"metric_type": "COSINE", "params": {"nprobe": 10}},
            limit=req.top_k,
            expr=f'player_id == "{req.player_id}"',
            output_fields=["text"],
        )
        messages = [hit.entity.get("text") for hit in results[0]]
        scores = [hit.score for hit in results[0]]
        return RecallResponse(messages=messages, scores=scores)
    except Exception:
        # Milvus 不可用时降级到空结果（spec §3.3）
        return RecallResponse(messages=[], scores=[])


@app.post("/v1/write")
async def write(req: WriteRequest):
    """写入 memory（player-NPC 对话）"""
    collection_name = f"mem_{req.npc_id.split('_')[1]}_{'_'.join(req.npc_id.split('_')[2:])}"
    client = get_milvus_client()
    try:
        collection = client.Collection(collection_name)
        embedding = embed_text(req.text)
        collection.insert([[req.player_id], [req.text], [embedding], [int(time.time()*1000)]])
        collection.flush()
        return {"status": "written", "npc_id": req.npc_id}
    except Exception:
        # Milvus 不可用，尝试 PG fallback
        from .pg_client import write_to_pg
        write_to_pg(req)
        return {"status": "written_pg_fallback", "npc_id": req.npc_id}


@app.get("/healthz")
async def healthz():
    return {"status": "ok", "service": "memory-service"}


@app.get("/readyz")
async def readyz():
    # TODO(week 1 day 4): check Milvus + PG connection
    return {"status": "ready", "milvus": False, "postgres": False}
