"""Semantic end-of-utterance detector sidecar.

Runs a local ONNX Runtime service for turn end detection:
POST /v1/eou/predict {"text":"..."} -> {"p_end":0.93,...}
"""

from __future__ import annotations

import glob
import json
import os
from dataclasses import dataclass
from typing import Any

import numpy as np
import onnxruntime as ort
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from transformers import AutoConfig, AutoTokenizer


def _env(key: str, default: str) -> str:
    return str(os.environ.get(key, default)).strip()


MODEL_REPO = _env("TABURA_EOU_MODEL_REPO", "livekit/turn-detector")
MODEL_DIR = _env("TABURA_EOU_MODEL_DIR", os.path.expanduser("~/.local/share/tabura-eou/model"))
ONNX_PATH = _env("TABURA_EOU_ONNX_PATH", "")
MAX_LENGTH = int(_env("TABURA_EOU_MAX_LENGTH", "256") or "256")
PROVIDERS_RAW = _env("TABURA_EOU_PROVIDERS", "CUDAExecutionProvider,CPUExecutionProvider")
PROVIDERS = [p.strip() for p in PROVIDERS_RAW.split(",") if p.strip()]


class PredictRequest(BaseModel):
    text: str
    lang_hint: str | None = None


class PredictResponse(BaseModel):
    p_end: float
    label: str
    model: str
    reason: str | None = None


@dataclass
class ModelState:
    ready: bool
    error: str
    tokenizer: Any
    session: Any
    end_index: int
    model_name: str
    onnx_path: str


state = ModelState(
    ready=False,
    error="not initialized",
    tokenizer=None,
    session=None,
    end_index=1,
    model_name=MODEL_REPO,
    onnx_path="",
)

app = FastAPI()


def _find_onnx_path() -> str:
    if ONNX_PATH and os.path.isfile(ONNX_PATH):
        return ONNX_PATH
    candidates = glob.glob(os.path.join(MODEL_DIR, "**", "*.onnx"), recursive=True)
    if not candidates:
        raise FileNotFoundError(f"no .onnx model found under {MODEL_DIR}")
    candidates.sort()
    for preferred in candidates:
        name = os.path.basename(preferred).lower()
        if "int8" in name or "quant" in name or "turn" in name:
            return preferred
    return candidates[0]


def _resolve_end_index(config: Any) -> int:
    id2label = getattr(config, "id2label", None) or {}
    if isinstance(id2label, dict):
        normalized = {}
        for k, v in id2label.items():
            try:
                idx = int(k)
            except Exception:
                continue
            normalized[idx] = str(v).strip().lower()
        for idx, label in normalized.items():
            if any(tok in label for tok in ("end", "eot", "final", "stop")):
                return idx
    num_labels = int(getattr(config, "num_labels", 2) or 2)
    if num_labels > 1:
        return 1
    return 0


def _sigmoid(x: np.ndarray) -> np.ndarray:
    return 1.0 / (1.0 + np.exp(-x))


def _softmax(x: np.ndarray) -> np.ndarray:
    shifted = x - np.max(x, axis=-1, keepdims=True)
    expv = np.exp(shifted)
    return expv / np.sum(expv, axis=-1, keepdims=True)


def _prepare_inputs(text: str) -> dict[str, np.ndarray]:
    encoded = state.tokenizer(
        text,
        return_tensors="np",
        truncation=True,
        max_length=MAX_LENGTH,
    )
    by_name: dict[str, np.ndarray] = {}
    input_names = [item.name for item in state.session.get_inputs()]
    for name in input_names:
        if name in encoded:
            arr = np.asarray(encoded[name])
            if arr.dtype != np.int64:
                arr = arr.astype(np.int64)
            by_name[name] = arr
            continue
        if name == "token_type_ids" and "input_ids" in encoded:
            by_name[name] = np.zeros_like(np.asarray(encoded["input_ids"]), dtype=np.int64)
    return by_name


def _predict_end_probability(text: str) -> float:
    inputs = _prepare_inputs(text)
    outputs = state.session.run(None, inputs)
    if not outputs:
        raise RuntimeError("model produced no outputs")
    logits = np.asarray(outputs[0])
    if logits.ndim == 1:
        logits = logits.reshape(1, -1)
    if logits.shape[-1] == 1:
        return float(_sigmoid(logits)[0][0])
    probs = _softmax(logits)
    idx = state.end_index
    if idx < 0 or idx >= probs.shape[-1]:
        idx = min(1, probs.shape[-1] - 1)
    return float(probs[0][idx])


@app.on_event("startup")
def startup() -> None:
    try:
        onnx_path = _find_onnx_path()
        tokenizer = AutoTokenizer.from_pretrained(MODEL_DIR, local_files_only=True)
        config = AutoConfig.from_pretrained(MODEL_DIR, local_files_only=True)
        session = ort.InferenceSession(onnx_path, providers=PROVIDERS)
        state.tokenizer = tokenizer
        state.session = session
        state.end_index = _resolve_end_index(config)
        state.model_name = MODEL_REPO
        state.onnx_path = onnx_path
        state.ready = True
        state.error = ""
        print(f"EOU model ready: repo={MODEL_REPO} onnx={onnx_path} providers={session.get_providers()}")
    except Exception as exc:
        state.ready = False
        state.error = str(exc)
        print(f"EOU model startup failed: {state.error}")


@app.get("/health")
def health() -> dict[str, Any]:
    return {
        "status": "ok" if state.ready else "degraded",
        "ready": state.ready,
        "error": state.error,
        "model_repo": MODEL_REPO,
        "model_dir": MODEL_DIR,
        "onnx_path": state.onnx_path,
        "providers": state.session.get_providers() if state.session else [],
    }


@app.post("/v1/eou/predict", response_model=PredictResponse)
def predict(req: PredictRequest) -> PredictResponse:
    if not state.ready or state.session is None:
        raise HTTPException(status_code=503, detail=f"model unavailable: {state.error}")
    text = (req.text or "").strip()
    if not text:
        return PredictResponse(
            p_end=0.0,
            label="continue",
            model=MODEL_REPO,
            reason="empty_text",
        )
    try:
        p_end = _predict_end_probability(text)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"inference failed: {exc}") from exc
    label = "end" if p_end >= 0.5 else "continue"
    return PredictResponse(p_end=p_end, label=label, model=MODEL_REPO)


@app.get("/")
def root() -> str:
    return json.dumps({"service": "tabura-eou", "ready": state.ready})

