// 로컬 런처(gojira-launcher) 클라이언트.
//
// 런처는 사용자 PC 의 127.0.0.1 데몬으로, 웹의 "수정" 버튼이 VS Code +
// CLI(opencode|claude)를 소환하도록 중계한다 (설계: docs/Long-term Considerations/
// local-agent-mvp.md §2). 런처가 설치/기동돼 있지 않으면 detect 는 조용히 null 을
// 돌려주고, 웹은 버튼을 숨긴다.
//
// 토큰 핸드셰이크(§6 PoC): /health 는 Origin 이 런처의 webOrigin 과 일치하는
// 요청에만 token 을 돌려준다. 그 토큰을 이후 /edit·/open 의 인증 헤더로 보낸다.
import config from '../config';

const BASE = (config.LAUNCHER_BASE || 'http://127.0.0.1:5599').replace(/\/$/, '');
const TOKEN_HEADER = 'X-Gojira-Launcher-Token';

// fetch + 타임아웃 (런처 미기동 시 connection refused 가 빠르게 떨어지지만,
// 방화벽/응답지연 대비 명시적 abort).
async function fetchWithTimeout(url, opts = {}, ms = 2500) {
  const ctrl = new AbortController();
  const t = setTimeout(() => ctrl.abort(), ms);
  try {
    return await fetch(url, { ...opts, signal: ctrl.signal });
  } finally {
    clearTimeout(t);
  }
}

/**
 * 런처 감지. 기동/설치돼 있으면 { version, agents, ready, token } 반환,
 * 아니면 null. (네트워크 에러·비정상 응답 모두 null)
 */
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

/**
 * "수정" — 로컬 clone 보장 + docs/{user} 체크아웃 + VS Code(특정 file) + CLI spawn.
 * @returns {Promise<{ok:boolean, data?:object, error?:string}>}
 */
export async function requestEdit({ code, user, file, agent, token }) {
  return post('/edit', { code, user, file, agent }, token);
}

/**
 * "열기" — clone 보장 + VS Code 폴더 오픈 (특정 파일/CLI 없이).
 */
export async function requestOpen({ code, user, token }) {
  return post('/open', { code, user }, token);
}

/**
 * "push" — 로컬 docs/{user} 브랜치를 origin 으로 push (§5-3 push→PR).
 * @returns {Promise<{ok:boolean, data?:{pushed,commits,dirty,branch,message}, error?:string}>}
 */
export async function requestPush({ code, user, token }) {
  return post('/push', { code, user }, token);
}

async function post(path, body, token) {
  try {
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers[TOKEN_HEADER] = token;
    const res = await fetchWithTimeout(`${BASE}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      return { ok: false, error: data.error || `런처 응답 오류 (${res.status})` };
    }
    return { ok: true, data };
  } catch {
    return { ok: false, error: '로컬 런처에 연결할 수 없습니다 (127.0.0.1 데몬 미기동?)' };
  }
}
