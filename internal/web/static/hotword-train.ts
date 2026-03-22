import { apiURL, appURL } from './paths.js';
import { float32ToWav } from './app-env.js';

type Recording = {
  id: string;
  kind: string;
  created_at: string;
  file_name: string;
  size_bytes: number;
  duration_ms: number;
  audio_url: string;
};

type StatusPayload = {
  state: string;
  stage: string;
  message?: string;
  error?: string;
  progress?: number;
  generated_samples?: number;
  latest_model?: string;
  models?: Array<{ name: string; state: string; message?: string; count?: number; target?: number; output_dir?: string }>;
};

type Model = {
  name: string;
  file_name: string;
  path: string;
  created_at: string;
  size_bytes: number;
  production: boolean;
};

const state = {
  recordingActive: false,
  audioContext: null as AudioContext | null,
  sourceNode: null as MediaStreamAudioSourceNode | null,
  processorNode: null as ScriptProcessorNode | null,
  sinkNode: null as GainNode | null,
  stream: null as MediaStream | null,
  sampleRate: 16000,
  chunks: [] as Float32Array[],
};

function byId<T extends HTMLElement>(id: string) {
  const node = document.getElementById(id);
  if (!(node instanceof HTMLElement)) {
    throw new Error(`missing element: ${id}`);
  }
  return node as T;
}

const bannerEl = byId<HTMLParagraphElement>('train-banner');
const recordingKindEl = byId<HTMLSelectElement>('recording-kind');
const recordingToggleEl = byId<HTMLButtonElement>('recording-toggle');
const recordingUploadEl = byId<HTMLInputElement>('recording-upload');
const recordingBadgeEl = byId<HTMLSpanElement>('recording-badge');
const recordingStatusEl = byId<HTMLParagraphElement>('recording-status');
const recordingListEl = byId<HTMLUListElement>('recording-list');
const generationBadgeEl = byId<HTMLSpanElement>('generation-badge');
const generationStatusEl = byId<HTMLParagraphElement>('generation-status');
const generationListEl = byId<HTMLUListElement>('generation-list');
const generationCountEl = byId<HTMLInputElement>('generation-count');
const generationStartEl = byId<HTMLButtonElement>('generation-start');
const trainingBadgeEl = byId<HTMLSpanElement>('training-badge');
const trainingStatusEl = byId<HTMLParagraphElement>('training-status');
const trainingStartEl = byId<HTMLButtonElement>('training-start');
const deploymentBadgeEl = byId<HTMLSpanElement>('deployment-badge');
const deploymentStatusEl = byId<HTMLParagraphElement>('deployment-status');
const modelListEl = byId<HTMLUListElement>('model-list');

function setBanner(message = '') {
  bannerEl.hidden = !message;
  bannerEl.textContent = message;
}

function setBadge(node: HTMLElement, value: string) {
  node.textContent = String(value || 'idle');
}

function formatDate(value: string) {
  const date = new Date(String(value || ''));
  if (Number.isNaN(date.getTime())) return 'unknown time';
  return date.toLocaleString();
}

function formatBytes(value: number) {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDuration(ms: number) {
  const total = Math.max(0, Math.round(Number(ms || 0) / 1000));
  if (total < 60) return `${total}s`;
  return `${Math.floor(total / 60)}m ${String(total % 60).padStart(2, '0')}s`;
}

async function loadJSON(path: string, init?: RequestInit) {
  const resp = await fetch(apiURL(path), { cache: 'no-store', ...init });
  const payload = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    throw new Error(String(payload?.error || `HTTP ${resp.status}`));
  }
  return payload;
}

function selectedGenerationModels() {
  return Array.from(document.querySelectorAll<HTMLInputElement>('input[name="generator-model"]:checked'))
    .map((node) => node.value);
}

function renderRecordings(recordings: Recording[]) {
  recordingListEl.replaceChildren();
  if (!Array.isArray(recordings) || recordings.length === 0) {
    const empty = document.createElement('li');
    empty.className = 'train-list-item';
    empty.textContent = 'No recordings yet.';
    recordingListEl.appendChild(empty);
    return;
  }
  for (const recording of recordings) {
    const item = document.createElement('li');
    item.className = 'train-list-item';
    const title = document.createElement('div');
    title.className = 'train-list-head';
    title.innerHTML = `<strong>${recording.kind}</strong><span>${formatDate(recording.created_at)}</span>`;
    const meta = document.createElement('p');
    meta.className = 'train-list-meta';
    meta.textContent = `${recording.file_name} | ${formatDuration(recording.duration_ms)} | ${formatBytes(recording.size_bytes)}`;
    const audio = document.createElement('audio');
    audio.controls = true;
    audio.src = appURL(recording.audio_url);
    const actions = document.createElement('div');
    actions.className = 'train-list-actions';
    const remove = document.createElement('button');
    remove.className = 'train-list-button';
    remove.type = 'button';
    remove.textContent = 'Delete';
    remove.addEventListener('click', async () => {
      remove.disabled = true;
      try {
        await fetch(apiURL(`hotword/train/recordings/${encodeURIComponent(recording.id)}`), { method: 'DELETE' });
        await refreshRecordings();
      } catch (err: any) {
        setBanner(String(err?.message || err || 'delete failed'));
      } finally {
        remove.disabled = false;
      }
    });
    actions.appendChild(remove);
    item.append(title, meta, audio, actions);
    recordingListEl.appendChild(item);
  }
}

function renderGenerationStatus(status: StatusPayload) {
  setBadge(generationBadgeEl, status.state || 'idle');
  generationStatusEl.textContent = String(status.error || status.message || 'Idle.');
  generationListEl.replaceChildren();
  const models = Array.isArray(status.models) ? status.models : [];
  if (models.length === 0) {
    const empty = document.createElement('li');
    empty.className = 'train-list-item';
    empty.textContent = 'No generation job has started yet.';
    generationListEl.appendChild(empty);
    return;
  }
  for (const model of models) {
    const item = document.createElement('li');
    item.className = 'train-list-item';
    item.innerHTML = `
      <div class="train-list-head"><strong>${model.name}</strong><span>${model.state}</span></div>
      <p class="train-list-meta">${model.count || 0}/${model.target || 0} samples${model.output_dir ? ` | ${model.output_dir}` : ''}</p>
      <p class="train-list-meta">${model.message || 'Waiting.'}</p>
    `;
    generationListEl.appendChild(item);
  }
}

function renderModels(models: Model[]) {
  modelListEl.replaceChildren();
  if (!Array.isArray(models) || models.length === 0) {
    const empty = document.createElement('li');
    empty.className = 'train-list-item';
    empty.textContent = 'No trained models yet.';
    modelListEl.appendChild(empty);
    return;
  }
  for (const model of models) {
    const item = document.createElement('li');
    item.className = 'train-list-item';
    const head = document.createElement('div');
    head.className = 'train-list-head';
    head.innerHTML = `<strong>${model.file_name}</strong><span>${model.production ? 'production' : 'trained'}</span>`;
    const meta = document.createElement('p');
    meta.className = 'train-list-meta';
    meta.textContent = `${formatDate(model.created_at)} | ${formatBytes(model.size_bytes)} | ${model.path}`;
    item.append(head, meta);
    if (!model.production) {
      const actions = document.createElement('div');
      actions.className = 'train-list-actions';
      const deploy = document.createElement('button');
      deploy.className = 'train-list-button';
      deploy.type = 'button';
      deploy.textContent = 'Deploy';
      deploy.addEventListener('click', async () => {
        deploy.disabled = true;
        try {
          const payload = await loadJSON('hotword/train/deploy', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ model: model.file_name }),
          });
          deploymentStatusEl.textContent = `Deployed ${payload?.model?.file_name || model.file_name}.`;
          setBadge(deploymentBadgeEl, 'deployed');
          await refreshModels();
        } catch (err: any) {
          deploymentStatusEl.textContent = String(err?.message || err || 'deploy failed');
          setBadge(deploymentBadgeEl, 'error');
        } finally {
          deploy.disabled = false;
        }
      });
      actions.appendChild(deploy);
      item.appendChild(actions);
    }
    modelListEl.appendChild(item);
  }
}

async function refreshRecordings() {
  const payload = await loadJSON('hotword/train/recordings');
  const recordings = Array.isArray(payload?.recordings) ? payload.recordings as Recording[] : [];
  renderRecordings(recordings);
  recordingStatusEl.textContent = recordings.length > 0
    ? `${recordings.filter((item) => item.kind === 'hotword').length} hotword clip(s), ${recordings.filter((item) => item.kind === 'reference').length} reference clip(s).`
    : 'Record or upload a WAV file to start.';
}

async function refreshModels() {
  const payload = await loadJSON('hotword/train/models');
  renderModels(Array.isArray(payload?.models) ? payload.models as Model[] : []);
}

function mergeChunks() {
  const total = state.chunks.reduce((sum, chunk) => sum + chunk.length, 0);
  const merged = new Float32Array(total);
  let offset = 0;
  for (const chunk of state.chunks) {
    merged.set(chunk, offset);
    offset += chunk.length;
  }
  return merged;
}

async function uploadBlob(blob: Blob, kind: string) {
  const form = new FormData();
  form.append('kind', kind);
  form.append('file', blob, `${kind}-${Date.now()}.wav`);
  const resp = await fetch(apiURL('hotword/train/recordings'), {
    method: 'POST',
    body: form,
  });
  const payload = await resp.json().catch(() => ({}));
  if (!resp.ok) {
    throw new Error(String(payload?.error || `HTTP ${resp.status}`));
  }
  return payload;
}

async function startRecording() {
  if (state.recordingActive) return;
  const AudioContextCtor = window.AudioContext || (window as any).webkitAudioContext;
  if (!AudioContextCtor) {
    throw new Error('AudioContext is unavailable in this browser');
  }
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const audioContext = new AudioContextCtor();
  const sourceNode = audioContext.createMediaStreamSource(stream);
  const processorNode = audioContext.createScriptProcessor(4096, 1, 1);
  const sinkNode = audioContext.createGain();
  sinkNode.gain.value = 0;
  state.audioContext = audioContext;
  state.sourceNode = sourceNode;
  state.processorNode = processorNode;
  state.sinkNode = sinkNode;
  state.stream = stream;
  state.sampleRate = Number(audioContext.sampleRate) || 16000;
  state.chunks = [];
  processorNode.onaudioprocess = (event) => {
    const input = event.inputBuffer?.getChannelData?.(0);
    if (!(input instanceof Float32Array) || input.length === 0) return;
    const copy = new Float32Array(input.length);
    copy.set(input);
    state.chunks.push(copy);
  };
  sourceNode.connect(processorNode);
  processorNode.connect(sinkNode);
  sinkNode.connect(audioContext.destination);
  state.recordingActive = true;
  setBadge(recordingBadgeEl, 'recording');
  recordingToggleEl.textContent = 'Stop recording';
  recordingStatusEl.textContent = 'Recording... say the hotword or reference speech, then stop.';
}

async function stopRecording() {
  if (!state.recordingActive) return;
  const samples = mergeChunks();
  state.recordingActive = false;
  try {
    state.processorNode?.disconnect();
    state.sourceNode?.disconnect();
    state.sinkNode?.disconnect();
    state.stream?.getTracks().forEach((track) => track.stop());
    await state.audioContext?.close();
  } catch (_) {}
  state.processorNode = null;
  state.sourceNode = null;
  state.sinkNode = null;
  state.stream = null;
  state.audioContext = null;
  state.chunks = [];
  setBadge(recordingBadgeEl, 'uploading');
  recordingToggleEl.textContent = 'Start recording';
  if (samples.length === 0) {
    recordingStatusEl.textContent = 'No audio captured.';
    setBadge(recordingBadgeEl, 'idle');
    return;
  }
  const blob = float32ToWav(samples, state.sampleRate);
  await uploadBlob(blob, recordingKindEl.value);
  recordingStatusEl.textContent = 'Recording uploaded.';
  setBadge(recordingBadgeEl, 'saved');
  await refreshRecordings();
}

async function connectStatusStream(path: string, render: (status: StatusPayload) => void) {
  const source = new EventSource(apiURL(path));
  source.addEventListener('status', (event) => {
    try {
      render(JSON.parse((event as MessageEvent<string>).data));
    } catch (_) {}
  });
  source.onerror = () => {
    source.close();
    window.setTimeout(() => {
      void connectStatusStream(path, render);
    }, 1200);
  };
}

async function bootstrap() {
  recordingToggleEl.addEventListener('click', async () => {
    try {
      if (state.recordingActive) {
        await stopRecording();
      } else {
        await startRecording();
      }
    } catch (err: any) {
      setBadge(recordingBadgeEl, 'error');
      recordingStatusEl.textContent = String(err?.message || err || 'recording failed');
    }
  });

  recordingUploadEl.addEventListener('change', async () => {
    const file = recordingUploadEl.files?.[0];
    recordingUploadEl.value = '';
    if (!(file instanceof File)) return;
    try {
      setBadge(recordingBadgeEl, 'uploading');
      await uploadBlob(file, recordingKindEl.value);
      setBadge(recordingBadgeEl, 'saved');
      recordingStatusEl.textContent = `Uploaded ${file.name}.`;
      await refreshRecordings();
    } catch (err: any) {
      setBadge(recordingBadgeEl, 'error');
      recordingStatusEl.textContent = String(err?.message || err || 'upload failed');
    }
  });

  generationStartEl.addEventListener('click', async () => {
    const models = selectedGenerationModels();
    if (models.length === 0) {
      generationStatusEl.textContent = 'Select at least one generator.';
      return;
    }
    generationStartEl.disabled = true;
    try {
      const payload = await loadJSON('hotword/train/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          models,
          sample_count: Number(generationCountEl.value || 250),
        }),
      });
      renderGenerationStatus(payload?.status || {});
    } catch (err: any) {
      generationStatusEl.textContent = String(err?.message || err || 'generation failed');
      setBadge(generationBadgeEl, 'error');
    } finally {
      generationStartEl.disabled = false;
    }
  });

  trainingStartEl.addEventListener('click', async () => {
    trainingStartEl.disabled = true;
    try {
      const payload = await loadJSON('hotword/train/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      });
      const status = payload?.status || {};
      setBadge(trainingBadgeEl, status.state || 'running');
      trainingStatusEl.textContent = String(status.message || 'Training started.');
    } catch (err: any) {
      trainingStatusEl.textContent = String(err?.message || err || 'training failed');
      setBadge(trainingBadgeEl, 'error');
    } finally {
      trainingStartEl.disabled = false;
    }
  });

  try {
    const hotwordStatus = await loadJSON('hotword/status');
    if (hotwordStatus?.ready === false) {
      setBanner('Wake word assets are not fully deployed yet. Use this page to train and promote a model.');
    }
  } catch (_) {}

  await Promise.all([refreshRecordings(), refreshModels()]);
  void connectStatusStream('hotword/train/generate/status', renderGenerationStatus);
  void connectStatusStream('hotword/train/status', (status) => {
    setBadge(trainingBadgeEl, status.state || 'idle');
    trainingStatusEl.textContent = String(status.error || status.message || 'Idle.');
    if (status.latest_model) {
      void refreshModels();
    }
  });
}

void bootstrap();
