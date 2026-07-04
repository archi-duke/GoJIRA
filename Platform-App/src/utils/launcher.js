// 로컬 런처(gojira-launcher) 클라이언트 — Platform-App 의 로고/타이틀 클릭 핸들러용.
// GoJIRA-App 의 동명 파일과 거의 같으나 Platform-App 컨테이너에서 동작 (LAUNCHER_BASE 도 별도).
import config from '../config';

const BASE = (config.LAUNCHER_BASE || 'http://127.0.0.1:5599').replace(/\/$/, '');
const TOKEN_HEADER = 'X-Gojira-Launcher-Token';
// SSO 세션토큰을 런처에 전달(Phase 4 C) — 런처가 에이전트 env(GOJIRA_SESSION_TOKEN)로 주입.
const SESSION_HEADER = 'X-Gojira-Session';
function ssoSession() {
  const m = (typeof document !== 'undefined' ? document.cookie : '').match(/(^| )SSO_SESSION=([^;]+)/);
  return m ? decodeURIComponent(m[2]) : '';
}

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

// 180s — /chat 은 최초 EnsureRepo clone 포함, 사내 폐쇄망에서 30s 를 넘겨
// 브라우저 abort("canceled") → '런처 미기동' 오진이 나던 문제 방지.
const LAUNCHER_OP_TIMEOUT_MS = 180000;

export async function requestChat({ code, user, agent, token }) {
  try {
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers[TOKEN_HEADER] = token;
    const sess = ssoSession();
    if (sess) headers[SESSION_HEADER] = sess;
    const res = await fetchWithTimeout(`${BASE}/chat`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ code: code || '', user, agent }),
    }, LAUNCHER_OP_TIMEOUT_MS);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: data.error || `런처 응답 오류 (${res.status})` };
    return { ok: true, data };
  } catch (e) {
    if (e && e.name === 'AbortError') {
      return { ok: false, error: '런처 응답 대기 시간 초과 — git 작업(최초 clone)이 오래 걸리는 중일 수 있습니다. 잠시 후 다시 시도하세요.' };
    }
    return { ok: false, error: '로컬 런처에 연결할 수 없습니다 (127.0.0.1 데몬 미기동?)' };
  }
}
