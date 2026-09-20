'use strict';

const $ = (id) => document.getElementById(id);
let currentHost = '';
let otpTimers = [];       // intervalos de refresco de OTP en la lista
let pendingOtp = null;    // otpauth pendiente de adjuntar (modo "attach")

function show(view) {
  clearOtpTimers();
  for (const v of ['view-loading', 'view-error', 'view-unlock', 'view-main']) {
    $(v).classList.toggle('hidden', v !== view);
  }
}

function clearOtpTimers() {
  otpTimers.forEach((t) => clearInterval(t));
  otpTimers = [];
}

// native envía un mensaje al host nativo a través del background.
function native(payload) {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ native: payload }, (resp) => resolve(resp || { ok: false, error: 'sin respuesta' }));
  });
}

async function activeTab() {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  return tab || null;
}

async function boot() {
  show('view-loading');
  const tab = await activeTab();
  try { currentHost = tab && tab.url ? new URL(tab.url).host : ''; } catch { currentHost = ''; }

  const st = await native({ type: 'status' });
  if (!st.ok) {
    $('error-msg').textContent =
      'No pude contactar con el agente. ¿Está instalado el host nativo y creado el vault? ' + (st.error || '');
    show('view-error');
    return;
  }
  if (st.data && st.data.locked === false) openMain();
  else { show('view-unlock'); $('master').focus(); }
}

$('retry').addEventListener('click', boot);

$('view-unlock').addEventListener('submit', async (e) => {
  e.preventDefault();
  $('unlock-err').classList.add('hidden');
  const r = await native({ type: 'unlock', master: $('master').value });
  $('master').value = '';
  if (!r.ok) {
    $('unlock-err').textContent = r.status === 401 ? 'Contraseña maestra incorrecta.' : r.error;
    $('unlock-err').classList.remove('hidden');
    return;
  }
  openMain();
});

$('lock').addEventListener('click', async () => {
  await native({ type: 'lock' });
  show('view-unlock');
  $('master').focus();
});

let searchTimer = null;
$('search').addEventListener('input', (e) => {
  clearTimeout(searchTimer);
  const q = e.target.value.trim();
  searchTimer = setTimeout(() => loadList(q), 180);
});

$('scan-tab').addEventListener('click', scanCurrentTab);
$('open-scan').addEventListener('click', () => {
  chrome.tabs.create({ url: chrome.runtime.getURL('scan.html') });
});

async function openMain() {
  show('view-main');
  $('search').value = '';
  await loadList(currentHost || '');
}

function updateAttachHint() {
  const hint = $('attach-hint');
  if (pendingOtp) {
    let label = '';
    try { const u = new URL(pendingOtp); label = u.searchParams.get('issuer') || decodeURIComponent(u.pathname.replace('/totp/', '')); } catch {}
    hint.textContent = `OTP detectado${label ? ' (' + label + ')' : ''}: toca una entrada para adjuntarlo o crea una nueva.`;
    hint.classList.remove('hidden');
  } else {
    hint.classList.add('hidden');
  }
}

async function loadList(q) {
  clearOtpTimers();
  const r = q ? await native({ type: 'search', query: q }) : await native({ type: 'list' });
  const ul = $('list');
  ul.innerHTML = '';
  updateAttachHint();

  if (!r.ok) {
    if (r.status === 423) { show('view-unlock'); $('master').focus(); return; }
    addEmpty(ul, r.error || 'Error');
    return;
  }
  const entries = r.data || [];
  if (pendingOtp) {
    const li = document.createElement('li');
    li.className = 'newotp';
    li.textContent = '＋ Crear entrada nueva para este OTP';
    li.addEventListener('click', createEntryForOtp);
    ul.appendChild(li);
  }
  if (entries.length === 0) { addEmpty(ul, 'Sin coincidencias.'); return; }

  for (const e of entries) ul.appendChild(renderItem(e));
}

function addEmpty(ul, text) {
  const li = document.createElement('li');
  li.className = 'row empty';
  li.textContent = text;
  ul.appendChild(li);
}

const FILL_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/><path d="M10 17l5-5-5-5"/><path d="M15 12H3"/></svg>';

function renderItem(e) {
  const li = document.createElement('li');
  li.className = 'row';

  const avatar = document.createElement('div');
  avatar.className = 'avatar';
  avatar.textContent = (e.title || '?').trim().charAt(0) || '?';

  const info = document.createElement('div');
  info.className = 'info';
  info.innerHTML = '<div class="title"></div><div class="sub"></div>';
  info.querySelector('.title').textContent = e.title;
  info.querySelector('.sub').textContent = e.username || e.url || '';

  const actions = document.createElement('div');
  actions.className = 'actions';

  if (pendingOtp) {
    // Modo adjuntar: al pulsar la entrada, se le adjunta el OTP.
    li.classList.add('attachable');
    li.addEventListener('click', () => attachOtp(e.id));
    const b = document.createElement('button');
    b.className = 'mini';
    b.textContent = 'Adjuntar';
    b.addEventListener('click', (ev) => { ev.stopPropagation(); attachOtp(e.id); });
    actions.appendChild(b);
  } else {
    if (e.has_otp) actions.appendChild(otpChip(e.id));
    const fill = document.createElement('button');
    fill.className = 'fillbtn';
    fill.title = 'Rellenar usuario y contraseña';
    fill.innerHTML = FILL_ICON;
    fill.addEventListener('click', (ev) => { ev.stopPropagation(); fillEntry(e.id); });
    actions.appendChild(fill);
    li.addEventListener('click', () => fillEntry(e.id));
  }

  li.appendChild(avatar);
  li.appendChild(info);
  li.appendChild(actions);
  return li;
}

// otpChip muestra el código OTP con cuenta atrás y lo autorellena/copia al pulsar.
function otpChip(id) {
  const chip = document.createElement('button');
  chip.className = 'otpchip';
  chip.title = 'Autorellenar y copiar OTP';
  let code = '';
  const refresh = async () => {
    const r = await native({ type: 'otp', entryId: id });
    if (r.ok && r.data) {
      code = r.data.code;
      chip.textContent = `${code} · ${r.data.expires_in}s`;
    }
  };
  chip.addEventListener('click', async (ev) => {
    ev.stopPropagation();
    if (!code) await refresh();
    const tab = await activeTab();
    if (tab) chrome.tabs.sendMessage(tab.id, { action: 'fillOtp', code }, () => {});
    await navigator.clipboard.writeText(code).catch(() => {});
    window.close();
  });
  refresh();
  otpTimers.push(setInterval(refresh, 1000));
  return chip;
}

async function fillEntry(id) {
  const r = await native({ type: 'get', entryId: id });
  if (!r.ok) { if (r.status === 423) { show('view-unlock'); $('master').focus(); } return; }
  const tab = await activeTab();
  if (!tab) return;
  chrome.tabs.sendMessage(tab.id, { action: 'fill', username: r.data.username, password: r.data.password }, () => window.close());
}

// ---- flujo de guardado de OTP ----

async function scanCurrentTab() {
  try {
    const dataUrl = await chrome.tabs.captureVisibleTab({ format: 'png' });
    const otpauth = await decodeQrDataUrl(dataUrl);
    if (!otpauth) { $('attach-hint').textContent = 'No encontré un QR de OTP en la pantalla visible.'; $('attach-hint').classList.remove('hidden'); return; }
    pendingOtp = otpauth;
    await loadList('');
  } catch (e) {
    $('attach-hint').textContent = 'No pude capturar la pestaña: ' + e.message;
    $('attach-hint').classList.remove('hidden');
  }
}

// decodeQrDataUrl decodifica un QR de una imagen (dataURL) con jsQR.
function decodeQrDataUrl(dataUrl) {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      const canvas = document.createElement('canvas');
      canvas.width = img.naturalWidth;
      canvas.height = img.naturalHeight;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(img, 0, 0);
      const imgData = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const result = jsQR(imgData.data, canvas.width, canvas.height);
      const text = result && result.data ? result.data : '';
      resolve(text.toLowerCase().startsWith('otpauth://') ? text : '');
    };
    img.onerror = () => resolve('');
    img.src = dataUrl;
  });
}

async function attachOtp(id) {
  if (!pendingOtp) return;
  const g = await native({ type: 'get', entryId: id });
  if (!g.ok) { if (g.status === 423) { show('view-unlock'); $('master').focus(); } return; }
  const entry = g.data;
  entry.otpauth = pendingOtp;
  const r = await native({ type: 'update', entryId: id, entry });
  finishAttach(r);
}

async function createEntryForOtp() {
  if (!pendingOtp) return;
  let title = 'OTP', username = '';
  try {
    const u = new URL(pendingOtp);
    const label = decodeURIComponent(u.pathname.replace('/totp/', ''));
    title = u.searchParams.get('issuer') || label.split(':')[0] || 'OTP';
    username = label.includes(':') ? label.split(':')[1] : '';
  } catch {}
  const r = await native({ type: 'add', entry: { type: 'login', title, username, otpauth: pendingOtp } });
  finishAttach(r);
}

function finishAttach(r) {
  if (!r.ok) {
    $('attach-hint').textContent = 'Error guardando el OTP: ' + (r.error || '');
    $('attach-hint').classList.remove('hidden');
    return;
  }
  pendingOtp = null;
  loadList($('search').value.trim() || currentHost || '');
}

boot();
