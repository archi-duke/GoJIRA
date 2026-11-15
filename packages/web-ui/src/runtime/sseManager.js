import { PLATFORM_API } from './env';
import { getToken } from './auth';

// SSE → WebSocket 전환 (HTTP/1.1 연결 한도 문제 해결)
const WS_URL = PLATFORM_API.replace(/^http/, 'ws') + '/Notice/ws';
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

  ws.onopen = () => {
    // 첫 메시지로 인증한다. 브라우저 WebSocket 핸드셰이크는 Authorization 헤더를
    // 실을 수 없고, 쿼리파라미터는 액세스 로그에 토큰이 남아 쓰지 않는다
    // (계약 05 §WebSocket 인증 · 2026-07-07 D2).
    // 서버는 인증 전에는 이벤트를 보내지 않으므로, 여기서 보내지 않으면 10초 후 닫힌다.
    try {
      ws.send(JSON.stringify({
        type: 'auth',
        token: getToken() || '',
        // dev(USE_SSO!=true) 폴백 — 서버가 이 값을 사용자로 받아들인다.
        user: (typeof window !== 'undefined' && window.__gojiraUser) || '',
      }));
    } catch {}
    // connected 수신 시점에 연결됨으로 알린다 — 인증 실패면 error 가 먼저 온다.
  };

  ws.onmessage = (e) => {
    try {
      const msg = JSON.parse(e.data);
      if (msg.event === 'connected') {
        // 인증까지 끝난 시점이다. 여기서 비로소 연결됨으로 본다.
        notifyStatus(true);
        return;
      }
      if (msg.event === 'error') {
        // 인증 실패 등 — 서버가 곧 닫는다. onclose 의 재연결에 맡긴다.
        console.warn('[ws]', msg.data);
        return;
      }
      // 디버그: 수신 메시지 표시 (테스트 시 주석 해제)
      // try { const d = JSON.parse(msg.data); window.alert(`[WS] ${msg.event}: ${d.type || ''} ${d.projectCode || ''} ${d.action || ''}`); } catch {}
      // SSE 호환: event명으로 라우팅, data는 JSON 문자열
      const cbs = state.listeners[msg.event];
      if (cbs) {
        // SSE Event 객체와 동일한 형태로 콜백 호출
        const fakeEvent = { data: msg.data };
        cbs.forEach(cb => { try { cb(fakeEvent); } catch {} });
      }
    } catch {}
  };

  ws.onclose = () => {
    notifyStatus(false);
    state.ws = null;
    // 3초 후 자동 재연결
    setTimeout(() => ensureConnection(), 3000);
  };

  ws.onerror = () => {
    notifyStatus(false);
  };

  state.ws = ws;
}

// 앱 시작 시 즉시 연결
if (typeof window !== 'undefined') {
  ensureConnection();
}

/**
 * 이벤트 구독 (SSE subscribe와 동일한 인터페이스)
 */
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

/**
 * 연결 상태 구독
 */
export function onStatus(cb) {
  const state = getState();
  state.statusCbs.add(cb);
  try { cb(state.connected); } catch {}
  return () => { getState().statusCbs.delete(cb); };
}
