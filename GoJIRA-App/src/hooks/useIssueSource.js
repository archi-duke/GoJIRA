// 웹 이슈 뷰 이원화 Phase 1 — 공식본(main/Mongo) ↔ 내 작업본(로컬 런처) 공용 훅.
// 설계: docs/Long-term Considerations/web-local-view-model.md §3 Phase 1.
//
// RelationMap/SATAM/IssueList 가 각자의 이슈 fetch 로직 대신 이 훅을 쓴다. 공식본 경로는
// 기존 동작(authFetch → GoJIRA-API /Issue)과 완전히 동일해야 하므로, 런처가 없거나
// sourceAvailable=false 인 환경에서는 이 훅이 기존 컴포넌트와 1비트도 다르지 않게 동작한다.
import { useState, useEffect, useRef, useCallback } from 'react';
import { detectLauncher, requestSessions, requestIssues } from '@platform/web-ui/runtime/launcher';
import { authFetch } from '../platform/auth';
import { parseLocalIssues } from '../utils/localIssues';
import config from '../config';

const API_BASE = config.API_BASE;

// 공식본 기본 fetch — RelationMap.jsx 159-166 과 동일한 Mongo 경유 조회.
// SATAM/IssueList 도 현재 동일한 쿼리를 쓰지만, 향후 갈라질 경우를 대비해 컴포넌트가
// mainFetch 콜백으로 override 할 수 있게 한다.
async function defaultMainFetch(projectCode) {
  const res = await authFetch(`${API_BASE}/Issue`, {
    headers: { XBody: JSON.stringify({ prjID: projectCode, infoType: '' }) },
  });
  const data = await res.json();
  return Array.isArray(data) ? data : [];
}

/**
 * useIssueSource — 공식본/내 작업본 이슈 뷰 전환 공용 훅.
 *
 * @param {object} opts
 * @param {string} opts.projectCode - 프로젝트 코드 (jiraCodes.main.jiraCode)
 * @param {string} opts.user - 현재 사용자 (컴포넌트가 받는 user prop 그대로)
 * @param {(projectCode:string)=>Promise<object[]>} [opts.mainFetch] - 공식본 fetch override
 * @returns {{
 *   source: 'main'|'local',
 *   setSource: (next:'main'|'local')=>void,
 *   sourceAvailable: boolean,
 *   issues: object[],
 *   loading: boolean,
 *   refetch: ()=>Promise<void>,
 *   localMeta: {count:number}|null,
 *   launcherToken: string,
 * }}
 */
export default function useIssueSource({ projectCode, user, mainFetch }) {
  const [source, setSourceState] = useState('main');
  const [sourceAvailable, setSourceAvailable] = useState(false);
  const [issues, setIssues] = useState([]);
  const [loading, setLoading] = useState(false);
  const [localMeta, setLocalMeta] = useState(null); // { count } — 배지 문구용

  // 최신 source 를 콜백/이펙트 클로저에서 안전하게 읽기 위한 ref (staleness 방지).
  const sourceRef = useRef('main');
  const launcherRef = useRef(null); // detectLauncher 결과(token 포함) 캐시.
  const initializedRef = useRef(false); // 초기 자동판별(세션 기준)은 1회만.

  const mainFetchRef = useRef(mainFetch);
  mainFetchRef.current = mainFetch;

  const runMainFetch = useCallback(
    () => (mainFetchRef.current ? mainFetchRef.current(projectCode) : defaultMainFetch(projectCode)),
    [projectCode],
  );

  const fetchLocal = useCallback(async () => {
    const launcher = launcherRef.current;
    if (!launcher?.ready || !projectCode || !user) return { ok: false };
    const r = await requestIssues({ code: projectCode, user, token: launcher.token });
    if (!r.ok) return { ok: false, error: r.error };
    const files = r.data?.issues || [];
    return { ok: true, issues: parseLocalIssues(files), count: r.data?.count ?? files.length };
  }, [projectCode, user]);

  // fetchForSource — 지정한 source 로 issues 를 채운다. 로컬 조회가 실패하면
  // (예: 그 사이 clone 삭제) 공식본으로 자동 폴백(설계 원칙: 폴백 안전망).
  const fetchForSource = useCallback(
    async (src) => {
      setLoading(true);
      try {
        if (src === 'local') {
          const r = await fetchLocal();
          if (r.ok) {
            setIssues(r.issues);
            setLocalMeta({ count: r.count });
            return;
          }
          console.warn('[useIssueSource] 로컬 조회 실패 — 공식본으로 폴백:', r.error);
          sourceRef.current = 'main';
          setSourceState('main');
        }
        setLocalMeta(null);
        const mainIssues = await runMainFetch();
        setIssues(mainIssues);
      } catch (err) {
        console.warn('[useIssueSource] fetch 실패:', err);
        setIssues([]);
      } finally {
        setLoading(false);
      }
    },
    [fetchLocal, runMainFetch],
  );

  // 외부에 노출하는 refetch — 인자 없이 "현재 source 로 다시 조회" (기존 fetchIssues() 호출부와 동일 사용감).
  const refetch = useCallback(() => fetchForSource(sourceRef.current), [fetchForSource]);

  // 뷰 전환 — 즉시 상태 반영 + 재조회.
  const setSource = useCallback(
    (next) => {
      if (next !== 'main' && next !== 'local') return;
      sourceRef.current = next;
      setSourceState(next);
      fetchForSource(next);
    },
    [fetchForSource],
  );

  // 마운트/과제 전환 시: 런처 감지 → 이 과제 로컬 clone 존재(/issues 200) 판정 →
  // 에이전트 세션(같은 과제) 살아있으면 초기 source='local', 아니면 'main'.
  useEffect(() => {
    let alive = true;
    if (!projectCode || !user) return undefined;
    initializedRef.current = false;

    (async () => {
      const launcher = await detectLauncher();
      if (!alive) return;
      launcherRef.current = launcher;

      if (!launcher?.ready) {
        setSourceAvailable(false);
        if (!initializedRef.current) {
          initializedRef.current = true;
          sourceRef.current = 'main';
          setSourceState('main');
          fetchForSource('main');
        }
        return;
      }

      const probe = await requestIssues({ code: projectCode, user, token: launcher.token });
      if (!alive) return;
      setSourceAvailable(!!probe.ok);

      if (initializedRef.current) return;
      initializedRef.current = true;

      let initial = 'main';
      if (probe.ok) {
        const sess = await requestSessions({ token: launcher.token });
        if (alive && sess.ok) {
          const liveHere = (sess.data?.sessions || []).some((s) => s.code === projectCode);
          if (liveHere) initial = 'local';
        }
      }
      if (!alive) return;
      sourceRef.current = initial;
      setSourceState(initial);
      fetchForSource(initial);
    })();

    return () => { alive = false; };
    // fetchForSource 는 projectCode/user 에 종속되므로 여기 의존성에서 의도적으로 제외
    // (마운트/과제전환 시 1회만 초기판별을 수행하기 위함 — initializedRef 로 재진입 방지).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectCode, user]);

  // Phase 3 편집 브릿지 — 로컬 쓰기 헬퍼(utils/localIssueWrite.js)가 런처 호출(write/delete/
  // issue-journal)에 쓸 토큰. launcherRef 는 마운트 시 detectLauncher 로 1회 채워지고, 이후
  // 재렌더(setSourceAvailable 등) 시점엔 이미 채워져 있으므로 스냅샷으로 노출해도 안전.
  const launcherToken = launcherRef.current?.token || '';

  return { source, setSource, sourceAvailable, issues, loading, refetch, localMeta, launcherToken };
}
