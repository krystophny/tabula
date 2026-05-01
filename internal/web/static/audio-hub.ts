const state = {
  stream: null,
  audioContext: null,
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

function getRunningAudioContext(preferred = null) {
  if (preferred && typeof preferred === 'object' && preferred.state !== 'closed') {
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

export async function ensureAudioHub(options: Record<string, any> = {}) {
  const preferredStream = hasLiveAudioTrack(options.stream) ? options.stream : null;
  if (preferredStream && !streamMatches(state.stream, preferredStream)) {
    state.stream = preferredStream;
  }
  if (!hasLiveAudioTrack(state.stream)) {
    const acquire = typeof options.acquireMicStream === 'function' ? options.acquireMicStream : null;
    state.stream = acquire ? await acquire() : await navigator.mediaDevices.getUserMedia({ audio: true });
  }
  if (!hasLiveAudioTrack(state.stream)) {
    throw new Error('microphone stream unavailable');
  }
  return {
    stream: state.stream,
    audioContext: getRunningAudioContext(options.audioContext || null),
  };
}

export function currentAudioHubStream() {
  return hasLiveAudioTrack(state.stream) ? state.stream : null;
}
