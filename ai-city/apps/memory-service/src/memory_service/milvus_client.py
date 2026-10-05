import os
from pymilvus import MilvusClient as _MC

_client = None

def get_milvus_client():
    global _client
    if _client is None:
        addr = os.getenv("MILVUS_ADDR", "milvus:19530")
        _client = _MC(uri=f"http://{addr}")
    return _client
