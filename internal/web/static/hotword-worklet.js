class HotwordProcessor extends AudioWorkletProcessor {
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (ch && ch.length > 0) {
      this.port.postMessage({ samples: new Float32Array(ch), sampleRate: sampleRate });
    }
    return true;
  }
}
registerProcessor('hotword-processor', HotwordProcessor);
