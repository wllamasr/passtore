// agentClient.js — cliente del agente local de passtore.
// Node puro (sin Electron): descubre el agente vía agent.json, opcionalmente lo
// arranca, y expone métodos sobre su API HTTP loopback autenticada por token.

'use strict';

const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');

// configDir replica os.UserConfigDir de Go para localizar agent.json.
function configDir() {
  if (process.platform === 'win32') {
    return process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming');
  }
  if (process.platform === 'darwin') {
    return path.join(os.homedir(), 'Library', 'Application Support');
  }
  return process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config');
}

function runtimePath() {
  return path.join(configDir(), 'passtore', 'agent.json');
}

function readRuntime() {
  const raw = fs.readFileSync(runtimePath(), 'utf8');
  return JSON.parse(raw);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

class AgentError extends Error {
  constructor(message, status) {
    super(message);
    this.name = 'AgentError';
    this.status = status;
  }
}

class AgentClient {
  constructor() {
    this.info = null; // { port, token, pid }
  }

  baseURL() {
    return `http://127.0.0.1:${this.info.port}`;
  }

  // request hace una llamada autenticada al agente.
  async request(method, endpoint, body) {
    if (!this.info) throw new AgentError('agente no conectado', 0);
    const res = await fetch(this.baseURL() + endpoint, {
      method,
      headers: {
        'X-Passtore-Token': this.info.token,
        'Content-Type': 'application/json',
      },
      body: body != null ? JSON.stringify(body) : undefined,
    });
    if (res.status === 204) return null;
    const text = await res.text();
    const data = text ? JSON.parse(text) : null;
    if (!res.ok) {
      throw new AgentError((data && data.error) || `HTTP ${res.status}`, res.status);
    }
    return data;
  }

  // probe intenta leer runtime y consultar /status; lanza si el agente no responde.
  async probe() {
    this.info = readRuntime();
    return this.request('GET', '/status');
  }

  // connect intenta conectar; si no hay agente vivo y se da binPath, lo arranca.
  async connect({ binPath, vaultPath, autostart = true } = {}) {
    try {
      const st = await this.probe();
      return st;
    } catch (_) {
      this.info = null;
      if (!autostart) throw new AgentError('el agente no está corriendo', 0);
    }
    // Arrancar el agente.
    const bin = binPath || process.env.PASSTORE_AGENT_BIN || 'passtore-agent';
    const args = vaultPath ? ['--vault', vaultPath] : [];
    const child = spawn(bin, args, { detached: true, stdio: 'ignore' });
    child.unref();

    // Esperar a que escriba agent.json y responda.
    for (let i = 0; i < 40; i++) {
      await sleep(150);
      try {
        return await this.probe();
      } catch (_) {
        this.info = null;
      }
    }
    throw new AgentError('no pude arrancar/contactar el agente', 0);
  }

  // ---- API de alto nivel ----
  status() { return this.request('GET', '/status'); }
  unlock(master) { return this.request('POST', '/unlock', { master }); }
  lock() { return this.request('POST', '/lock'); }
  list() { return this.request('GET', '/entries'); }
  search(q) { return this.request('GET', `/search?q=${encodeURIComponent(q)}`); }
  get(id) { return this.request('GET', `/entries/${encodeURIComponent(id)}`); }
  add(entry) { return this.request('POST', '/entries', entry); }
  update(id, entry) { return this.request('PUT', `/entries/${encodeURIComponent(id)}`, entry); }
  remove(id) { return this.request('DELETE', `/entries/${encodeURIComponent(id)}`); }
  generate(opts) { return this.request('POST', '/generate', opts || {}); }
  otp(id) { return this.request('GET', `/entries/${encodeURIComponent(id)}/otp`); }
  capabilities() { return this.request('GET', '/capabilities'); }
}

module.exports = { AgentClient, AgentError, runtimePath, configDir, readRuntime };
