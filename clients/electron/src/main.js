// main.js — proceso principal de Electron.
// Descubre/arranca el agente local y hace de puente seguro entre la UI
// (renderer) y la API del agente. El token de sesión vive solo aquí, nunca
// llega al renderer.

'use strict';

const { app, BrowserWindow, ipcMain, clipboard } = require('electron');
const path = require('path');
const { AgentClient } = require('./agentClient');

const agent = new AgentClient();
let mainWindow = null;

// agentBinPath resuelve el ejecutable del agente. Empaquetado, viene en los
// recursos de la app (resources/bin); en desarrollo, se usa PASSTORE_AGENT_BIN
// o el PATH.
function agentBinPath() {
  if (app.isPackaged) {
    return path.join(process.resourcesPath, 'bin', 'passtore-agent.exe');
  }
  return process.env.PASSTORE_AGENT_BIN || undefined;
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 900,
    height: 640,
    minWidth: 680,
    minHeight: 480,
    title: 'Passtore',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  });
  mainWindow.loadFile(path.join(__dirname, 'renderer', 'index.html'));
}

// wrap ejecuta una operación del agente y devuelve un resultado uniforme,
// preservando el status HTTP (p.ej. 401 contraseña mala, 423 bloqueado).
async function wrap(fn) {
  try {
    const data = await fn();
    return { ok: true, data };
  } catch (err) {
    return { ok: false, error: err.message || String(err), status: err.status || 0 };
  }
}

function registerIPC() {
  ipcMain.handle('agent:connect', () =>
    wrap(() => agent.connect({ binPath: agentBinPath() })));
  ipcMain.handle('agent:status', () => wrap(() => agent.status()));
  ipcMain.handle('agent:unlock', (_e, master) => wrap(() => agent.unlock(master)));
  ipcMain.handle('agent:lock', () => wrap(() => agent.lock()));
  ipcMain.handle('agent:list', () => wrap(() => agent.list()));
  ipcMain.handle('agent:search', (_e, q) => wrap(() => agent.search(q)));
  ipcMain.handle('agent:get', (_e, id) => wrap(() => agent.get(id)));
  ipcMain.handle('agent:add', (_e, entry) => wrap(() => agent.add(entry)));
  ipcMain.handle('agent:update', (_e, id, entry) => wrap(() => agent.update(id, entry)));
  ipcMain.handle('agent:remove', (_e, id) => wrap(() => agent.remove(id)));
  ipcMain.handle('agent:generate', (_e, opts) => wrap(() => agent.generate(opts)));
  ipcMain.handle('agent:otp', (_e, id) => wrap(() => agent.otp(id)));
  ipcMain.handle('agent:capabilities', () => wrap(() => agent.capabilities()));

  // Copiar al portapapeles y limpiarlo tras un tiempo (higiene).
  ipcMain.handle('clipboard:write', (_e, text, clearMs) => {
    clipboard.writeText(String(text));
    if (clearMs && clearMs > 0) {
      setTimeout(() => {
        if (clipboard.readText() === text) clipboard.clear();
      }, clearMs);
    }
    return { ok: true };
  });
}

app.whenReady().then(() => {
  registerIPC();
  createWindow();
  // Auto-actualización desde GitHub Releases (solo en la app empaquetada).
  if (app.isPackaged) {
    try {
      require('electron-updater').autoUpdater.checkForUpdatesAndNotify();
    } catch (err) {
      console.error('auto-update no disponible:', err);
    }
  }
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
