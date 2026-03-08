(function () {
  const page = document.getElementById('capture-page');
  const noteInput = document.getElementById('capture-note');
  const recordButton = document.getElementById('capture-record');
  const recordLabel = recordButton ? recordButton.querySelector('.capture-record-label') : null;
  const recordHint = recordButton ? recordButton.querySelector('.capture-record-hint') : null;
  const saveButton = document.getElementById('capture-save');
  const resetButton = document.getElementById('capture-reset');
  const statusNode = document.getElementById('capture-status');

  if (!page || !noteInput || !recordButton || !recordLabel || !recordHint || !saveButton || !resetButton || !statusNode) {
    return;
  }

  const state = {
    statusTimer: 0,
    recording: false,
    saving: false,
    discardRecording: false,
    mediaStream: null,
    mediaRecorder: null,
    audioChunks: [],
    audioBlob: null,
  };

  function clearStatusTimer() {
    if (state.statusTimer) {
      window.clearTimeout(state.statusTimer);
      state.statusTimer = 0;
    }
  }

  function setStatus(message, tone) {
    statusNode.textContent = String(message || '');
    if (tone) {
      statusNode.dataset.tone = tone;
    } else {
      delete statusNode.dataset.tone;
    }
  }

  function scheduleStatusClear(delayMS) {
    clearStatusTimer();
    state.statusTimer = window.setTimeout(() => {
      setStatus('', '');
      state.statusTimer = 0;
    }, delayMS);
  }

  function setCaptureState(nextState) {
    const cleanState = String(nextState || 'idle').trim() || 'idle';
    document.body.dataset.captureState = cleanState;
    page.dataset.state = cleanState;
    recordButton.setAttribute('aria-pressed', cleanState === 'recording' ? 'true' : 'false');
    if (cleanState === 'recording') {
      recordLabel.textContent = 'Recording';
      recordHint.textContent = 'Tap again to stop.';
      return;
    }
    recordLabel.textContent = 'Record';
    recordHint.textContent = state.audioBlob ? 'Recording captured. Type a note or clear to reset.' : 'Tap once to start, again to stop.';
  }

  function normalizeNote(raw) {
    return String(raw || '').replace(/\s+/g, ' ').trim();
  }

  function deriveItemTitle(raw) {
    const clean = normalizeNote(raw);
    if (!clean) {
      return '';
    }
    const sentenceMatch = clean.match(/^.*?[.!?](?:\s|$)/);
    const firstSentence = normalizeNote(sentenceMatch ? sentenceMatch[0] : clean);
    if (firstSentence.length <= 80) {
      return firstSentence;
    }
    return `${firstSentence.slice(0, 77).trimEnd()}...`;
  }

  function updateSaveState() {
    const hasNote = normalizeNote(noteInput.value) !== '';
    saveButton.disabled = state.saving || !hasNote;
  }

  function releaseMediaStream() {
    if (!state.mediaStream || typeof state.mediaStream.getTracks !== 'function') {
      state.mediaStream = null;
      return;
    }
    for (const track of state.mediaStream.getTracks()) {
      if (track && typeof track.stop === 'function') {
        track.stop();
      }
    }
    state.mediaStream = null;
  }

  function finishRecording() {
    state.recording = false;
    state.mediaRecorder = null;
    releaseMediaStream();
    setCaptureState('idle');
    if (state.audioBlob) {
      setStatus('Recording captured.', 'success');
      scheduleStatusClear(1800);
    }
  }

  async function startRecording() {
    if (!navigator.mediaDevices || typeof navigator.mediaDevices.getUserMedia !== 'function') {
      setStatus('Voice capture is not available in this browser.', 'error');
      return;
    }
    if (typeof window.MediaRecorder !== 'function') {
      setStatus('MediaRecorder is unavailable in this browser.', 'error');
      return;
    }
    clearStatusTimer();
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const recorder = new window.MediaRecorder(stream);
      state.recording = true;
      state.mediaStream = stream;
      state.mediaRecorder = recorder;
      state.audioChunks = [];
      state.audioBlob = null;
      recorder.addEventListener('dataavailable', (event) => {
        if (event.data) {
          state.audioChunks.push(event.data);
        }
      });
      recorder.addEventListener('stop', () => {
        const mimeType = String(recorder.mimeType || 'audio/webm').trim() || 'audio/webm';
        if (!state.discardRecording && state.audioChunks.length > 0) {
          state.audioBlob = new Blob(state.audioChunks, { type: mimeType });
        }
        state.discardRecording = false;
        finishRecording();
      });
      recorder.start();
      setStatus('', '');
      setCaptureState('recording');
    } catch (error) {
      releaseMediaStream();
      state.recording = false;
      state.mediaRecorder = null;
      setCaptureState('idle');
      setStatus(`Voice capture failed: ${String(error && error.message ? error.message : error)}`, 'error');
    }
  }

  function stopRecording() {
    if (!state.recording || !state.mediaRecorder) {
      finishRecording();
      return;
    }
    if (state.mediaRecorder.state !== 'inactive') {
      state.mediaRecorder.stop();
      return;
    }
    finishRecording();
  }

  function resetCapture() {
    clearStatusTimer();
    if (state.mediaRecorder && state.mediaRecorder.state !== 'inactive') {
      state.discardRecording = true;
      state.mediaRecorder.stop();
    }
    state.recording = false;
    state.mediaRecorder = null;
    state.audioChunks = [];
    state.audioBlob = null;
    releaseMediaStream();
    noteInput.value = '';
    setCaptureState('idle');
    setStatus('', '');
    updateSaveState();
  }

  async function saveCapture() {
    const note = normalizeNote(noteInput.value);
    if (!note || state.saving) {
      updateSaveState();
      return;
    }
    const title = deriveItemTitle(note);
    if (!title) {
      updateSaveState();
      return;
    }
    state.saving = true;
    updateSaveState();
    setCaptureState('saving');
    setStatus('Saving...', '');
    try {
      const response = await fetch('./api/items', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          title,
        }),
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      noteInput.value = '';
      state.audioBlob = null;
      setCaptureState('idle');
      setStatus(`Saved: ${title}`, 'success');
      scheduleStatusClear(1800);
    } catch (error) {
      setCaptureState(state.recording ? 'recording' : 'idle');
      setStatus(`Save failed: ${String(error && error.message ? error.message : error)}`, 'error');
    } finally {
      state.saving = false;
      updateSaveState();
    }
  }

  recordButton.addEventListener('click', () => {
    if (state.recording) {
      stopRecording();
      return;
    }
    void startRecording();
  });
  noteInput.addEventListener('input', updateSaveState);
  saveButton.addEventListener('click', () => {
    void saveCapture();
  });
  resetButton.addEventListener('click', resetCapture);

  updateSaveState();
  setCaptureState('idle');
  window.__taburaCapture = {
    deriveItemTitle,
    resetCapture,
  };
})();
