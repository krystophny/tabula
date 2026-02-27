#!/usr/bin/env python3
import hashlib
import json
import time

DATASET_SIZE = 512
THROUGHPUT_OPS = 300_000
LATENCY_OPS = 50_000


def make_payloads():
    actions = [
        "review this patch",
        "switch project alpha",
        "cancel active turn",
        "open pr review",
        "summarize the latest logs",
    ]
    payloads = []
    for i in range(DATASET_SIZE):
        action = actions[i % len(actions)]
        repeat = 4 + (i % 9)
        parts = [f"token{i % 17}_{j}" for j in range(repeat)]
        req = {
            "turn_id": f"turn-{i:06d}",
            "project_key": f"/workspace/proj-{i % 11:02d}",
            "message": f"Please {action} while handling backend request {i % 97}. {' '.join(parts)}",
            "chat_mode": "chat",
            "recent_files": [
                "internal/web/chat.go",
                "internal/web/server.go",
                "internal/web/static/app.js",
            ],
            "flags": {
                "silent": i % 3 == 0,
                "conversation": i % 2 == 0,
            },
            "meta": {
                "branch": "fix/tap-stop-working",
                "model": "spark",
            },
            "timestamp": 1700000000 + i,
        }
        payloads.append(json.dumps(req, separators=(",", ":")))
    return payloads


def detect_intent(text: str) -> str:
    if "open pr" in text or "review" in text:
        return "open_pr_review"
    if "switch project" in text:
        return "switch_project"
    if "cancel" in text:
        return "cancel_turn"
    return "chat"


def handle(raw: str):
    req = json.loads(raw)
    normalized = req["message"].strip().lower()
    token_count = len(normalized.split())
    digest = hashlib.sha256(
        f'{req["project_key"]}|{req["turn_id"]}|{normalized}'.encode("utf-8")
    ).hexdigest()[:16]
    response = {
        "ok": True,
        "turn_id": req["turn_id"],
        "intent": detect_intent(normalized),
        "token_count": token_count,
        "render_on_canvas": token_count > 30 or "diff" in normalized,
        "hash_prefix": digest,
    }
    out = json.dumps(response, separators=(",", ":"))
    first = ord(out[0]) if out else 0
    return len(out), first


def percentile(sorted_samples, p: float) -> float:
    if not sorted_samples:
        return 0.0
    if p <= 0:
        return sorted_samples[0]
    if p >= 1:
        return sorted_samples[-1]
    index = int(p * (len(sorted_samples) - 1))
    return sorted_samples[index]


def main():
    payloads = make_payloads()
    checksum = 0

    start = time.perf_counter()
    for i in range(THROUGHPUT_OPS):
        n, b = handle(payloads[i % DATASET_SIZE])
        checksum += n + b
    throughput_elapsed = time.perf_counter() - start

    samples = []
    for i in range(LATENCY_OPS):
        raw = payloads[(i * 7) % DATASET_SIZE]
        t0 = time.perf_counter_ns()
        n, b = handle(raw)
        dt_us = (time.perf_counter_ns() - t0) / 1000.0
        samples.append(dt_us)
        checksum += n + b

    samples.sort()
    result = {
        "runtime": "python",
        "dataset_size": DATASET_SIZE,
        "throughput_ops": THROUGHPUT_OPS,
        "latency_samples": LATENCY_OPS,
        "throughput_ops_per_sec": THROUGHPUT_OPS / throughput_elapsed,
        "latency_us": {
            "p50": percentile(samples, 0.50),
            "p95": percentile(samples, 0.95),
            "p99": percentile(samples, 0.99),
        },
        "checksum": checksum,
    }
    print(json.dumps(result, separators=(",", ":")))


if __name__ == "__main__":
    main()
