'use strict';

const $ = (id) => document.getElementById(id);
let pendingOtp = null;
let camStream = null;
let camTimer = null;

function native(payload) {
  return new Promise((resolve) => {
    chrome.runtime.sendMessage({ native: payload }, (resp) => resolve(resp || { ok: false, error: 'sin respuesta' }));
  });
}

function msg(text, isError = true) {
  const el = $('scan-msg');
  el.textContent = text;
  el.className = isError ? 'err' : 'muted';
  el.classList.remove('hidden');
}

// Al cargar: comprobar si la webcam está permitida (gate a nivel admin).
(async function init() {
  const caps = await native({ type: 'capabilities' });
  const webcamOk = caps.ok && caps.data && caps.data.otp_webcam_enabled;
  $('webcam-block').classList.toggle('hidden', !webcamOk);
  $('webcam-disabled-card').classList.toggle('hidden', !!webcamOk);
})();

// ---- entradas de OTP ----
$('manual-use').addEventListener('click', () => {
  const v = $('manual').value.trim();
  if (!v) return;
  // Aceptar otpauth:// o secreto suelto (se normaliza a otpauth).
  const otpauth = v.toLowerCase().startsWith('otpauth://')
    ? v
    : 'otpauth://totp/passtore?secret=' + v.replace(/\s+/g, '').toUpperCase();
  onDecoded(otpauth);
});

$('file').addEventListener('change', async (e) => {
  const f = e.target.files[0];
  if (!f) return;
  const url = URL.createObjectURL(f);
  const otpauth = await decodeImage(url);
  URL.revokeObjectURL(url);
  if (otpauth) onDecoded(otpauth);
  else msg('No encontré un QR de OTP en esa imagen.');
});

$('cam-start').addEventListener('click', startCamera);
$('cam-stop').addEventListener('click', stopCamera);

async function startCamera() {
  try {
    camStream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } });
    const video = $('video');
    video.srcObject = camStream;
    $('cam-start').classList.add('hidden');
    $('cam-stop').classList.remove('hidden');
    camTimer = setInterval(() => scanVideoFrame(video), 300);
  } catch (e) {
    msg('No pude acceder a la cámara: ' + e.message);
  }
}

function stopCamera() {
  if (camTimer) { clearInterval(camTimer); camTimer = null; }
  if (camStream) { camStream.getTracks().forEach((t) => t.stop()); camStream = null; }
  $('cam-start').classList.remove('hidden');
  $('cam-stop').classList.add('hidden');
}

function scanVideoFrame(video) {
  if (!video.videoWidth) return;
  const canvas = $('canvas');
  canvas.width = video.videoWidth;
  canvas.height = video.videoHeight;
  const ctx = canvas.getContext('2d');
  ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
  const data = ctx.getImageData(0, 0, canvas.width, canvas.height);
  const result = jsQR(data.data, canvas.width, canvas.height);
  if (result && result.data && result.data.toLowerCase().startsWith('otpauth://')) {
    stopCamera();
    onDecoded(result.data);
  }
}

function decodeImage(url) {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      const canvas = $('canvas');
      canvas.width = img.naturalWidth;
      canvas.height = img.naturalHeight;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(img, 0, 0);
      const data = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const result = jsQR(data.data, canvas.width, canvas.height);
      const text = result && result.data ? result.data : '';
      resolve(text.toLowerCase().startsWith('otpauth://') ? text : '');
    };
    img.onerror = () => resolve('');
    img.src = url;
  });
}

// ---- adjuntar / crear ----
async function onDecoded(otpauth) {
  pendingOtp = otpauth;
  $('scan-msg').classList.add('hidden');
  let label = '';
  try { const u = new URL(otpauth); label = u.searchParams.get('issuer') || decodeURIComponent(u.pathname.replace('/totp/', '')); } catch {}
  $('otp-label').textContent = label ? `Emisor/cuenta: ${label}` : '';

  const list = await native({ type: 'list' });
  const sel = $('entry-select');
  sel.innerHTML = '';
  if (list.ok && Array.isArray(list.data)) {
    for (const e of list.data) {
      const opt = document.createElement('option');
      opt.value = e.id;
      opt.textContent = e.title + (e.username ? ' — ' + e.username : '');
      sel.appendChild(opt);
    }
  }
  $('attach').classList.remove('hidden');
}

$('save-existing').addEventListener('click', async () => {
  const id = $('entry-select').value;
  if (!id || !pendingOtp) return;
  const g = await native({ type: 'get', entryId: id });
  if (!g.ok) return attachErr(g);
  const entry = g.data;
  entry.otpauth = pendingOtp;
  const r = await native({ type: 'update', entryId: id, entry });
  attachDone(r);
});

$('save-new').addEventListener('click', async () => {
  if (!pendingOtp) return;
  let title = 'OTP', username = '';
  try {
    const u = new URL(pendingOtp);
    const label = decodeURIComponent(u.pathname.replace('/totp/', ''));
    title = u.searchParams.get('issuer') || label.split(':')[0] || 'OTP';
    username = label.includes(':') ? label.split(':')[1] : '';
  } catch {}
  const r = await native({ type: 'add', entry: { type: 'login', title, username, otpauth: pendingOtp } });
  attachDone(r);
});

function attachErr(r) {
  if (r.status === 423) msg('El vault está bloqueado. Ábrelo desde el popup de Passtore.');
  else msg(r.error || 'Error');
}

function attachDone(r) {
  if (!r.ok) return attachErr(r);
  const m = $('attach-msg');
  m.textContent = '✔ OTP guardado en el vault. Ya puedes cerrar esta pestaña.';
  m.classList.remove('hidden');
  $('save-existing').disabled = true;
  $('save-new').disabled = true;
}
