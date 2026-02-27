use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::time::Instant;

const DATASET_SIZE: usize = 512;
const THROUGHPUT_OPS: usize = 300_000;
const LATENCY_OPS: usize = 50_000;

#[derive(Deserialize)]
struct RequestPayload {
    turn_id: String,
    project_key: String,
    message: String,
    chat_mode: String,
    recent_files: Vec<String>,
    flags: HashMap<String, bool>,
    meta: HashMap<String, String>,
    timestamp: i64,
}

#[derive(Serialize)]
struct ResponsePayload {
    ok: bool,
    turn_id: String,
    intent: String,
    token_count: usize,
    render_on_canvas: bool,
    hash_prefix: String,
}

#[derive(Serialize)]
struct ResultPayload {
    runtime: String,
    dataset_size: usize,
    throughput_ops: usize,
    latency_samples: usize,
    throughput_ops_per_sec: f64,
    latency_us: HashMap<String, f64>,
    checksum: u64,
}

fn make_payloads() -> Vec<String> {
    let actions = [
        "review this patch",
        "switch project alpha",
        "cancel active turn",
        "open pr review",
        "summarize the latest logs",
    ];
    let mut payloads: Vec<String> = Vec::with_capacity(DATASET_SIZE);
    for i in 0..DATASET_SIZE {
        let action = actions[i % actions.len()];
        let repeat = 4 + (i % 9);
        let mut parts: Vec<String> = Vec::with_capacity(repeat);
        for j in 0..repeat {
            parts.push(format!("token{}_{}", i % 17, j));
        }
        let req = serde_json::json!({
            "turn_id": format!("turn-{i:06}"),
            "project_key": format!("/workspace/proj-{num:02}", num = i % 11),
            "message": format!("Please {action} while handling backend request {}. {}", i % 97, parts.join(" ")),
            "chat_mode": "chat",
            "recent_files": ["internal/web/chat.go", "internal/web/server.go", "internal/web/static/app.js"],
            "flags": {
                "silent": i % 3 == 0,
                "conversation": i % 2 == 0
            },
            "meta": {
                "branch": "fix/tap-stop-working",
                "model": "spark"
            },
            "timestamp": 1700000000_i64 + i as i64
        });
        payloads.push(req.to_string());
    }
    payloads
}

fn detect_intent(text: &str) -> &'static str {
    if text.contains("open pr") || text.contains("review") {
        return "open_pr_review";
    }
    if text.contains("switch project") {
        return "switch_project";
    }
    if text.contains("cancel") {
        return "cancel_turn";
    }
    "chat"
}

fn handle(raw: &str) -> (usize, u8) {
    let req: RequestPayload = serde_json::from_str(raw).expect("parse request");
    let normalized = req.message.trim().to_ascii_lowercase();
    let token_count = normalized.split_whitespace().count();
    let digest = Sha256::digest(
        format!("{}|{}|{}", req.project_key, req.turn_id, normalized).as_bytes(),
    );
    let hash_prefix = hex_prefix(&digest);
    let response = ResponsePayload {
        ok: true,
        turn_id: req.turn_id,
        intent: detect_intent(&normalized).to_string(),
        token_count,
        render_on_canvas: token_count > 30 || normalized.contains("diff"),
        hash_prefix,
    };
    let out = serde_json::to_string(&response).expect("serialize response");
    let first = out.as_bytes().first().copied().unwrap_or(0);
    (out.len(), first)
}

fn hex_prefix(bytes: &[u8]) -> String {
    let mut out = String::with_capacity(16);
    for b in bytes.iter().take(8) {
        out.push_str(&format!("{b:02x}"));
    }
    out
}

fn percentile(sorted_samples: &[f64], p: f64) -> f64 {
    if sorted_samples.is_empty() {
        return 0.0;
    }
    if p <= 0.0 {
        return sorted_samples[0];
    }
    if p >= 1.0 {
        return *sorted_samples.last().expect("non-empty");
    }
    let idx = (p * ((sorted_samples.len() - 1) as f64)).floor() as usize;
    sorted_samples[idx]
}

fn main() {
    let payloads = make_payloads();
    let mut checksum: u64 = 0;

    let start_throughput = Instant::now();
    for i in 0..THROUGHPUT_OPS {
        let (n, b) = handle(&payloads[i % DATASET_SIZE]);
        checksum += n as u64 + b as u64;
    }
    let throughput_elapsed = start_throughput.elapsed().as_secs_f64();

    let mut latency_samples: Vec<f64> = Vec::with_capacity(LATENCY_OPS);
    for i in 0..LATENCY_OPS {
        let raw = &payloads[(i * 7) % DATASET_SIZE];
        let start = Instant::now();
        let (n, b) = handle(raw);
        let dt_us = start.elapsed().as_nanos() as f64 / 1000.0;
        latency_samples.push(dt_us);
        checksum += n as u64 + b as u64;
    }
    latency_samples.sort_by(|a, b| a.total_cmp(b));

    let mut latency_us = HashMap::new();
    latency_us.insert("p50".to_string(), percentile(&latency_samples, 0.50));
    latency_us.insert("p95".to_string(), percentile(&latency_samples, 0.95));
    latency_us.insert("p99".to_string(), percentile(&latency_samples, 0.99));

    let result = ResultPayload {
        runtime: "rust".to_string(),
        dataset_size: DATASET_SIZE,
        throughput_ops: THROUGHPUT_OPS,
        latency_samples: LATENCY_OPS,
        throughput_ops_per_sec: THROUGHPUT_OPS as f64 / throughput_elapsed,
        latency_us,
        checksum,
    };
    let out = serde_json::to_string(&result).expect("serialize output");
    println!("{out}");
}
