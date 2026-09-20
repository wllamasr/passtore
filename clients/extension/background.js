// background.js — service worker. Único punto que habla con el host de native
// messaging; el popup y los content scripts le piden a él vía runtime messages.

'use strict';

const HOST = 'com.passtore.host';

// nativeRequest envía un mensaje al host nativo y resuelve con su respuesta.
function nativeRequest(payload) {
  return new Promise((resolve) => {
    try {
      chrome.runtime.sendNativeMessage(HOST, payload, (resp) => {
        if (chrome.runtime.lastError) {
          resolve({ ok: false, error: chrome.runtime.lastError.message });
        } else {
          resolve(resp || { ok: false, error: 'sin respuesta del host' });
        }
      });
    } catch (e) {
      resolve({ ok: false, error: String(e) });
    }
  });
}

// Relay: cualquier parte de la extensión pide { native: <payload> }.
chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg && msg.native) {
    nativeRequest(msg.native).then(sendResponse);
    return true; // respuesta asíncrona
  }
  return false;
});
