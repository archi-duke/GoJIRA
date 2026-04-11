import config from '../config';

// SSE → WebSocket 전환 (HTTP/1.1 연결 한도 문제 해결)
const WS_URL = config.PLATFORM_API.replace(/^http/, 'ws') + '/Notice/ws';
const WIN_KEY = '__gojira_ws__';

function getState() {
  if (!window[WIN_KEY]) window[WIN_KEY] = {};
  const s = window[WIN_KEY];
  if (!s.listeners) s.listeners = {};
  if (!s.statusCbs) s.statusCbs = new Set();
  if (s.connected === undefined) s.connected = false;
  return s;
}

function notifyStatus(connected) {
  const state = getState();
  state.connected = connected;
  state.statusCbs.forEach(cb => { try { cb(connected); } catch {} });
}

function ensureConnection() {
  if (typeof WebSocket === 'undefined') return;
  const state = getState();
  if (state.ws && state.ws.readyState <= WebSocket.OPEN) return;

  const ws = new WebSocket(WS_URL);

  ws.onopen = () => { notifyStatus(true); };

  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data);
      if (msg.event === 'connected') return;
      const cbs = state.listeners[msg.event];
      if (cbs) {
        const fakeEvent = { data: msg.data };
        cbs.forEach(cb => { try { cb(fakeEvent); } catch {} });
      }
    } catch {}
  };

  ws.onclose = () => {
    notifyStatus(false);
    state.ws = null;
    setTimeout(() => ensureConnection(), 3000);
  };

  ws.onerror = () => { notifyStatus(false); };

  state.ws = ws;
}

if (typeof window !== 'undefined') {
  ensureConnection();
}

export function subscribe(event, callback) {
  const state = getState();
  if (!state.listeners[event]) {
    state.listeners[event] = new Set();
  }
  state.listeners[event].add(callback);
  ensureConnection();

  return () => {
    const s = getState();
    s.listeners[event]?.delete(callback);
    if (s.listeners[event]?.size === 0) delete s.listeners[event];
  };
}

export function onStatus(cb) {
  const state = getState();
  state.statusCbs.add(cb);
  try { cb(state.connected); } catch {}
  return () => { getState().statusCbs.delete(cb); };
}
