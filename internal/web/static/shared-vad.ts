import { ensureAudioHub } from './audio-hub.js';
import { initVAD } from './vad.js';

export const SHARED_VAD_MODE_IDLE = 'idle';
export const SHARED_VAD_MODE_CAPTURE = 'capture';
export const SHARED_VAD_MODE_DIALOGUE = 'dialogue';
export const SHARED_VAD_MODE_MEETING = 'meeting';

const state: Record<string, any> = {
  instance: null,
  stream: null,
  starting: null,
  mode: SHARED_VAD_MODE_IDLE,
  handlers: {},
};

function normalizeMode(mode) {
  const clean = String(mode || '').trim().toLowerCase();
  switch (clean) {
    case SHARED_VAD_MODE_CAPTURE:
    case SHARED_VAD_MODE_DIALOGUE:
    case SHARED_VAD_MODE_MEETING:
      return clean;
    default:
      return SHARED_VAD_MODE_IDLE;
  }
}

function callHandler(name, ...args) {
  const fn = state.handlers?.[name];
  if (typeof fn !== 'function') return;
  try {
    fn(...args);
  } catch (err) {
    const onError = state.handlers?.onError;
    if (typeof onError === 'function') onError(err);
  }
}

function sameStream(stream) {
  return state.stream && stream && state.stream === stream;
}

async function createSharedVAD(options) {
  const hub = await ensureAudioHub(options);
  if (state.instance && sameStream(hub.stream)) {
    if (!state.instance.isActive?.()) state.instance.start();
    return state.instance;
  }
  if (state.instance) {
    try { state.instance.destroy(); } catch (_) {}
    state.instance = null;
  }
  state.stream = hub.stream;
  const instance = await initVAD({
    stream: hub.stream,
    audioContext: hub.audioContext || undefined,
    positiveSpeechThreshold: 0.5,
    negativeSpeechThreshold: 0.3,
    redemptionMs: 1000,
    minSpeechMs: 200,
    preSpeechPadMs: 800,
    onSpeechStart() {
      callHandler('onSpeechStart');
    },
    onSpeechEnd(audio) {
      callHandler('onSpeechEnd', audio);
    },
    onFrameProcessed(probs) {
      callHandler('onFrameProcessed', probs);
    },
    onError(err) {
      callHandler('onError', err);
    },
  });
  if (!instance) return null;
  state.instance = instance;
  instance.start();
  return instance;
}

export async function ensureSharedVAD(options: Record<string, any> = {}) {
  if (!state.starting) {
    state.starting = createSharedVAD(options).finally(() => {
      state.starting = null;
    });
  }
  return state.starting;
}

export function setSharedVADMode(mode, handlers: Record<string, any> = {}) {
  state.mode = normalizeMode(mode);
  state.handlers = state.mode === SHARED_VAD_MODE_IDLE ? {} : { ...handlers };
}

export function clearSharedVADMode(mode = '') {
  if (mode && normalizeMode(mode) !== state.mode) return;
  state.mode = SHARED_VAD_MODE_IDLE;
  state.handlers = {};
}

export function sharedVADMode() {
  return state.mode;
}
