"""bge-large-zh-v1.5 embeddings - 1024 维"""
import os

_DIM = 1024

def embed_text(text: str) -> list[float]:
    """生成 1024 维 embedding（生产用 FlagEmbedding / bge-large-zh-v1.5）

    本地无模型时 fallback 到全零向量（让 Milvus search 不报错但匹配为零分）。
    """
    try:
        from FlagEmbedding import FlagModel
        if not hasattr(embed_text, "_model"):
            embed_text._model = FlagModel('BAAI/bge-large-zh-v1.5')
        return embed_text._model.encode(text).tolist()
    except Exception:
        # Fallback - 真实环境应有模型
        return [0.0] * _DIM
