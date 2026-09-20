'use strict';

const api = window.passtore;
const $ = (id) => document.getElementById(id);

let entries = [];      // listado actual (sin contraseñas)
let selectedId = null;
let otpTimer = null;   // refresco del código OTP en el detalle

function clearOtpTimer() {
  if (otpTimer) { clearInterval(otpTimer); otpTimer = null; }
}

// ---- Navegación entre vistas ----
function show(view) {
  clearOtpTimer(); // al cambiar de vista, detener el refresco de OTP
  for (const v of ['connect-view', 'unlock-view', 'main-view']) {
    $(v).classList.toggle('hidden', v !== view);
  }
}

function toast(msg) {
  const t = document.createElement('div');
  t.className = 'toast';
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), 1600);
}

// ---- Arranque: conectar y ver estado ----
async function boot() {
  show('connect-view');
  $('connect-error').classList.add('hidden');
  $('retry-btn').classList.add('hidden');
  const r = await api.connect();
  if (!r.ok) {
    $('connect-msg').classList.add('hidden');
    $('connect-error').textContent =
      'No pude conectar con el agente (passtore-agent). Asegúrate de que esté instalado. ' + r.error;
    $('connect-error').classList.remove('hidden');
    $('retry-btn').classList.remove('hidden');
    return;
  }
  routeByStatus(r.data);
}

function routeByStatus(status) {
  if (status && status.locked === false) {
    openMain();
  } else {
    $('vault-path').textContent = (status && status.vault_path) || '';
    show('unlock-view');
    $('master').focus();
  }
}

// ---- Desbloqueo ----
$('unlock-form').addEventListener('submit', async (e) => {
  e.preventDefault();
  $('unlock-error').classList.add('hidden');
  const master = $('master').value;
  const r = await api.unlock(master);
  $('master').value = '';
  if (!r.ok) {
    $('unlock-error').textContent =
      r.status === 401 ? 'Contraseña maestra incorrecta.' : r.error;
    $('unlock-error').classList.remove('hidden');
    return;
  }
  openMain();
});

$('retry-btn').addEventListener('click', boot);

// ---- Vista principal ----
async function openMain() {
  show('main-view');
  $('search').value = '';
  await refreshList('');
}

async function refreshList(q) {
  const r = q ? await api.search(q) : await api.list();
  if (!r.ok) return handleMaybeLocked(r);
  entries = r.data || [];
  renderList();
}

function renderList() {
  const ul = $('entry-list');
  ul.innerHTML = '';
  if (entries.length === 0) {
    ul.innerHTML = '<li class="muted">(sin entradas)</li>';
    return;
  }
  for (const e of entries) {
    const li = document.createElement('li');
    li.dataset.id = e.id;
    if (e.id === selectedId) li.classList.add('active');
    li.innerHTML = `<div class="title"></div><div class="sub"></div>`;
    li.querySelector('.title').textContent = e.title;
    li.querySelector('.sub').textContent =
      e.username || e.url || (e.type === 'note' ? 'Nota segura' : '');
    li.addEventListener('click', () => selectEntry(e.id));
    ul.appendChild(li);
  }
}

async function selectEntry(id) {
  selectedId = id;
  clearOtpTimer();
  renderList();
  const r = await api.get(id);
  if (!r.ok) return handleMaybeLocked(r);
  renderDetail(r.data);
}

function field(label, value, opts = {}) {
  const wrap = document.createElement('div');
  wrap.className = 'field';
  const lab = document.createElement('label');
  lab.textContent = label;
  const val = document.createElement('div');
  val.className = 'val';
  const code = document.createElement('code');
  code.textContent = opts.secret ? '••••••••' : value;
  val.appendChild(code);

  if (opts.secret) {
    const showBtn = document.createElement('button');
    showBtn.className = 'mini secondary';
    showBtn.textContent = 'Ver';
    let shown = false;
    showBtn.addEventListener('click', () => {
      shown = !shown;
      code.textContent = shown ? value : '••••••••';
      showBtn.textContent = shown ? 'Ocultar' : 'Ver';
    });
    val.appendChild(showBtn);
  }
  if (opts.copy) {
    const copyBtn = document.createElement('button');
    copyBtn.className = 'mini';
    copyBtn.textContent = 'Copiar';
    copyBtn.addEventListener('click', async () => {
      await api.copy(value, 20000); // se limpia del portapapeles a los 20s
      toast('Copiado');
    });
    val.appendChild(copyBtn);
  }
  wrap.appendChild(lab);
  wrap.appendChild(val);
  return wrap;
}

// otpField crea la fila de OTP con código, cuenta atrás y copiar; se refresca solo.
function otpField(id) {
  const wrap = document.createElement('div');
  wrap.className = 'field';
  const lab = document.createElement('label');
  lab.textContent = 'OTP (2FA)';
  const val = document.createElement('div');
  val.className = 'val';
  const code = document.createElement('code');
  code.textContent = '……';
  const count = document.createElement('span');
  count.className = 'otp-count muted';
  const copyBtn = document.createElement('button');
  copyBtn.className = 'mini';
  copyBtn.textContent = 'Copiar';

  let current = '';
  copyBtn.addEventListener('click', async () => {
    if (current) { await api.copy(current, 20000); toast('OTP copiado'); }
  });

  async function refresh() {
    const r = await api.otp(id);
    if (!r.ok) {
      if (r.status === 423) { clearOtpTimer(); handleMaybeLocked(r); }
      return;
    }
    current = r.data.code;
    code.textContent = current.replace(/(\d{3})(\d+)/, '$1 $2');
    count.textContent = `${r.data.expires_in}s`;
  }

  val.appendChild(code);
  val.appendChild(count);
  val.appendChild(copyBtn);
  wrap.appendChild(lab);
  wrap.appendChild(val);

  refresh();
  clearOtpTimer();
  otpTimer = setInterval(refresh, 1000);
  return wrap;
}

function renderDetail(e) {
  const d = $('detail');
  d.innerHTML = '';
  const h = document.createElement('h2');
  h.textContent = e.title;
  d.appendChild(h);

  if (e.username) d.appendChild(field('Usuario', e.username, { copy: true }));
  if (e.password) d.appendChild(field('Contraseña', e.password, { secret: true, copy: true }));
  if (e.url) d.appendChild(field('URL', e.url, { copy: true }));
  if (e.notes) d.appendChild(field(e.type === 'note' ? 'Contenido' : 'Notas', e.notes));
  if (e.tags && e.tags.length) d.appendChild(field('Tags', e.tags.join(', ')));
  if (e.otpauth) d.appendChild(otpField(e.id));

  const actions = document.createElement('div');
  actions.className = 'detail-actions';
  const edit = document.createElement('button');
  edit.textContent = 'Editar';
  edit.addEventListener('click', () => openModal(e));
  const del = document.createElement('button');
  del.className = 'secondary';
  del.textContent = 'Borrar';
  del.addEventListener('click', () => removeEntry(e));
  actions.appendChild(edit);
  actions.appendChild(del);
  d.appendChild(actions);
}

async function removeEntry(e) {
  if (!confirm(`¿Borrar la entrada "${e.title}"?`)) return;
  const r = await api.remove(e.id);
  if (!r.ok) return handleMaybeLocked(r);
  selectedId = null;
  $('detail').innerHTML = '<p class="muted">Selecciona una entrada.</p>';
  await refreshList($('search').value.trim());
  toast('Borrada');
}

// ---- Buscar ----
let searchTimer = null;
$('search').addEventListener('input', (e) => {
  clearTimeout(searchTimer);
  const q = e.target.value.trim();
  searchTimer = setTimeout(() => refreshList(q), 180);
});

// ---- Bloquear ----
$('lock-btn').addEventListener('click', async () => {
  await api.lock();
  selectedId = null;
  show('unlock-view');
  $('master').focus();
});

// ---- Modal crear/editar ----
// El botón Añadir despliega un menú con los tipos que se pueden crear.
$('add-btn').addEventListener('click', (e) => {
  e.stopPropagation();
  $('add-menu').classList.toggle('hidden');
});
document.addEventListener('click', () => $('add-menu').classList.add('hidden'));
for (const b of document.querySelectorAll('#add-menu button')) {
  b.addEventListener('click', () => {
    $('add-menu').classList.add('hidden');
    openModal(null, b.dataset.type);
  });
}
$('cancel-btn').addEventListener('click', closeModal);
$('gen-btn').addEventListener('click', async () => {
  const r = await api.generate({ Length: 20, Upper: true, Digits: true, Symbols: true });
  if (r.ok) $('f-password').value = r.data.password;
});

// openModal(e, type): e = entrada a editar (o null para crear).
// type = 'login' | 'note' (solo al crear; al editar se toma de la entrada).
function openModal(e, type) {
  const t = e ? (e.type || 'login') : (type || 'login');
  const isNote = t === 'note';
  $('f-type').value = t;

  // Mostrar/ocultar los campos exclusivos de credenciales.
  for (const el of document.querySelectorAll('.login-only')) el.classList.toggle('hidden', isNote);
  $('notes-label').textContent = isNote ? 'Contenido' : 'Notas';
  $('f-notes').rows = isNote ? 8 : 3;

  $('modal-error').classList.add('hidden');
  $('modal-title').textContent = e
    ? (isNote ? 'Editar nota segura' : 'Editar credencial')
    : (isNote ? 'Nueva nota segura' : 'Nueva credencial');
  $('f-id').value = e ? e.id : '';
  $('f-title').value = e ? e.title : '';
  $('f-username').value = e ? e.username || '' : '';
  $('f-password').value = e ? e.password || '' : '';
  $('f-url').value = e ? e.url || '' : '';
  $('f-notes').value = e ? e.notes || '' : '';
  $('f-tags').value = e && e.tags ? e.tags.join(', ') : '';
  $('f-otp').value = e ? e.otpauth || '' : '';
  $('modal').classList.remove('hidden');
  $('f-title').focus();
}
function closeModal() { $('modal').classList.add('hidden'); }

$('entry-form').addEventListener('submit', async (ev) => {
  ev.preventDefault();
  const id = $('f-id').value;
  const type = $('f-type').value || 'login';
  const entry = {
    type,
    title: $('f-title').value.trim(),
    notes: $('f-notes').value.trim(),
    tags: $('f-tags').value.split(',').map((s) => s.trim()).filter(Boolean),
  };
  if (type !== 'note') {
    entry.username = $('f-username').value.trim();
    entry.password = $('f-password').value;
    entry.url = $('f-url').value.trim();
    entry.otpauth = $('f-otp').value.trim();
  }
  const r = id ? await api.update(id, entry) : await api.add(entry);
  if (!r.ok) {
    $('modal-error').textContent = r.error;
    $('modal-error').classList.remove('hidden');
    return;
  }
  closeModal();
  const newId = id || (r.data && r.data.id);
  await refreshList($('search').value.trim());
  if (newId) selectEntry(newId);
  toast(id ? 'Actualizada' : 'Añadida');
});

// Si el auto-lock del agente saltó, volvemos a la pantalla de desbloqueo.
function handleMaybeLocked(r) {
  if (r.status === 423) {
    show('unlock-view');
    $('master').focus();
  } else {
    alert(r.error || 'Error');
  }
}

boot();
