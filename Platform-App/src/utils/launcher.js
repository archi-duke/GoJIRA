// 로컬 런처(gojira-launcher) 클라이언트 — Platform-App 의 로고/타이틀 클릭 핸들러용.
// GoJIRA-App 의 동명 파일과 거의 같으나 Platform-App 컨테이너에서 동작 (LAUNCHER_BASE 도 별도).
import config from '../config';

const BASE = (config.LAUNCHER_BASE || 'http://127.0.0.1:5599').replace(/\/$/, '');
const TOKEN_HEADER = 'X-Gojira-Launcher-Token';

async function fetchWithTimeout(url, opts = {}, ms = 2500) {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), ms);
  try {
    return await fetch(url, { ...opts, signal: ctrl.signal });
  } finally {
    clearTimeout(t);
  }
}

export async function detectLauncher() {
  try {
    const res = await fetchWithTimeout(`${BASE}/health`, { method: 'GET' }, 2000);
    if (!res.ok) return null;
    const data = await res.json();
    return {
      version: data.version,
      agents: Array.isArray(data.agents) ? data.agents : [],
      ready: !!data.ready,
      token: data.token || '',
    };
  } catch {
    return null;
  }
}

// 30s — git fetch/clone 가 느린 네트워크(폐쇄망 등) 에서 2.5s 안에 안 끝나는 케이스 대비.
const LAUNCHER_OP_TIMEOUT_MS = 30000;

export async function requestChat({ code, user, agent, token }) {
  try {
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers[TOKEN_HEADER] = token;
    const res = await fetchWithTimeout(`${BASE}/chat`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ code: code || '', user, agent }),
    }, LAUNCHER_OP_TIMEOUT_MS);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: data.error || `런처 응답 오류 (${res.status})` };
    return { ok: true, data };
  } catch {
    return { ok: false, error: '로컬 런처에 연결할 수 없습니다 (127.0.0.1 데몬 미기동?)' };
  }
}
