from functools import lru_cache
from .embeddings import embed_text

@lru_cache(maxsize=128)
def cached_embed(text: str) -> tuple:
    """Embeddings LRU 缓存（128 entry）"""
    return tuple(embed_text(text))