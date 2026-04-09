import config from '../config';

const SSE_URL = config.API_BASE + '/Notice/stream';

let es = null;
let listeners = {}; // event → Set<callback>
let refCount = 0;

function ensureConnection() {
  if (es && es.readyState !== EventSource.CLOSED) return;

  es = new EventSource(SSE_URL);

  es.addEventListener('connected', () => {});

  // 등록된 모든 이벤트 타입에 대해 리스너 연결
  Object.keys(listeners).forEach(event => {
    attachEvent(event);
  });

  es.onerror = () => {
    // EventSource는 자동 재연결됨 — 별도 처리 불필요
  };
}

function attachEvent(event) {
  if (!es) return;
  es.addEventListener(event, (e) => {
    const cbs = listeners[event];
    if (cbs) cbs.forEach(cb => cb(e));
  });
}

/**
 * SSE 이벤트 구독. 창 하나당 EventSource 1개만 공유.
 * @param {string} event - 이벤트 이름 (예: 'doc_update', 'emergency')
 * @param {function} callback - 이벤트 핸들러
 * @returns {function} unsubscribe 함수
 */
export function subscribe(event, callback) {
  if (!listeners[event]) {
    listeners[event] = new Set();
    // 이미 연결이 있으면 새 이벤트 타입 등록
    if (es && es.readyState !== EventSource.CLOSED) {
      attachEvent(event);
    }
  }
  listeners[event].add(callback);
  refCount++;
  ensureConnection();

  return () => {
    listeners[event]?.delete(callback);
    if (listeners[event]?.size === 0) delete listeners[event];
    refCount--;
    if (refCount <= 0) {
      refCount = 0;
      if (es) { es.close(); es = null; }
    }
  };
}
