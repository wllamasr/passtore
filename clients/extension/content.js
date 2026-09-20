// content.js — se inyecta en las páginas. Hace dos cosas:
//  1) Rellenar formularios cuando el popup lo pide.
//  2) Detectar envíos de login para ofrecer guardar en Passtore.

'use strict';

// --- utilidades para localizar campos ---
function findPasswordField() {
  return document.querySelector('input[type="password"]');
}

function findUsernameField(pwField) {
  // Heurística: campo de texto/email/tel visible antes del password, o por nombre.
  const candidates = Array.from(
    document.querySelectorAll('input[type="text"], input[type="email"], input[type="tel"], input:not([type])')
  ).filter((el) => el.offsetParent !== null);
  if (pwField && pwField.form) {
    const inForm = candidates.filter((el) => el.form === pwField.form);
    if (inForm.length) return inForm[inForm.length - 1];
  }
  const byName = candidates.find((el) => /user|email|login|correo|usuario/i.test(el.name + ' ' + el.id));
  return byName || candidates[0] || null;
}

function setValue(el, value) {
  if (!el) return;
  const proto = Object.getPrototypeOf(el);
  const setter = Object.getOwnPropertyDescriptor(proto, 'value');
  if (setter && setter.set) setter.set.call(el, value);
  else el.value = value;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

function fillCredentials(username, password) {
  const pw = findPasswordField();
  const user = findUsernameField(pw);
  if (user && username) setValue(user, username);
  if (pw && password) setValue(pw, password);
  return { filled: !!(user || pw) };
}

// findOtpField localiza el campo de código OTP de la página.
function findOtpField() {
  return (
    document.querySelector('input[autocomplete="one-time-code"]') ||
    Array.from(document.querySelectorAll('input[type="text"], input[type="tel"], input[type="number"], input:not([type])'))
      .filter((el) => el.offsetParent !== null)
      .find((el) => /otp|one[-_]?time|2fa|mfa|totp|token|code|c[oó]digo|verif/i.test(el.name + ' ' + el.id + ' ' + (el.getAttribute('aria-label') || '') + ' ' + (el.placeholder || ''))) ||
    null
  );
}

function fillOtp(code) {
  const el = findOtpField();
  if (el) setValue(el, code);
  return { filled: !!el };
}

// --- mensajes desde el popup ---
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.action === 'fill') {
    sendResponse(fillCredentials(msg.username, msg.password));
    return true;
  }
  if (msg && msg.action === 'fillOtp') {
    sendResponse(fillOtp(msg.code));
    return true;
  }
  if (msg && msg.action === 'readForm') {
    const pw = findPasswordField();
    const user = findUsernameField(pw);
    sendResponse({ username: user ? user.value : '', password: pw ? pw.value : '' });
    return true;
  }
  return false;
});

// --- detección de guardado ---
let lastCaptured = null;

function captureFromForm(form) {
  const pw = form.querySelector('input[type="password"]');
  if (!pw || !pw.value) return;
  const user = findUsernameField(pw);
  lastCaptured = {
    url: location.origin,
    username: user ? user.value : '',
    password: pw.value,
  };
}

document.addEventListener(
  'submit',
  (e) => {
    if (e.target && e.target.querySelector) captureFromForm(e.target);
    if (lastCaptured) maybeOfferSave(lastCaptured);
  },
  true
);

// También captura clicks en botones de tipo submit (SPAs que no disparan submit).
document.addEventListener(
  'click',
  (e) => {
    const btn = e.target.closest('button, input[type="submit"]');
    if (!btn) return;
    const pw = findPasswordField();
    if (pw && pw.value) {
      const form = pw.form || document;
      captureFromForm(form.querySelector ? form : document.body);
      if (lastCaptured) setTimeout(() => maybeOfferSave(lastCaptured), 300);
    }
  },
  true
);

// --- banner de guardado ---
async function maybeOfferSave(cred) {
  // ¿Ya existe una entrada para este dominio con este usuario? Preguntamos al agente.
  const host = new URL(cred.url).host;
  const res = await sendNative({ type: 'search', query: host });
  if (res && res.ok && Array.isArray(res.data)) {
    const exists = res.data.some(
      (e) => (e.username || '') === cred.username && (e.url || '').includes(host)
    );
    if (exists) return; // no molestar si ya está
  }
  showBanner(cred);
}

function showBanner(cred) {
  if (document.getElementById('passtore-banner')) return;
  const bar = document.createElement('div');
  bar.id = 'passtore-banner';
  bar.style.cssText =
    'position:fixed;top:14px;right:14px;z-index:2147483647;background:#262832;color:#e6e7ee;' +
    'border:1px solid #3a3d4d;border-radius:10px;padding:12px 14px;font:14px system-ui,sans-serif;' +
    'box-shadow:0 6px 24px rgba(0,0,0,.4);max-width:300px;';
  const text = document.createElement('div');
  text.textContent = `¿Guardar esta contraseña de ${new URL(cred.url).host} en Passtore?`;
  text.style.marginBottom = '10px';
  const save = document.createElement('button');
  save.textContent = 'Guardar';
  save.style.cssText = 'background:#6d8bff;color:#fff;border:0;border-radius:6px;padding:6px 12px;margin-right:8px;cursor:pointer;';
  const dismiss = document.createElement('button');
  dismiss.textContent = 'Ahora no';
  dismiss.style.cssText = 'background:transparent;color:#9aa0b5;border:0;cursor:pointer;';

  save.addEventListener('click', async () => {
    save.disabled = true;
    save.textContent = 'Guardando…';
    const entry = {
      type: 'login',
      title: new URL(cred.url).host,
      username: cred.username,
      password: cred.password,
      url: cred.url,
    };
    const r = await sendNative({ type: 'add', entry });
    text.textContent = r && r.ok ? '✔ Guardado en Passtore' : 'Error: ' + ((r && r.error) || 'desconocido');
    save.remove();
    dismiss.textContent = 'Cerrar';
    if (r && r.ok) setTimeout(() => bar.remove(), 1500);
  });
  dismiss.addEventListener('click', () => bar.remove());

  bar.appendChild(text);
  bar.appendChild(save);
  bar.appendChild(dismiss);
  document.body.appendChild(bar);
  setTimeout(() => bar.remove(), 15000);
}

// sendNative pasa por el background (los content scripts no pueden hablar
// directamente con native messaging).
function sendNative(payload) {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ native: payload }, (resp) => resolve(resp));
  });
}
