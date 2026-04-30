import { execSync } from 'child_process';
import { readFileSync } from 'fs';
import { resolve } from 'path';
import WebSocket from 'ws';

const DEFAULT_SERVER_URL = 'http://127.0.0.1:8420';
const DEFAULT_MANAGED_SERVER_PASSWORD = 'slopshell-test-password';
const managedServerURL = String(process.env.E2E_MANAGED_SERVER_URL || DEFAULT_SERVER_URL).trim() || DEFAULT_SERVER_URL;
const useManagedServer = process.env.E2E_MANAGED_SERVER === '1'
  || (!process.env.E2E_BASE_URL && !process.env.SLOPSHELL_TEST_SERVER_URL);
const configuredServerURL = String(
  process.env.E2E_BASE_URL
    || process.env.SLOPSHELL_TEST_SERVER_URL
    || (useManagedServer ? managedServerURL : '')
    || DEFAULT_SERVER_URL,
).trim();

export const SERVER_URL = configuredServerURL || DEFAULT_SERVER_URL;
export const WS_URL = (() => {
  const url = new URL(SERVER_URL);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString().replace(/\/$/, '');
})();
const SESSION_COOKIE_NAME = 'slopshell_session';
const preseededSessionToken = String(process.env.SLOPSHELL_TEST_SESSION_TOKEN || '').trim();

function usesManagedServer(): boolean {
  return useManagedServer;
}

// ---------------------------------------------------------------------------
// .env password loading
// ---------------------------------------------------------------------------

function loadDotEnvPassword(): string {
  try {
    const envPath = resolve(__dirname, '../../.env');
    const lines = readFileSync(envPath, 'utf-8').split('\n');
    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed.startsWith('#') || !trimmed.includes('=')) continue;
      const [key, ...rest] = trimmed.split('=');
      if (key.trim() === 'SLOPSHELL_TEST_PASSWORD') {
        const val = rest.join('=').trim();
        if (val) return val;
      }
    }
  } catch {}
  return '';
}

function loadTestPassword(): string {
  if (process.env.SLOPSHELL_TEST_PASSWORD) return process.env.SLOPSHELL_TEST_PASSWORD;
  const dotEnvPassword = loadDotEnvPassword();
  if (dotEnvPassword) return dotEnvPassword;
  if (usesManagedServer()) return DEFAULT_MANAGED_SERVER_PASSWORD;
  return '';
}

const testPassword = loadTestPassword();

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

export async function authenticate(): Promise<string> {
  if (preseededSessionToken) {
    return preseededSessionToken;
  }
  const setupResp = await fetch(`${SERVER_URL}/api/setup`);
  const setup = (await setupResp.json()) as Record<string, unknown>;

  if (!setup.has_password) return '';
  if (!testPassword) {
    throw new Error('Server requires auth but SLOPSHELL_TEST_PASSWORD not set (env or .env)');
  }

  const loginResp = await fetch(`${SERVER_URL}/api/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password: testPassword }),
  });
  if (!loginResp.ok) {
    throw new Error(`Login failed: HTTP ${loginResp.status}`);
  }

  const setCookie = loginResp.headers.get('set-cookie') || '';
  const match = setCookie.match(new RegExp(`${SESSION_COOKIE_NAME}=([^;]+)`));
  if (!match) {
    throw new Error('Login succeeded but no session cookie returned');
  }
  return match[1];
}

export function requireTestPassword(): string {
  if (!testPassword) {
    throw new Error('Server requires auth but SLOPSHELL_TEST_PASSWORD not set (env or .env)');
  }
  return testPassword;
}

export async function authFetch(url: string, sessionToken: string, init?: RequestInit): Promise<Response> {
  const headers: Record<string, string> = { ...(init?.headers as Record<string, string> || {}) };
  if (sessionToken) {
    headers['Cookie'] = `${SESSION_COOKIE_NAME}=${sessionToken}`;
  }
  return fetch(url, { ...init, headers });
}

export async function getChatSessionId(sessionToken: string): Promise<string> {
  const resp = await authFetch(`${SERVER_URL}/api/runtime/workspaces`, sessionToken);
  if (!resp.ok) throw new Error(`/api/runtime/workspaces failed: HTTP ${resp.status}`);
  const body = (await resp.json()) as Record<string, unknown>;
  const list = (body.workspaces as Array<Record<string, unknown>>) || [];
  const project = list[0];
  if (!project?.chat_session_id) {
    throw new Error('No project with chat_session_id found');
  }
  return String(project.chat_session_id);
}

export async function clearLiveChat(sessionToken: string): Promise<void> {
  const sessionID = await getChatSessionId(sessionToken);
  const resp = await authFetch(`${SERVER_URL}/api/chat/sessions/${encodeURIComponent(sessionID)}/commands`, sessionToken, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ command: '/clear' }),
  });
  if (!resp.ok) {
    throw new Error(`/api/chat/sessions/${sessionID}/commands failed: HTTP ${resp.status}`);
  }
}

// ---------------------------------------------------------------------------
// WAV generators
// ---------------------------------------------------------------------------

export function buildWavSilence(durationMs: number, sampleRate = 16000, bitsPerSample = 16): Buffer {
  const bytesPerSample = bitsPerSample / 8;
  const numSamples = Math.floor(sampleRate * (durationMs / 1000));
  const dataSize = numSamples * bytesPerSample;
  const buf = Buffer.alloc(44 + dataSize);
  buf.write('RIFF', 0);
  buf.writeUInt32LE(36 + dataSize, 4);
  buf.write('WAVE', 8);
  buf.write('fmt ', 12);
  buf.writeUInt32LE(16, 16);
  buf.writeUInt16LE(1, 20);
  buf.writeUInt16LE(1, 22);
  buf.writeUInt32LE(sampleRate, 24);
  buf.writeUInt32LE(sampleRate * bytesPerSample, 28);
  buf.writeUInt16LE(bytesPerSample, 32);
  buf.writeUInt16LE(bitsPerSample, 34);
  buf.write('data', 36);
  buf.writeUInt32LE(dataSize, 40);
  return buf;
}

export function buildWavSineWave(durationMs: number, freq = 440, sampleRate = 16000, bitsPerSample = 16): Buffer {
  const bytesPerSample = bitsPerSample / 8;
  const numSamples = Math.floor(sampleRate * (durationMs / 1000));
  const dataSize = numSamples * bytesPerSample;
  const buf = Buffer.alloc(44 + dataSize);
  buf.write('RIFF', 0);
  buf.writeUInt32LE(36 + dataSize, 4);
  buf.write('WAVE', 8);
  buf.write('fmt ', 12);
  buf.writeUInt32LE(16, 16);
  buf.writeUInt16LE(1, 20);
  buf.writeUInt16LE(1, 22);
  buf.writeUInt32LE(sampleRate, 24);
  buf.writeUInt32LE(sampleRate * bytesPerSample, 28);
  buf.writeUInt16LE(bytesPerSample, 32);
  buf.writeUInt16LE(bitsPerSample, 34);
  buf.write('data', 36);
  buf.writeUInt32LE(dataSize, 40);
  const amplitude = 0.8 * (Math.pow(2, bitsPerSample - 1) - 1);
  for (let i = 0; i < numSamples; i++) {
    const sample = Math.round(amplitude * Math.sin(2 * Math.PI * freq * i / sampleRate));
    buf.writeInt16LE(sample, 44 + i * bytesPerSample);
  }
  return buf;
}

export async function synthesizePiperWav(text: string): Promise<Buffer> {
  const resp = await fetch('http://127.0.0.1:8424/v1/audio/speech', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ input: text, voice: 'en', response_format: 'wav' }),
  });
  if (!resp.ok) {
    throw new Error(`Piper TTS failed: HTTP ${resp.status}`);
  }
  return Buffer.from(await resp.arrayBuffer());
}

export function transcodeWavToM4A(wav: Buffer): Buffer {
  const { mkdtempSync, writeFileSync, rmSync } = require('fs') as typeof import('fs');
  const { join } = require('path') as typeof import('path');
  const { tmpdir } = require('os') as typeof import('os');
  const dir = mkdtempSync(join(tmpdir(), 'slopshell-e2e-m4a-'));
  const inPath = join(dir, 'input.wav');
  const outPath = join(dir, 'output.m4a');
  writeFileSync(inPath, wav);
  try {
    execSync(
      `ffmpeg -hide_banner -loglevel error -nostdin -y -i "${inPath}" -ac 1 -ar 16000 -c:a aac -b:a 64k "${outPath}"`,
      { stdio: 'pipe' },
    );
    return readFileSync(outPath);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// ---------------------------------------------------------------------------
// Raw WebSocket wrapper with auth cookie
// ---------------------------------------------------------------------------

export type WSMessage = { kind: 'text'; data: string } | { kind: 'binary'; data: Buffer };

export interface RawWSConn {
  ws: WebSocket;
  messages: WSMessage[];
  waitForText: (predicate: (msg: Record<string, unknown>) => boolean, timeoutMs?: number) => Promise<Record<string, unknown>>;
  waitForBinary: (timeoutMs?: number) => Promise<Buffer>;
  close: () => void;
}

export function openRawWS(url: string, sessionToken: string): Promise<RawWSConn> {
  return new Promise((resolve, reject) => {
    const headers: Record<string, string> = {};
    if (sessionToken) {
      headers['Cookie'] = `${SESSION_COOKIE_NAME}=${sessionToken}`;
    }
    const ws = new WebSocket(url, { headers });
    const messages: WSMessage[] = [];
    const listeners: Array<(msg: WSMessage) => void> = [];

    ws.on('open', () => {
      resolve({
        ws,
        messages,
        waitForText(predicate, timeoutMs = 10_000) {
          return new Promise((res, rej) => {
            const timer = setTimeout(() => rej(new Error(`waitForText timed out after ${timeoutMs}ms`)), timeoutMs);
            for (const m of messages) {
              if (m.kind === 'text') {
                try {
                  const parsed = JSON.parse(m.data);
                  if (predicate(parsed)) { clearTimeout(timer); res(parsed); return; }
                } catch {}
              }
            }
            const listener = (msg: WSMessage) => {
              if (msg.kind !== 'text') return;
              try {
                const parsed = JSON.parse(msg.data);
                if (predicate(parsed)) {
                  clearTimeout(timer);
                  const idx = listeners.indexOf(listener);
                  if (idx >= 0) listeners.splice(idx, 1);
                  res(parsed);
                }
              } catch {}
            };
            listeners.push(listener);
          });
        },
        waitForBinary(timeoutMs = 15_000) {
          return new Promise((res, rej) => {
            const timer = setTimeout(() => rej(new Error(`waitForBinary timed out after ${timeoutMs}ms`)), timeoutMs);
            for (const m of messages) {
              if (m.kind === 'binary') { clearTimeout(timer); res(m.data); return; }
            }
            const listener = (msg: WSMessage) => {
              if (msg.kind !== 'binary') return;
              clearTimeout(timer);
              const idx = listeners.indexOf(listener);
              if (idx >= 0) listeners.splice(idx, 1);
              res(msg.data);
            };
            listeners.push(listener);
          });
        },
        close() { ws.close(); },
      });
    });

    ws.on('message', (data: Buffer | string, isBinary: boolean) => {
      const msg: WSMessage = isBinary
        ? { kind: 'binary', data: Buffer.isBuffer(data) ? data : Buffer.from(String(data)) }
        : { kind: 'text', data: Buffer.isBuffer(data) ? data.toString('utf-8') : String(data) };
      messages.push(msg);
      for (const listener of listeners.slice()) listener(msg);
    });

    ws.on('error', reject);
  });
}

// ---------------------------------------------------------------------------
// STT HTTP API helper
// ---------------------------------------------------------------------------

export async function postSTTTranscribeAPI(sessionToken: string, mimeType: string, audio: Buffer) {
  const headers: Record<string, string> = {};
  if (sessionToken) {
    headers['Cookie'] = `${SESSION_COOKIE_NAME}=${sessionToken}`;
  }
  const form = new FormData();
  form.append('mime_type', mimeType);
  form.append('file', new Blob([audio], { type: mimeType }), 'audio-input');
  const resp = await fetch(`${SERVER_URL}/api/stt/transcribe`, {
    method: 'POST',
    headers,
    body: form,
  });
  const raw = await resp.text();
  let payload: Record<string, unknown> = {};
  if (raw) {
    try {
      payload = JSON.parse(raw) as Record<string, unknown>;
    } catch {
      payload = {};
    }
  }
  return { status: resp.status, payload, raw };
}
