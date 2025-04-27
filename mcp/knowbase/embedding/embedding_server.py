from fastapi import FastAPI
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer
from typing import List

app = FastAPI()
model = SentenceTransformer('all-MiniLM-L6-v2')  # 你可以换成bge、gte等模型

class EmbeddingRequest(BaseModel):
    input: List[str]

@app.post("/embed")
async def embed_text(req: EmbeddingRequest):
    embeddings = model.encode(req.input, normalize_embeddings=True).tolist()
    return {"embeddings": embeddings}

# 启动服务器
# uvicorn filename:app --host 0.0.0.0 --port 8000