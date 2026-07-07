// 로컬뷰 이슈 쓰기 헬퍼 (웹 이슈 뷰 이원화 Phase 3 — 편집 브릿지 부활).
// 설계: docs/Long-term Considerations/web-local-view-model.md §3 Phase 3,
//       §Phase 2 base-from-client 프로토콜(GoJIRA-API handlers/issue_local.go).
//
// 공통 흐름: GoJIRA-API 호출(X-Local-Write 헤더 + base 필드 동봉) → 응답 localWrites 를
// 런처에 반영(content 있으면 requestWrite, delete:true 면 requestDelete) → requestJournal
// (best-effort — 실패해도 편집 자체를 막지 않음, 콘솔 경고만).
//
// 이 헬퍼들은 로컬뷰(source==='local')에서만 호출된다 — 그 전제 하에 issue._baseYaml
// (parseLocalIssues 가 얹은 raw YAML 원문)이 존재해야 하며, 없으면 명확한 에러를 던져
// 호출측(컴포넌트)이 alert 로 사용자에게 알릴 수 있게 한다.
import { authFetch } from '../platform/auth';
import { requestWrite, requestDelete, requestJournal } from '@platform/web-ui/runtime/launcher';
import config from '../config';

const API_BASE = config.API_BASE;

async function callApi(path, method, body) {
  const res = await authFetch(`${API_BASE}${path}`, {
    method,
    headers: { 'Content-Type': 'application/json', 'X-Local-Write': 'true' },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.error || `요청 실패 (${res.status})`);
  }
  return data;
}

// applyLocalWrites — 서버 응답의 localWrites 를 런처(로컬 clone)에 반영한다.
async function applyLocalWrites({ projectCode, user, token, localWrites }) {
  for (const w of localWrites || []) {
    if (w.delete) {
      const r = await requestDelete({ code: projectCode, user, path: w.path, token });
      if (!r.ok) throw new Error(`로컬 파일 삭제 실패 (${w.path}): ${r.error}`);
    } else {
      const r = await requestWrite({ code: projectCode, user, path: w.path, content: w.content, token });
      if (!r.ok) throw new Error(`로컬 파일 쓰기 실패 (${w.path}): ${r.error}`);
    }
  }
}

// journalBestEffort — 에이전트 변경 전달용 저널 기록. 실패는 콘솔 경고만 (편집을 막지 않음).
async function journalBestEffort({ projectCode, user, token, entry }) {
  try {
    const r = await requestJournal({ code: projectCode, user, entry, token });
    if (!r.ok) console.warn('[localIssueWrite] 변경 저널 기록 실패(무시):', r.error);
  } catch (err) {
    console.warn('[localIssueWrite] 변경 저널 기록 실패(무시):', err);
  }
}

function requireBaseYaml(issue, action) {
  if (!issue?._baseYaml) {
    throw new Error(
      `${action}: 로컬 원본(YAML)을 찾을 수 없습니다. 내 작업본 뷰에서만 편집할 수 있습니다 — 새로고침 후 다시 시도하세요.`,
    );
  }
  return issue._baseYaml;
}

/**
 * localUpdateIssue — 필드 업데이트. issue._baseYaml 필수.
 * @param {{projectCode:string, user:string, token:string, issue:object, data:object}} args
 */
export async function localUpdateIssue({ projectCode, user, token, issue, data }) {
  const baseYaml = requireBaseYaml(issue, '이슈 수정');
  const resp = await callApi('/Issue', 'PUT', { key: issue.key, data, baseYaml, prjID: projectCode });
  await applyLocalWrites({ projectCode, user, token, localWrites: resp.localWrites });
  const fields = Object.keys(data || {});
  await journalBestEffort({
    projectCode, user, token,
    entry: {
      op: 'update', keys: [issue.key], fields,
      summary: `${issue.subkey || issue.key} 필드 수정: ${fields.join(', ') || '(없음)'}`,
    },
  });
  return resp;
}

/**
 * localCreateIssue — 이슈 생성. subkey 는 넘기지 않는다(서버 카운터가 원자 할당 — 클라이언트
 * 계산 subkey 는 다중 사용자 충돌 위험). body 에 localOnly:true 를 자동 부가한다.
 * @param {{projectCode:string, user:string, token:string, body:object}} args
 */
export async function localCreateIssue({ projectCode, user, token, body }) {
  const { subkey: _ignoredSubkey, ...rest } = body || {};
  const resp = await callApi('/Issue', 'POST', { ...rest, localOnly: true });
  await applyLocalWrites({ projectCode, user, token, localWrites: resp.localWrites });
  await journalBestEffort({
    projectCode, user, token,
    entry: {
      op: 'create', keys: [resp.key].filter(Boolean),
      summary: `${resp.subkey || resp.key || '새 이슈'} 생성: ${body?.summary || ''}`.trim(),
    },
  });
  return resp;
}

/**
 * localDeleteIssue — 이슈 삭제. issue._baseYaml 필수.
 * @param {{projectCode:string, user:string, token:string, issue:object}} args
 */
export async function localDeleteIssue({ projectCode, user, token, issue }) {
  const baseYaml = requireBaseYaml(issue, '이슈 삭제');
  const resp = await callApi('/Issue', 'DELETE', { key: issue.key, prjID: projectCode, baseYaml });
  await applyLocalWrites({ projectCode, user, token, localWrites: resp.localWrites });
  await journalBestEffort({
    projectCode, user, token,
    entry: { op: 'delete', keys: [issue.key], summary: `${issue.subkey || issue.key} 삭제: ${issue.summary || ''}`.trim() },
  });
  return resp;
}

/**
 * localCreateLink — Contains 링크 생성. fromIssue/toIssue 중 _baseYaml 있는 쪽만 동봉
 * (타 과제 이슈라 로컬 clone 에 없을 수 있음 — 한쪽만 있어도 서버가 처리). 둘 다 없으면 에러.
 * @param {{projectCode:string, user:string, token:string, fromIssue:object, toIssue:object}} args
 */
export async function localCreateLink({ projectCode, user, token, fromIssue, toIssue }) {
  const inwardYaml = fromIssue?._baseYaml || '';
  const outwardYaml = toIssue?._baseYaml || '';
  if (!inwardYaml && !outwardYaml) {
    throw new Error('연결 생성: 로컬 원본(YAML)을 찾을 수 없습니다 (양쪽 이슈 모두). 내 작업본 뷰에서만 연결할 수 있습니다.');
  }
  const resp = await callApi('/Issue/link', 'POST', {
    prjID: projectCode,
    inwardKey: fromIssue.key,
    outwardKey: toIssue.key,
    inwardYaml,
    outwardYaml,
  });
  await applyLocalWrites({ projectCode, user, token, localWrites: resp.localWrites });
  await journalBestEffort({
    projectCode, user, token,
    entry: {
      op: 'link', keys: [fromIssue.key, toIssue.key],
      summary: `${fromIssue.subkey || fromIssue.key} → ${toIssue.subkey || toIssue.key} 연결 생성`,
    },
  });
  return resp;
}

/**
 * localDeleteLink — Contains 링크 삭제. 로드된 로컬 issues 에서 issueLinks[].id===linkId 인
 * 이슈들을 찾아 그들의 _baseYaml 을 baseFiles 로 동봉(0개면 에러).
 * @param {{projectCode:string, user:string, token:string, linkId:string, issues:object[]}} args
 */
export async function localDeleteLink({ projectCode, user, token, linkId, issues }) {
  const involved = (issues || []).filter((i) => (i.issueLinks || []).some((l) => l.id === linkId));
  const baseFiles = involved.map((i) => i._baseYaml).filter(Boolean);
  if (baseFiles.length === 0) {
    throw new Error('연결 삭제: 로컬 원본(YAML)을 찾을 수 없습니다. 내 작업본 뷰에서만 연결을 삭제할 수 있습니다.');
  }
  const resp = await callApi('/Issue/link', 'DELETE', { prjID: projectCode, linkId, baseFiles });
  await applyLocalWrites({ projectCode, user, token, localWrites: resp.localWrites });
  const involvedLabel = involved.map((i) => i.subkey || i.key).join(', ');
  await journalBestEffort({
    projectCode, user, token,
    entry: { op: 'unlink', keys: involved.map((i) => i.key), summary: `연결 삭제: ${involvedLabel}` },
  });
  return resp;
}
