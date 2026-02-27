import { createHash } from "node:crypto";
import { performance } from "node:perf_hooks";

const DATASET_SIZE = 512;
const THROUGHPUT_OPS = 300_000;
const LATENCY_OPS = 50_000;

type RequestPayload = {
  turn_id: string;
  project_key: string;
  message: string;
  chat_mode: string;
  recent_files: string[];
  flags: Record<string, boolean>;
  meta: Record<string, string>;
  timestamp: number;
};

type Result = {
  runtime: string;
  dataset_size: number;
  throughput_ops: number;
  latency_samples: number;
  throughput_ops_per_sec: number;
  latency_us: Record<string, number>;
  checksum: number;
};

function makePayloads(): string[] {
  const actions = [
    "review this patch",
    "switch project alpha",
    "cancel active turn",
    "open pr review",
    "summarize the latest logs",
  ];
  const payloads: string[] = [];
  for (let i = 0; i < DATASET_SIZE; i += 1) {
    const action = actions[i % actions.length];
    const repeat = 4 + (i % 9);
    const parts: string[] = [];
    for (let j = 0; j < repeat; j += 1) {
      parts.push(`token${i % 17}_${j}`);
    }
    const req: RequestPayload = {
      turn_id: `turn-${String(i).padStart(6, "0")}`,
      project_key: `/workspace/proj-${String(i % 11).padStart(2, "0")}`,
      message: `Please ${action} while handling backend request ${i % 97}. ${parts.join(" ")}`,
      chat_mode: "chat",
      recent_files: [
        "internal/web/chat.go",
        "internal/web/server.go",
        "internal/web/static/app.js",
      ],
      flags: {
        silent: i % 3 === 0,
        conversation: i % 2 === 0,
      },
      meta: {
        branch: "fix/tap-stop-working",
        model: "spark",
      },
      timestamp: 1_700_000_000 + i,
    };
    payloads.push(JSON.stringify(req));
  }
  return payloads;
}

function detectIntent(text: string): string {
  if (text.includes("open pr") || text.includes("review")) return "open_pr_review";
  if (text.includes("switch project")) return "switch_project";
  if (text.includes("cancel")) return "cancel_turn";
  return "chat";
}

function handle(raw: string): [number, number] {
  const req = JSON.parse(raw) as RequestPayload;
  const normalized = req.message.trim().toLowerCase();
  const tokenCount = normalized.split(/\s+/).length;
  const digest = createHash("sha256")
    .update(`${req.project_key}|${req.turn_id}|${normalized}`)
    .digest("hex")
    .slice(0, 16);
  const response = {
    ok: true,
    turn_id: req.turn_id,
    intent: detectIntent(normalized),
    token_count: tokenCount,
    render_on_canvas: tokenCount > 30 || normalized.includes("diff"),
    hash_prefix: digest,
  };
  const out = JSON.stringify(response);
  const first = out.length > 0 ? out.charCodeAt(0) : 0;
  return [out.length, first];
}

function percentile(sortedSamples: number[], p: number): number {
  if (sortedSamples.length === 0) return 0;
  if (p <= 0) return sortedSamples[0];
  if (p >= 1) return sortedSamples[sortedSamples.length - 1];
  const index = Math.floor(p * (sortedSamples.length - 1));
  return sortedSamples[index];
}

function run(): void {
  const payloads = makePayloads();
  let checksum = 0;

  const t0 = performance.now();
  for (let i = 0; i < THROUGHPUT_OPS; i += 1) {
    const [n, b] = handle(payloads[i % DATASET_SIZE]);
    checksum += n + b;
  }
  const throughputElapsedSec = (performance.now() - t0) / 1000;

  const samples: number[] = [];
  for (let i = 0; i < LATENCY_OPS; i += 1) {
    const raw = payloads[(i * 7) % DATASET_SIZE];
    const startUS = performance.now() * 1000;
    const [n, b] = handle(raw);
    const dtUS = (performance.now() * 1000) - startUS;
    samples.push(dtUS);
    checksum += n + b;
  }
  samples.sort((a, b) => a - b);

  const result: Result = {
    runtime: "typescript-node",
    dataset_size: DATASET_SIZE,
    throughput_ops: THROUGHPUT_OPS,
    latency_samples: LATENCY_OPS,
    throughput_ops_per_sec: THROUGHPUT_OPS / throughputElapsedSec,
    latency_us: {
      p50: percentile(samples, 0.50),
      p95: percentile(samples, 0.95),
      p99: percentile(samples, 0.99),
    },
    checksum,
  };
  process.stdout.write(`${JSON.stringify(result)}\n`);
}

run();
