// preload.js — expone una API mínima y segura al renderer vía contextBridge.
// El renderer no tiene acceso a Node ni al token; solo a estos métodos.

'use strict';

const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('passtore', {
  connect: () => ipcRenderer.invoke('agent:connect'),
  status: () => ipcRenderer.invoke('agent:status'),
  unlock: (master) => ipcRenderer.invoke('agent:unlock', master),
  lock: () => ipcRenderer.invoke('agent:lock'),
  list: () => ipcRenderer.invoke('agent:list'),
  search: (q) => ipcRenderer.invoke('agent:search', q),
  get: (id) => ipcRenderer.invoke('agent:get', id),
  add: (entry) => ipcRenderer.invoke('agent:add', entry),
  update: (id, entry) => ipcRenderer.invoke('agent:update', id, entry),
  remove: (id) => ipcRenderer.invoke('agent:remove', id),
  generate: (opts) => ipcRenderer.invoke('agent:generate', opts),
  otp: (id) => ipcRenderer.invoke('agent:otp', id),
  capabilities: () => ipcRenderer.invoke('agent:capabilities'),
  copy: (text, clearMs) => ipcRenderer.invoke('clipboard:write', text, clearMs),
});
