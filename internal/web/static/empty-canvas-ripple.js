const MAX_IMPULSES = 14;

const VERTEX_SHADER_SOURCE = `
attribute vec2 a_position;
varying vec2 v_uv;

void main() {
  v_uv = (a_position * 0.5) + 0.5;
  gl_Position = vec4(a_position, 0.0, 1.0);
}
`;

const FRAGMENT_SHADER_SOURCE = `
precision highp float;

const int MAX_IMPULSES = ${MAX_IMPULSES};

uniform vec2 u_resolution;
uniform float u_time;
uniform float u_energy;
uniform vec4 u_impulses[MAX_IMPULSES];

varying vec2 v_uv;

float impulseHeight(vec2 uv, vec4 impulse) {
  float age = u_time - impulse.z;
  if (age < 0.0 || age > 8.0 || impulse.w <= 0.0) {
    return 0.0;
  }

  vec2 delta = uv - impulse.xy;
  float dist = length(delta);
  float envelope = exp(-dist * 14.0) * exp(-age * 1.35);
  float phase = dist * 72.0 - age * 18.0;
  float ring = sin(phase);
  float trailing = 0.45 * sin((dist * 48.0) - (age * 11.0));
  return impulse.w * envelope * (ring + trailing);
}

float heightAt(vec2 uv) {
  float h = 0.0;
  for (int i = 0; i < MAX_IMPULSES; i += 1) {
    h += impulseHeight(uv, u_impulses[i]);
  }
  return h;
}

void main() {
  vec2 px = 1.0 / max(u_resolution, vec2(1.0, 1.0));
  float h = heightAt(v_uv);
  float hx = heightAt(v_uv + vec2(px.x, 0.0));
  float hy = heightAt(v_uv + vec2(0.0, px.y));

  vec3 n = normalize(vec3((h - hx) * 7.0, (h - hy) * 7.0, 1.0));
  vec3 light = normalize(vec3(-0.35, -0.55, 1.0));
  vec3 viewDir = vec3(0.0, 0.0, 1.0);
  vec3 reflected = reflect(-light, n);

  float fresnel = pow(1.0 - max(dot(n, viewDir), 0.0), 2.8);
  float rim = (1.0 - n.z);
  float spec = pow(max(dot(reflected, viewDir), 0.0), 68.0);
  float diffuse = max(dot(n, light), 0.0);

  float waveInk = abs(h) * 0.55 + rim * 0.7 + fresnel * 0.45 + spec * 0.55 + (1.0 - diffuse) * 0.2;
  waveInk *= (0.6 + (u_energy * 0.65));
  float ink = clamp(waveInk, 0.0, 0.92);

  float white = 1.0 - ink;
  gl_FragColor = vec4(vec3(white), 1.0);
}
`;

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function createShader(gl, type, source) {
  const shader = gl.createShader(type);
  if (!shader) return null;
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const info = gl.getShaderInfoLog(shader) || 'unknown shader compile error';
    gl.deleteShader(shader);
    throw new Error(info);
  }
  return shader;
}

function createProgram(gl, vertexSource, fragmentSource) {
  const vertexShader = createShader(gl, gl.VERTEX_SHADER, vertexSource);
  const fragmentShader = createShader(gl, gl.FRAGMENT_SHADER, fragmentSource);
  if (!vertexShader || !fragmentShader) {
    if (vertexShader) gl.deleteShader(vertexShader);
    if (fragmentShader) gl.deleteShader(fragmentShader);
    return null;
  }

  const program = gl.createProgram();
  if (!program) {
    gl.deleteShader(vertexShader);
    gl.deleteShader(fragmentShader);
    return null;
  }
  gl.attachShader(program, vertexShader);
  gl.attachShader(program, fragmentShader);
  gl.linkProgram(program);
  gl.deleteShader(vertexShader);
  gl.deleteShader(fragmentShader);

  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    const info = gl.getProgramInfoLog(program) || 'unknown link error';
    gl.deleteProgram(program);
    throw new Error(info);
  }
  return program;
}

function createFallbackController() {
  return {
    setEnabled() {},
    addImpulse() {},
    destroy() {},
  };
}

class EmptyCanvasRippleController {
  constructor(viewportEl) {
    this.viewportEl = viewportEl;
    this.canvas = document.createElement('canvas');
    this.canvas.className = 'empty-canvas-ripple';
    this.canvas.setAttribute('aria-hidden', 'true');
    this.canvas.width = 1;
    this.canvas.height = 1;
    viewportEl.appendChild(this.canvas);

    this.gl = this.canvas.getContext('webgl', {
      alpha: false,
      antialias: true,
      depth: false,
      stencil: false,
      premultipliedAlpha: false,
      preserveDrawingBuffer: false,
      powerPreference: 'high-performance',
    });

    if (!this.gl) {
      throw new Error('WebGL unavailable');
    }

    this.program = createProgram(this.gl, VERTEX_SHADER_SOURCE, FRAGMENT_SHADER_SOURCE);
    if (!this.program) {
      throw new Error('Unable to create ripple shader program');
    }

    const gl = this.gl;
    gl.useProgram(this.program);

    this.positionLocation = gl.getAttribLocation(this.program, 'a_position');
    this.resolutionLocation = gl.getUniformLocation(this.program, 'u_resolution');
    this.timeLocation = gl.getUniformLocation(this.program, 'u_time');
    this.energyLocation = gl.getUniformLocation(this.program, 'u_energy');
    this.impulsesLocation = gl.getUniformLocation(this.program, 'u_impulses[0]');

    this.buffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.buffer);
    gl.bufferData(
      gl.ARRAY_BUFFER,
      new Float32Array([
        -1.0, -1.0,
        3.0, -1.0,
        -1.0, 3.0,
      ]),
      gl.STATIC_DRAW,
    );
    gl.enableVertexAttribArray(this.positionLocation);
    gl.vertexAttribPointer(this.positionLocation, 2, gl.FLOAT, false, 0, 0);

    this.impulses = new Float32Array(MAX_IMPULSES * 4);
    for (let i = 0; i < MAX_IMPULSES; i += 1) {
      const base = i * 4;
      this.impulses[base + 0] = 0.5;
      this.impulses[base + 1] = 0.5;
      this.impulses[base + 2] = -9999.0;
      this.impulses[base + 3] = 0.0;
    }
    this.impulseCursor = 0;

    this.energy = 0;
    this.enabled = false;
    this.destroyed = false;
    this.startAt = performance.now() * 0.001;
    this.rafId = 0;
    this.lastFrameAt = this.startAt;
    this.render = this.render.bind(this);

    this.resizeObserver = null;
    if (typeof ResizeObserver === 'function') {
      this.resizeObserver = new ResizeObserver(() => this.resize());
      this.resizeObserver.observe(viewportEl);
    } else {
      this.onWindowResize = () => this.resize();
      window.addEventListener('resize', this.onWindowResize, { passive: true });
    }

    this.canvas.addEventListener('webglcontextlost', (event) => {
      event.preventDefault();
      this.stop();
    });
    this.canvas.addEventListener('webglcontextrestored', () => {
      this.resize();
      this.start();
    });

    this.resize();
    this.clearToWhite();
  }

  destroy() {
    if (this.destroyed) return;
    this.destroyed = true;
    this.stop();
    if (this.resizeObserver) {
      this.resizeObserver.disconnect();
      this.resizeObserver = null;
    }
    if (this.onWindowResize) {
      window.removeEventListener('resize', this.onWindowResize);
      this.onWindowResize = null;
    }
    if (this.program && this.gl) {
      this.gl.deleteProgram(this.program);
    }
    if (this.buffer && this.gl) {
      this.gl.deleteBuffer(this.buffer);
    }
    this.program = null;
    this.buffer = null;
    this.gl = null;
    if (this.canvas?.parentElement) {
      this.canvas.parentElement.removeChild(this.canvas);
    }
  }

  setEnabled(enabled) {
    const shouldEnable = Boolean(enabled);
    if (this.enabled === shouldEnable) return;
    this.enabled = shouldEnable;
    if (shouldEnable) {
      this.canvas.classList.add('is-visible');
      this.start();
      this.energy = Math.max(this.energy, 0.2);
      this.addImpulseAtNormalized(0.5, 0.5, 0.38);
    } else {
      this.canvas.classList.remove('is-visible');
      this.stop();
      this.clearToWhite();
    }
  }

  addImpulse(clientX, clientY, magnitude = 1) {
    if (!this.enabled || this.destroyed) return;
    const rect = this.canvas.getBoundingClientRect();
    if (!rect.width || !rect.height) return;

    const x = (clientX - rect.left) / rect.width;
    const y = (clientY - rect.top) / rect.height;
    if (x < 0 || x > 1 || y < 0 || y > 1) return;
    this.addImpulseAtNormalized(x, y, magnitude);
  }

  addImpulseAtNormalized(normX, normY, magnitude = 1) {
    if (!this.enabled || this.destroyed) return;
    const base = this.impulseCursor * 4;
    const now = performance.now() * 0.001;
    this.impulses[base + 0] = clamp(normX, 0.0, 1.0);
    this.impulses[base + 1] = clamp(1.0 - normY, 0.0, 1.0);
    this.impulses[base + 2] = now - this.startAt;
    this.impulses[base + 3] = clamp(magnitude, 0.08, 1.3);
    this.impulseCursor = (this.impulseCursor + 1) % MAX_IMPULSES;
    this.energy = Math.min(1.6, this.energy + clamp(magnitude, 0.05, 0.8) * 0.38);
    if (!this.rafId) {
      this.start();
    }
  }

  resize() {
    if (this.destroyed || !this.canvas) return;
    const dpr = clamp(window.devicePixelRatio || 1, 1, 2);
    const width = Math.max(1, Math.round(this.viewportEl.clientWidth * dpr));
    const height = Math.max(1, Math.round(this.viewportEl.clientHeight * dpr));
    if (this.canvas.width !== width || this.canvas.height !== height) {
      this.canvas.width = width;
      this.canvas.height = height;
    }
    if (this.gl) {
      this.gl.viewport(0, 0, width, height);
    }
  }

  start() {
    if (this.destroyed || !this.enabled || this.rafId) return;
    this.lastFrameAt = performance.now() * 0.001;
    this.rafId = window.requestAnimationFrame(this.render);
  }

  stop() {
    if (!this.rafId) return;
    window.cancelAnimationFrame(this.rafId);
    this.rafId = 0;
  }

  clearToWhite() {
    if (!this.gl) return;
    this.gl.viewport(0, 0, this.canvas.width, this.canvas.height);
    this.gl.clearColor(1, 1, 1, 1);
    this.gl.clear(this.gl.COLOR_BUFFER_BIT);
  }

  render(nowMs) {
    this.rafId = 0;
    if (this.destroyed || !this.enabled || !this.gl || !this.program) return;

    const now = nowMs * 0.001;
    const dt = clamp(now - this.lastFrameAt, 0.0, 0.16);
    this.lastFrameAt = now;
    this.energy = Math.max(0.0, this.energy - dt * 0.26);

    this.resize();

    const gl = this.gl;
    gl.useProgram(this.program);
    gl.uniform2f(this.resolutionLocation, this.canvas.width, this.canvas.height);
    gl.uniform1f(this.timeLocation, now - this.startAt);
    gl.uniform1f(this.energyLocation, this.energy);
    gl.uniform4fv(this.impulsesLocation, this.impulses);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    const shouldContinue = this.enabled && (this.energy > 0.01 || this.hasRecentImpulse(now - this.startAt));
    if (shouldContinue) {
      this.rafId = window.requestAnimationFrame(this.render);
    }
  }

  hasRecentImpulse(nowSeconds) {
    for (let i = 0; i < MAX_IMPULSES; i += 1) {
      const base = i * 4;
      if (nowSeconds - this.impulses[base + 2] < 2.5) {
        return true;
      }
    }
    return false;
  }
}

export function createEmptyCanvasRipple(viewportEl) {
  if (!(viewportEl instanceof HTMLElement)) {
    return createFallbackController();
  }
  try {
    return new EmptyCanvasRippleController(viewportEl);
  } catch (err) {
    console.debug('empty-canvas-ripple disabled:', err);
    return createFallbackController();
  }
}
