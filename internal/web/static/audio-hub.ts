import { staticURL } from './paths.js';

const AUDIO_HUB_WORKLET_URL = staticURL('hotword-worklet.js');
const AUDIO_HUB_RING_BUFFER_SIZE = 16000 * 4;

const state = {
  stream: null,
  audioContext: null,
  sourceNode: null,
  processorNode: null,
  workletContext: null,
};

const subscribers = new Set<(...args: any[]) => void>();
const ringBuffer = {
  buffer: new Float32Array(AUDIO_HUB_RING_BUFFER_SIZE),
  writePos: 0,
  filled: 0,
};

function hasLiveAudioTrack(stream) {
  if (!stream || typeof stream.getAudioTracks !== 'function') return false;
  const tracks = stream.getAudioTracks();
  if (!Array.isArray(tracks) || tracks.length === 0) return false;
  return tracks.some((track) => String(track?.readyState || '').toLowerCase() === 'live');
}

function streamMatches(current, next) {
  return current && next && current === next && hasLiveAudioTrack(current);
}

function resetFrameTap() {
  if (state.processorNode) {
    if (state.processorNode.port) state.processorNode.port.onmessage = null;
    try { state.processorNode.disconnect(); } catch (_) {}
    state.processorNode = null;
  }
  if (state.sourceNode) {
    try { state.sourceNode.disconnect(); } catch (_) {}
    state.sourceNode = null;
  }
  state.workletContext = null;
}

function getRunningAudioContext(preferred = null) {
  if (preferred && typeof preferred === 'object' && preferred.state !== 'closed') {
    state.audioContext = preferred;
    return preferred;
  }
  const AudioContextCtor = window.AudioContext || window.webkitAudioContext;
  if (!AudioContextCtor) return null;
  if (!state.audioContext || state.audioContext.state === 'closed') {
    state.audioContext = new AudioContextCtor();
  }
  if (state.audioContext.state === 'suspended' && typeof state.audioContext.resume === 'function') {
    void state.audioContext.resume().catch(() => {});
  }
  return state.audioContext;
}

function resetPreRoll() {
  ringBuffer.buffer = new Float32Array(AUDIO_HUB_RING_BUFFER_SIZE);
  ringBuffer.writePos = 0;
  ringBuffer.filled = 0;
}

function writePreRoll(samples) {
  if (!(samples instanceof Float32Array) || samples.length === 0) return;
  const buf = ringBuffer.buffer;
  const size = buf.length;
  for (let i = 0; i < samples.length; i += 1) {
    buf[ringBuffer.writePos] = samples[i];
    ringBuffer.writePos = (ringBuffer.writePos + 1) % size;
  }
  ringBuffer.filled = Math.min(ringBuffer.filled + samples.length, size);
}

function dispatchFrame(event) {
  const data = event?.data;
  if (!data || !(data.samples instanceof Float32Array) || data.samples.length === 0) return;
  writePreRoll(data.samples);
  subscribers.forEach((subscriber) => {
    try {
      subscriber(data.samples, data.sampleRate);
    } catch (_) {}
  });
}

async function ensureFrameTap(stream, audioContext) {
  if (!audioContext?.audioWorklet || typeof audioContext.audioWorklet.addModule !== 'function') {
    throw new Error('AudioWorklet is not supported in this browser');
  }
  if (state.processorNode && state.workletContext === audioContext && streamMatches(state.stream, stream)) {
    return;
  }
  resetFrameTap();
  await audioContext.audioWorklet.addModule(AUDIO_HUB_WORKLET_URL);
  const sourceNode = audioContext.createMediaStreamSource(stream);
  const processorNode = new AudioWorkletNode(audioContext, 'hotword-processor');
  processorNode.port.onmessage = dispatchFrame;
  sourceNode.connect(processorNode);
  processorNode.connect(audioContext.destination);
  state.sourceNode = sourceNode;
  state.processorNode = processorNode;
  state.workletContext = audioContext;
}

export async function ensureAudioHub(options: Record<string, any> = {}) {
  const preferredStream = hasLiveAudioTrack(options.stream) ? options.stream : null;
  if (preferredStream && !streamMatches(state.stream, preferredStream)) {
    resetFrameTap();
    state.stream = preferredStream;
  }
  if (!hasLiveAudioTrack(state.stream)) {
    resetFrameTap();
    const acquire = typeof options.acquireMicStream === 'function' ? options.acquireMicStream : null;
    state.stream = acquire ? await acquire() : await navigator.mediaDevices.getUserMedia({ audio: true });
  }
  if (!hasLiveAudioTrack(state.stream)) {
    throw new Error('microphone stream unavailable');
  }
  const audioContext = getRunningAudioContext(options.audioContext || null);
  if (options.frames === true) {
    await ensureFrameTap(state.stream, audioContext);
  }
  return {
    stream: state.stream,
    audioContext,
  };
}

export function currentAudioHubStream() {
  return hasLiveAudioTrack(state.stream) ? state.stream : null;
}

export function subscribeAudioHubFrames(callback) {
  if (typeof callback !== 'function') return () => {};
  subscribers.add(callback);
  return () => {
    subscribers.delete(callback);
  };
}

export function getAudioHubPreRoll() {
  if (ringBuffer.filled === 0) return new Float32Array(0);
  const size = ringBuffer.buffer.length;
  const len = Math.min(ringBuffer.filled, size);
  const out = new Float32Array(len);
  const startPos = (ringBuffer.writePos - len + size) % size;
  if (startPos + len <= size) {
    out.set(ringBuffer.buffer.subarray(startPos, startPos + len));
  } else {
    const firstPart = size - startPos;
    out.set(ringBuffer.buffer.subarray(startPos, size), 0);
    out.set(ringBuffer.buffer.subarray(0, len - firstPart), firstPart);
  }
  return out;
}

export function audioHubPreRollCapacitySamplesForTest() {
  return AUDIO_HUB_RING_BUFFER_SIZE;
}

export function setAudioHubPreRollForTest(samples) {
  resetPreRoll();
  if (samples instanceof Float32Array && samples.length > 0) {
    writePreRoll(samples);
  }
}
