from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from sentence_transformers import SentenceTransformer
from typing import List, Dict, Optional, Any
import torch  # For device checking
import logging  # For better logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="Embedding Service API",
    description="API for generating text embeddings using various local models.",
    version="1.0.0"
)

# --- Model Configuration ---
AVAILABLE_MODELS_CONFIG: Dict[str, Dict[str, Any]] = {
    "minilm": {"hf_name": "all-MiniLM-L6-v2", "trust_remote_code": False},
    "bge-base-en": {"hf_name": "BAAI/bge-base-en-v1.5", "trust_remote_code": False},
    "bge-large-en": {"hf_name": "BAAI/bge-large-en-v1.5", "trust_remote_code": False},
    "bge-m3": {"hf_name": "BAAI/bge-m3", "trust_remote_code": True},  # Powerful multilingual
    "gte-base": {"hf_name": "thenlper/gte-base", "trust_remote_code": False},
    "gte-large": {"hf_name": "thenlper/gte-large", "trust_remote_code": False},
    "e5-large": {"hf_name": "intfloat/e5-large-v2", "trust_remote_code": False},
    "instructor-large": {"hf_name": "hkunlp/instructor-large", "trust_remote_code": False},
}

# --- Global Variables ---
loaded_models: Dict[str, SentenceTransformer] = {}
# VVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVVV
# 修改这里，将 bge-m3 设置为默认模型
DEFAULT_MODEL_ALIAS = "bge-m3"


# ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

# --- Helper Function to Get Model ---
def get_model(model_alias: str, device_preference: Optional[str] = None) -> SentenceTransformer:
    if model_alias not in AVAILABLE_MODELS_CONFIG:
        raise HTTPException(
            status_code=400,
            detail=f"Model alias '{model_alias}' not found. Available models: {list(AVAILABLE_MODELS_CONFIG.keys())}"
        )

    if model_alias not in loaded_models:
        model_config = AVAILABLE_MODELS_CONFIG[model_alias]
        hf_model_name = model_config["hf_name"]
        trust_code = model_config.get("trust_remote_code", False)

        selected_device = device_preference
        if not selected_device:
            if torch.cuda.is_available():
                selected_device = "cuda"
            # 检查 Apple Silicon (M1/M2/M3) 的 MPS (Metal Performance Shaders)
            elif torch.backends.mps.is_available() and torch.backends.mps.is_built():
                selected_device = "mps"
                logger.info("MPS backend is available and will be used for Apple Silicon.")
            else:
                selected_device = "cpu"
                logger.info("CUDA and MPS not available, using CPU.")

        logger.info(
            f"Attempting to load model '{hf_model_name}' (alias: '{model_alias}') onto device '{selected_device}'...")
        try:
            loaded_models[model_alias] = SentenceTransformer(
                model_name_or_path=hf_model_name,
                device=selected_device,
                trust_remote_code=trust_code
            )
            logger.info(f"Model '{hf_model_name}' loaded successfully onto '{selected_device}'.")
        except Exception as e:
            logger.error(f"Error loading model {hf_model_name}: {e}", exc_info=True)
            if model_alias in loaded_models:  # Clean up if partial load occurred
                del loaded_models[model_alias]
            raise HTTPException(status_code=500, detail=f"Could not load model '{model_alias}': {str(e)}")

    return loaded_models[model_alias]


# --- Pydantic Models ---
class EmbeddingRequest(BaseModel):
    input: List[str]
    model_alias: Optional[str] = None  # 将默认值设为 None，以便在路由函数中使用 DEFAULT_MODEL_ALIAS
    normalize_embeddings: bool = True


class EmbeddingResponse(BaseModel):
    embeddings: List[List[float]]
    model_used: str
    model_alias_used: str


class ModelInfo(BaseModel):
    alias: str
    hf_name: str


class AvailableModelsResponse(BaseModel):
    default_model_alias: str
    available_models: Dict[str, ModelInfo]


# --- API Endpoints ---
@app.on_event("startup")
async def startup_event():
    logger.info("Application startup...")
    # 考虑在启动时预加载默认模型，以加快首次请求的速度
    # 但请注意，这会增加启动时间，并立即消耗内存
    # 对于 bge-m3 这样的大模型，在Mac上启动时加载可能会比较慢
    # logger.info(f"Pre-loading default model: {DEFAULT_MODEL_ALIAS}")
    # try:
    #     get_model(DEFAULT_MODEL_ALIAS) # This will load it into memory
    # except Exception as e:
    #     logger.warning(f"Could not pre-load default model {DEFAULT_MODEL_ALIAS}: {e}")
    #     logger.warning("The service will still start, but the first request to the default model might be significantly slower.")
    pass


@app.post("/embed", response_model=EmbeddingResponse)
async def embed_text(req: EmbeddingRequest):
    if not req.input:
        raise HTTPException(status_code=400, detail="Input list cannot be empty.")

    # 如果请求中没有指定 model_alias，则使用 DEFAULT_MODEL_ALIAS
    model_alias_to_use = req.model_alias if req.model_alias is not None else DEFAULT_MODEL_ALIAS

    try:
        model = get_model(model_alias_to_use)
    except HTTPException as e:
        raise e
    except Exception as e:
        logger.error(f"Unexpected error getting model '{model_alias_to_use}': {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"An unexpected error occurred: {str(e)}")

    texts_to_embed = req.input
    try:
        embeddings = model.encode(
            texts_to_embed,
            normalize_embeddings=req.normalize_embeddings
        ).tolist()
    except Exception as e:
        logger.error(f"Error during model.encode for model '{model_alias_to_use}': {e}", exc_info=True)
        raise HTTPException(status_code=500,
                            detail=f"Error generating embeddings with model '{model_alias_to_use}': {str(e)}")

    return EmbeddingResponse(
        embeddings=embeddings,
        model_used=AVAILABLE_MODELS_CONFIG[model_alias_to_use]["hf_name"],
        model_alias_used=model_alias_to_use
    )


@app.get("/models", response_model=AvailableModelsResponse)
async def list_available_models():
    models_info = {
        alias: ModelInfo(alias=alias, hf_name=config["hf_name"])
        for alias, config in AVAILABLE_MODELS_CONFIG.items()
    }
    return AvailableModelsResponse(
        default_model_alias=DEFAULT_MODEL_ALIAS,
        available_models=models_info
    )

# --- How to Run ---
# Save this as main.py
# Install dependencies:
# pip install "fastapi[all]" uvicorn sentence-transformers torch pydantic
# (For Apple Silicon Macs, ensure PyTorch is installed correctly for MPS support, often `pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/nightly/cpu` or a specific MPS build)
# Run with Uvicorn:
# uvicorn main:app --reload --host 0.0.0.0 --port 8000


# 启动服务器
# uvicorn embedding_server:app --host 0.0.0.0 --port 8000
# uvicorn embedding_server:app --reload --host 0.0.0.0 --port 8000