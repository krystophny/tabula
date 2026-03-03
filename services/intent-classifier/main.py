from __future__ import annotations

import json
import math
import os
import re
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import numpy as np
import onnxruntime as ort
from fastapi import FastAPI
from pydantic import BaseModel
from transformers import AutoTokenizer

SUPPORTED_ACTIONS = {
    "switch_project",
    "switch_model",
    "toggle_silent",
    "toggle_conversation",
    "cancel_work",
    "show_status",
}

STOP_WORDS = {
    "a", "an", "and", "are", "for", "from", "how", "in", "is", "me",
    "of", "on", "or", "please", "the", "to", "we", "you",
}

ACTION_PATTERNS: list[tuple[re.Pattern[str], str, float]] = [
    (re.compile(r"\b(switch|change|set|use)\b.*\b(codex|gpt|spark|big model)\b", re.I), "switch_model", 0.98),
    (re.compile(r"\b(switch|change|go|open|activate|work in|move)\b.*\bproject\b", re.I), "switch_project", 0.94),
    (re.compile(r"\b(be quiet|mute|silent|disable speech|turn off voice|stop talking)\b", re.I), "toggle_silent", 0.99),
    (re.compile(r"\b(conversation mode|continuous conversation|keep listening|toggle conversation)\b", re.I), "toggle_conversation", 0.99),
    (re.compile(r"\b(cancel|abort|halt|stop now|interrupt)\b", re.I), "cancel_work", 0.96),
    (re.compile(r"\b(status|progress|current activity|how are we doing)\b", re.I), "show_status", 0.94),
]

MODEL_DIR_ENV = "TABURA_INTENT_MODEL_DIR"
DATASET_ENV = "TABURA_INTENT_DATASET"


class ClassificationRequest(BaseModel):
    text: str


class ClassificationResponse(BaseModel):
    intent: str
    confidence: float
    entities: dict[str, Any]
    latency_ms: float


@dataclass
class Example:
    text: str
    intent: str
    entities: dict[str, Any]
    token_set: set[str]


def normalize_text(text: str) -> str:
    return re.sub(r"\s+", " ", text.strip().lower())


def tokenize(text: str) -> list[str]:
    parts = re.findall(r"[a-zA-Z0-9_\-/]+", normalize_text(text))
    return [part for part in parts if part not in STOP_WORDS]


def jaccard_similarity(a: set[str], b: set[str]) -> float:
    if not a or not b:
        return 0.0
    union = a | b
    if not union:
        return 0.0
    return len(a & b) / len(union)


def softmax(values: list[float]) -> list[float]:
    max_val = max(values)
    exps = [math.exp(v - max_val) for v in values]
    denom = sum(exps)
    return [value / denom for value in exps]


class IntentClassifier:
    def __init__(self, dataset_path: Path, model_dir: Path):
        self.examples = self._load_examples(dataset_path)
        self.label_order = sorted({example.intent for example in self.examples})
        self.session: ort.InferenceSession
        self.tokenizer: AutoTokenizer
        self.label_map: dict[int, str]
        self._load_onnx_model(model_dir)

    def _load_examples(self, dataset_path: Path) -> list[Example]:
        if not dataset_path.is_file():
            raise RuntimeError(f"intent dataset not found: {dataset_path}")
        records = json.loads(dataset_path.read_text(encoding="utf-8"))
        examples: list[Example] = []
        for record in records:
            text = str(record.get("text", "")).strip()
            intent = str(record.get("intent", "")).strip()
            if not text or not intent:
                continue
            entities = record.get("entities") or {}
            examples.append(
                Example(
                    text=text,
                    intent=intent,
                    entities=dict(entities),
                    token_set=set(tokenize(text)),
                )
            )
        if not examples:
            raise RuntimeError(f"intent dataset is empty: {dataset_path}")
        return examples

    def _load_onnx_model(self, model_dir: Path) -> None:
        model_path = model_dir / "model.onnx"
        labels_path = model_dir / "labels.json"
        tokenizer_path = model_dir / "tokenizer"
        if not model_path.is_file():
            raise RuntimeError(f"ONNX model not found: {model_path}")
        if not labels_path.is_file():
            raise RuntimeError(f"labels.json not found: {labels_path}")
        if not tokenizer_path.is_dir():
            raise RuntimeError(f"tokenizer directory not found: {tokenizer_path}")
        labels = json.loads(labels_path.read_text(encoding="utf-8"))
        self.label_map = {int(index): str(label) for index, label in labels.items()}
        self.session = ort.InferenceSession(str(model_path), providers=["CPUExecutionProvider"])
        self.tokenizer = AutoTokenizer.from_pretrained(str(tokenizer_path), use_fast=True)

    def classify(self, text: str) -> tuple[str, float, dict[str, Any]]:
        text = text.strip()
        if not text:
            return "chat", 0.0, {}

        pattern_intent, pattern_conf = self._pattern_classification(text)
        lexical_intent, lexical_conf = self._lexical_classification(text)
        model_intent, model_conf = self._onnx_classification(text)

        candidates = [
            (pattern_intent, pattern_conf),
            (lexical_intent, lexical_conf),
            (model_intent, model_conf),
        ]
        intent, confidence = max(candidates, key=lambda entry: entry[1])

        if confidence < 0.40:
            return "chat", max(confidence, 0.30), {}
        entities = self._extract_entities(text, intent)
        if intent not in SUPPORTED_ACTIONS:
            return "chat", min(confidence, 0.55), entities
        return intent, confidence, entities

    def _pattern_classification(self, text: str) -> tuple[str, float]:
        for pattern, intent, confidence in ACTION_PATTERNS:
            if pattern.search(text):
                return intent, confidence
        return "chat", 0.0

    def _lexical_classification(self, text: str) -> tuple[str, float]:
        query_tokens = set(tokenize(text))
        if not query_tokens:
            return "chat", 0.0
        best_intent = "chat"
        best_score = 0.0
        for example in self.examples:
            score = jaccard_similarity(query_tokens, example.token_set)
            if score > best_score:
                best_score = score
                best_intent = example.intent
        calibrated = min(0.95, 0.45 + (best_score * 0.55))
        return best_intent, calibrated

    def _onnx_classification(self, text: str) -> tuple[str, float]:
        encoded = self.tokenizer(
            text,
            return_tensors="np",
            truncation=True,
            padding="max_length",
            max_length=64,
        )
        inputs = {}
        for input_meta in self.session.get_inputs():
            if input_meta.name in encoded:
                inputs[input_meta.name] = encoded[input_meta.name].astype("int64")
        if not inputs:
            raise RuntimeError("ONNX model produced no matching inputs for tokenized text")

        outputs = self.session.run(None, inputs)
        if not outputs:
            raise RuntimeError("ONNX model produced no outputs")

        logits = outputs[0][0].tolist()
        probabilities = softmax(logits)
        best_index = int(max(range(len(probabilities)), key=lambda idx: probabilities[idx]))
        confidence = float(probabilities[best_index])
        intent = self.label_map[best_index]
        return intent, confidence

    def _extract_entities(self, text: str, intent: str) -> dict[str, Any]:
        entities: dict[str, Any] = {}
        normalized = normalize_text(text)

        if intent == "switch_model":
            model_match = re.search(r"\b(codex|gpt|spark|big model)\b", normalized)
            if model_match:
                alias = model_match.group(1)
                entities["alias"] = "spark" if alias == "big model" else alias
            effort_match = re.search(r"\b(low|medium|high|xhigh|extra[_ ]high)\b", normalized)
            if effort_match:
                effort = effort_match.group(1).replace(" ", "_")
                if effort == "extra_high":
                    effort = "xhigh"
                entities["effort"] = effort

        if intent == "switch_project":
            project_match = re.search(
                r"\b(?:project(?:\s+to)?|to|open|activate|work in)\s+([a-zA-Z0-9._\-]+)\b",
                normalized,
            )
            if project_match:
                entities["name"] = project_match.group(1)

        return entities


def resolve_dataset_path() -> Path:
    configured = os.getenv(DATASET_ENV, "").strip()
    if configured:
        return Path(configured).expanduser().resolve()
    return (Path(__file__).resolve().parent / "intents.json").resolve()


def resolve_model_dir() -> Path:
    configured = os.getenv(MODEL_DIR_ENV, "").strip()
    if configured:
        return Path(configured).expanduser().resolve()
    return (Path(__file__).resolve().parent / "model").resolve()


classifier = IntentClassifier(resolve_dataset_path(), resolve_model_dir())
app = FastAPI(title="Tabura Intent Classifier", version="1.0.0")


@app.get("/health")
def health() -> dict[str, Any]:
    return {
        "ok": True,
        "model_loaded": True,
        "examples": len(classifier.examples),
    }


@app.post("/classify", response_model=ClassificationResponse)
def classify(request: ClassificationRequest) -> ClassificationResponse:
    started = time.perf_counter()
    intent, confidence, entities = classifier.classify(request.text)
    latency_ms = (time.perf_counter() - started) * 1000.0
    return ClassificationResponse(
        intent=intent,
        confidence=round(float(confidence), 4),
        entities=entities,
        latency_ms=round(latency_ms, 3),
    )
