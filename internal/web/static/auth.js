import { showMain } from './app.js';

export function initAuth() {
  const form = document.getElementById('auth-form');
  const subtitle = document.getElementById('auth-subtitle');
  const submitBtn = document.getElementById('auth-submit');
  const errorEl = document.getElementById('auth-error');

  checkSetup(subtitle, submitBtn);

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    errorEl.textContent = '';
    const password = document.getElementById('auth-password').value;
    const hasPassword = submitBtn.dataset.mode === 'login';

    const url = hasPassword ? '/api/login' : '/api/setup';
    try {
      const resp = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password }),
      });
      if (!resp.ok) {
        const text = await resp.text();
        errorEl.textContent = text || 'Authentication failed';
        return;
      }
      showMain();
    } catch (err) {
      errorEl.textContent = 'Connection error: ' + err.message;
    }
  });
}

async function checkSetup(subtitle, submitBtn) {
  try {
    const resp = await fetch('/api/setup');
    const data = await resp.json();
    if (data.has_password) {
      subtitle.textContent = 'Enter your admin password';
      submitBtn.textContent = 'Login';
      submitBtn.dataset.mode = 'login';
    } else {
      subtitle.textContent = 'Set up your admin password (min 8 characters)';
      submitBtn.textContent = 'Set Password';
      submitBtn.dataset.mode = 'setup';
    }
  } catch (e) {
    subtitle.textContent = 'Server unreachable';
  }
}
