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
// SSO 세션토큰을 런처에 전달(Phase 4 C) — 런처가 백엔드 호출/에이전트 env 에 Bearer 로 쓴다.
const SESSION_HEADER = 'X-Gojira-Session';
function ssoSession() {
  const m = (typeof document !== 'undefined' ? document.cookie : '').match(/(^| )SSO_SESSION=([^;]+)/);
  return m ? decodeURIComponent(m[2]) : '';
}

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
 * "수정" — 로컬 clone 보장 + docs/{user} 체크아웃 + VS Code(특정 file) 오픈.
 *           에이전트 CLI 는 spawn 하지 않는다 (별도 /chat).
 * @returns {Promise<{ok:boolean, data?:object, error?:string}>}
 */
export async function requestEdit({ code, user, file, token }) {
  return post('/edit', { code, user, file }, token);
}

/**
 * "채팅" — 등록된 에이전트 CLI(opencode|claude)만 새 터미널로 spawn.
 *           VS Code 는 띄우지 않음 (필요하면 /edit 으로 분리 호출).
 * replace=true 면 다른 과제의 에이전트를 종료한 뒤 spawn (과제전환 — 단일 현재과제 에이전트).
 */
export async function requestChat({ code, user, agent, replace, token }) {
  return post('/chat', { code, user, agent, replace: !!replace }, token);
}

/**
 * 현재 살아있는 에이전트 세션 목록. 과제전환 시 "다른 과제 에이전트가 떠 있는가" 판정용.
 * @returns {Promise<{ok:boolean, data?:{sessions:[{code,agent,pid}]}, error?:string}>}
 */
export async function requestSessions({ token }) {
  return get('/sessions', token);
}

/**
 * "프리웜" — launcher 가 EnsureRepo(clone + 체크아웃)를 백그라운드로 미리 수행한다.
 * 과제 생성 직후 호출하면 사용자가 로고를 클릭하는 시점에 EnsureRepo 가 이미 끝나
 * SpawnAgent 까지 체감 지연이 거의 없다. 즉시 200 응답 ({ status: "started" }) —
 * 클라이언트는 결과를 기다리지 않고 fire-and-forget.
 */
export async function requestPrewarm({ code, user, token }) {
  return post('/prewarm', { code, user }, token);
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

/**
 * "등록" — dirty 면 git add -A + commit, 그 다음 push. 미커밋/미푸시를 한 번에 origin 까지 올린다.
 * @returns {Promise<{ok:boolean, data?:{committed:boolean, push:object}, error?:string}>}
 */
export async function requestSync({ code, user, message, token }) {
  return post('/sync', { code, user, message }, token);
}

/**
 * "Pull" — origin/main 의 새 내용을 로컬 docs/{user} 로 끌어온다.
 * FF 가능하면 fast-forward, 사용자 커밋이 있으면 rebase. dirty 면 거부.
 * @returns {Promise<{ok:boolean, data?:{status, strategy, merged, ahead, branch, message, lastCommit}, error?:string}>}
 */
export async function requestPullFromMain({ code, user, token }) {
  return post('/pull', { code, user }, token);
}

/**
 * 인라인 편집 저장 — 로컬 working tree 의 단일 파일에 raw text 쓰기. commit/push 안 함.
 */
export async function requestWrite({ code, user, path, content, token }) {
  return post('/write', { code, user, path, content }, token);
}

/**
 * 파일 삭제 — 로컬 working tree 의 단일 파일 삭제. commit/push 안 함(/sync 가 포착).
 */
export async function requestDelete({ code, user, path, token }) {
  return post('/delete', { code, user, path }, token);
}

/**
 * 파일 이동/이름변경 — 로컬 working tree 의 from→to. commit/push 안 함(/sync 가 포착).
 */
export async function requestRename({ code, user, from, to, token }) {
  return post('/rename', { code, user, from, to }, token);
}

/**
 * 로컬 working tree 파일 목록. 응답 shape 은 서버 /DocRepo/{code}/tree 와 호환:
 *   { tree: [{path, type:"blob", sha, size}], total_count, source }
 * sha 는 git blob 해시라 main 트리 sha 와 그대로 비교 가능.
 */
export async function requestFiles({ code, user, token }) {
  return get(`/files?code=${encodeURIComponent(code)}&user=${encodeURIComponent(user)}`, token);
}

/**
 * 로컬 working tree 단일 파일 내용. 응답 shape:
 *   { path, content(base64), encoding:"base64", size, source }
 */
export async function requestLocalFile({ code, user, path, token }) {
  const q = `code=${encodeURIComponent(code)}&user=${encodeURIComponent(user)}&path=${encodeURIComponent(path)}`;
  return get(`/file?${q}`, token);
}

// 30s — 사용자 PC ↔ Gitea 네트워크 RTT 가 느린 환경에서 git fetch/clone/merge 가
// 2.5s 안에 안 끝나는 케이스 (사내 폐쇄망 등) 대비. health 만 짧게 (별도 호출).
const LAUNCHER_OP_TIMEOUT_MS = 30000;

async function get(path, token) {
  try {
    const headers = {};
    if (token) headers[TOKEN_HEADER] = token;
    const sess = ssoSession();
    if (sess) headers[SESSION_HEADER] = sess;
    const res = await fetchWithTimeout(`${BASE}${path}`, { method: 'GET', headers }, LAUNCHER_OP_TIMEOUT_MS);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) return { ok: false, error: data.error || `런처 응답 오류 (${res.status})` };
    return { ok: true, data };
  } catch {
    return { ok: false, error: '로컬 런처에 연결할 수 없습니다 (127.0.0.1 데몬 미기동?)' };
  }
}

async function post(path, body, token) {
  try {
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers[TOKEN_HEADER] = token;
    const sess = ssoSession();
    if (sess) headers[SESSION_HEADER] = sess;
    const res = await fetchWithTimeout(`${BASE}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
    }, LAUNCHER_OP_TIMEOUT_MS);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      return { ok: false, error: data.error || `런처 응답 오류 (${res.status})` };
    }
    return { ok: true, data };
  } catch {
    return { ok: false, error: '로컬 런처에 연결할 수 없습니다 (127.0.0.1 데몬 미기동?)' };
  }
}
