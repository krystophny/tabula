import { Terminal } from './vendor/xterm.mjs';
import { FitAddon } from './vendor/addon-fit.mjs';

let terminal = null;
let fitAddon = null;

export function initTerminal(container) {
  if (terminal) {
    terminal.dispose();
  }

  terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: "'SF Mono', 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace",
    theme: {
      background: '#1a1a2e',
      foreground: '#e0e0e0',
      cursor: '#53a8b6',
      selectionBackground: '#53a8b644',
      black: '#1a1a2e',
      red: '#e94560',
      green: '#4ecca3',
      yellow: '#f0a500',
      blue: '#53a8b6',
      magenta: '#c084fc',
      cyan: '#79c7d4',
      white: '#e0e0e0',
    },
    allowProposedApi: true,
  });

  fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  terminal.open(container);
  fitAddon.fit();

  window._tabulaTerminal = terminal;

  const resizeObserver = new ResizeObserver(() => {
    if (fitAddon) fitAddon.fit();
  });
  resizeObserver.observe(container);

  return terminal;
}

export function destroyTerminal() {
  if (terminal) {
    terminal.dispose();
    terminal = null;
    fitAddon = null;
    window._tabulaTerminal = null;
  }
  const container = document.getElementById('terminal-container');
  if (container) container.innerHTML = '';
}

export function writeToTerminal(data) {
  if (terminal) terminal.write(data);
}

