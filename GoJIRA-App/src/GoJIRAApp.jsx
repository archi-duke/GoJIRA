import React, { useState, useCallback, useEffect, useRef } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';
import DocEditor from './components/DocEditor';
import AlertModal, { modalConfirm, modalAlert } from './components/AlertModal';
import config from './config';
import { detectLauncher, requestSessions, requestChat } from './utils/launcher';
import { subscribe } from './utils/sseManager';
import { createEventQueue } from './utils/eventQueue';

// 전역 fetch 에 인증 헤더 자동 주입. 모듈 로드 시점에 1회 설치 — child useEffect(fetch) 가
// parent 보다 먼저 실행되는 React 순서 문제를 피한다.
// - SSO: SSO_SESSION 토큰 → Authorization: Bearer (백엔드가 introspect 검증).
// - dev: SSO_SESSION 없으면 기존처럼 X-User-Id(window.__gojiraUser) 주입.
// API 경로(/api/v2 또는 백엔드 베이스)에만 적용 — MobilAve/외부 호출엔 미주입.
if (typeof window !== 'undefined' && !window.__gojiraFetchWrapped) {
  const orig = window.fetch;
  window.__gojiraFetchWrapped = true;
  const getSession = () => {
    const m = document.cookie.match(/(^| )SSO_SESSION=([^;]+)/);
    return m ? decodeURIComponent(m[2]) : '';
  };
  const isApi = (url) =>
    url.includes('/api/v2') ||
    (config.PLATFORM_API && url.startsWith(config.PLATFORM_API)) ||
    (config.GOJIRA_API && url.startsWith(config.GOJIRA_API));
  window.fetch = function (input, init) {
    const url = typeof input === 'string' ? input : (input && input.url) || '';
    if (isApi(url)) {
      init = init || {};
      const headers = new Headers(init.headers || {});
      const token = getSession();
      if (token) {
        if (!headers.has('Authorization')) headers.set('Authorization', 'Bearer ' + token);
      } else {
        const user = window.__gojiraUser;
        if (user && !headers.has('X-User-Id')) headers.set('X-User-Id', user);
      }
      init.headers = headers;
    }
    return orig.call(window, input, init);
  };
}

export default function GoJIRAApp({ user, authority, resetKey }) {
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [projectAlert, setProjectAlert] = useState(null);

  if (typeof window !== 'undefined') window.__gojiraUser = user;

  useEffect(() => {
    if (resetKey > 0) {
      setPage('map');
      setSelectedProject(null);
      sessionStorage.setItem('gojira_page', 'map');
      sessionStorage.removeItem('gojira_project');
    }
  }, [resetKey]); // eslint-disable-line
  const [selectedProject, setSelectedProject] = useState(() => {
    try { return JSON.parse(sessionStorage.getItem('gojira_project')); }
    catch { return null; }
  });

  const navigate = useCallback((p, project) => {
    setPage(p);
    setSelectedProject(project ?? null);
    sessionStorage.setItem('gojira_page', p);
    if (project) sessionStorage.setItem('gojira_project', JSON.stringify(project));
    else sessionStorage.removeItem('gojira_project');
  }, []);

  // Design B 과제전환: 에이전트가 "다른 과제" 에 열려 있는 상태에서 현재 과제가 바뀌면,
  // 사용자에게 확인한 뒤 옛 에이전트를 닫고 새 과제로 다시 띄운다(단일 "현재 과제" 에이전트).
  // 런처 미감지/미실행이거나, 떠 있는 에이전트가 없거나 같은 과제면 아무것도 안 한다
  // (others.length===0 가드 → 최초 진입/에이전트 없음 시 조용함).
  const prevProjectCodeRef = useRef(null);
  useEffect(() => {
    const code = selectedProject?.jiraCodes?.main?.jiraCode || '';
    const prev = prevProjectCodeRef.current;
    prevProjectCodeRef.current = code;
    if (!code || code === prev) return;
    let cancelled = false;
    (async () => {
      const r = await detectLauncher();
      if (cancelled || !r?.ready || !r.token) return;
      const s = await requestSessions({ token: r.token });
      if (cancelled || !s.ok) return;
      const others = (s.data?.sessions || []).filter((x) => x.code && x.code !== code);
      if (others.length === 0) return;
      const names = others.map((x) => x.code).join(', ');
      const ok = await modalConfirm(
        `다른 과제(${names}) 에이전트가 실행 중입니다.\n'${code}' 로 전환하시겠습니까?\n(기존 에이전트는 닫히고 '${code}' 에이전트가 새로 열립니다)`
      );
      if (cancelled || !ok) return;
      const res = await requestChat({ code, user, replace: true, token: r.token });
      if (!res.ok) modalAlert(res.error || '에이전트 전환에 실패했습니다.');
    })();
    return () => { cancelled = true; };
  }, [selectedProject, user]);

  // Agent 독립 창에서 보낸 네비게이션 명령 수신 (cross-origin postMessage + same-origin BroadcastChannel)
  // minimal project 로 덮어쓰면 documents (repoName 등) 가 사라져 DocEditor 가 "저장소 없음" 으로 오판.
  // → 기존 state/sessionStorage 의 full project 와 merge, 없으면 API 에서 조회 후 navigate.
  useEffect(() => {
    const API_BASE_URL = config.GOJIRA_API || config.API_BASE;
    async function resolveProject(projectCode, projectName) {
      const minimal = {
        name: projectName || projectCode,
        jiraCodes: { main: { jiraCode: projectCode } },
      };
      // 1. state 의 selectedProject 가 동일 코드면 재사용 (merge 로 name 만 갱신)
      try {
        const cur = selectedProject;
        if (cur && cur.jiraCodes?.main?.jiraCode === projectCode && cur.documents) {
          return { ...cur, name: projectName || cur.name };
        }
      } catch {}
      // 2. sessionStorage 캐시 확인
      try {
        const cached = JSON.parse(sessionStorage.getItem('gojira_project') || 'null');
        if (cached && cached.jiraCodes?.main?.jiraCode === projectCode && cached.documents) {
          return { ...cached, name: projectName || cached.name };
        }
      } catch {}
      // 3. API 조회 — /Project 전체 목록에서 코드 매칭
      try {
        const res = await fetch(`${API_BASE_URL}/Project`, { headers: { XBody: JSON.stringify({}) } });
        const arr = await res.json();
        if (Array.isArray(arr)) {
          const full = arr.find(p => p?.jiraCodes?.main?.jiraCode === projectCode);
          if (full) return full;
        }
      } catch {}
      return minimal;
    }

    async function handleMessage(e) {
      if (e.data?.type === 'gojira-navigate' && e.data.projectCode && e.data.page) {
        const p = await resolveProject(e.data.projectCode, e.data.projectName);
        navigate(e.data.page, p);
      }
    }
    window.addEventListener('message', handleMessage);
    const ch = new BroadcastChannel('gojira-navigate');
    ch.onmessage = async (evt) => {
      const { projectCode, projectName, page: targetPage } = evt.data;
      if (projectCode && targetPage) {
        const p = await resolveProject(projectCode, projectName);
        navigate(targetPage, p);
      }
    };
    return () => { window.removeEventListener('message', handleMessage); ch.close(); };
  }, [navigate, selectedProject]);

  const onNavigate = useCallback((target) => {
    navigate(target, selectedProject);
  }, [navigate, selectedProject]);

  const projectCode = selectedProject?.jiraCodes?.main?.jiraCode || '';
  const PLATFORM_API = config.PLATFORM_API || config.API_BASE;
  const GOJIRA_API = config.GOJIRA_API || config.API_BASE;

  useEffect(() => {
    if (!projectCode || page === 'map') { setProjectAlert(null); return; }
    const API = PLATFORM_API + '/DocRepo';

    const p1 = fetch(`${GOJIRA_API}/Project/alerts?user=${encodeURIComponent(user || '')}`)
      .then(r => r.json())
      .then(arr => {
        if (Array.isArray(arr)) return arr.find(a => a.jiraCode === projectCode) || null;
        return null;
      }).catch(() => null);

    const p2 = Promise.all([
      fetch(`${API}/${projectCode}/tree?ref=main`).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent('docs/' + user)}`).then(r => r.ok ? r.json() : null).catch(() => null),
    ]).then(([mainTree, userTree]) => {
      if (!mainTree?.tree || !userTree?.tree) return false;
      const mainMap = {};
      mainTree.tree.forEach(f => { if (f.type !== 'tree') mainMap[f.path] = f.sha; });
      const userMap = {};
      userTree.tree.forEach(f => { if (f.type !== 'tree') userMap[f.path] = f.sha; });
      const mainPaths = Object.keys(mainMap);
      const userPaths = Object.keys(userMap);
      if (mainPaths.length !== userPaths.length) return true;
      return mainPaths.some(p => mainMap[p] !== userMap[p]);
    }).catch(() => false);

    Promise.all([p1, p2]).then(([serverAlert, hasDiff]) => {
      const alert = { ...serverAlert, jiraCode: projectCode, branchDiff: hasDiff };
      if (!alert.branchDiff && !alert.hasOpenPR) setProjectAlert(null);
      else setProjectAlert(alert);
    });
  }, [projectCode, page, user, PLATFORM_API, GOJIRA_API]);

  const queueRef = useRef(null);
  if (!queueRef.current) queueRef.current = createEventQueue();

  async function compareTree(code, userId) {
    const API = PLATFORM_API + '/DocRepo';
    const [mainTree, userTree] = await Promise.all([
      fetch(`${API}/${code}/tree?ref=main`).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch(`${API}/${code}/tree?ref=${encodeURIComponent('docs/' + userId)}`).then(r => r.ok ? r.json() : null).catch(() => null),
    ]);
    if (!mainTree?.tree || !userTree?.tree) return false;
    const mainMap = {};
    mainTree.tree.forEach(f => { if (f.type !== 'tree') mainMap[f.path] = f.sha; });
    const userMap = {};
    userTree.tree.forEach(f => { if (f.type !== 'tree') userMap[f.path] = f.sha; });
    const mk = Object.keys(mainMap), uk = Object.keys(userMap);
    return mk.length !== uk.length || mk.some(p => mainMap[p] !== userMap[p]);
  }

  useEffect(() => {
    if (!projectCode || page === 'map') return;
    const queue = queueRef.current;

    const unsub = subscribe('doc_update', (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'pr_update' && data.projectCode === projectCode) {
          queue.push(async () => {
            const isReviewer = (data.reviewers || []).includes(user);
            if (['opened', 'reopened', 'review_requested'].includes(data.action) && isReviewer) {
              setProjectAlert(prev => ({ ...prev, jiraCode: projectCode, hasOpenPR: true }));
            } else if (data.action === 'closed' || data.state === 'closed') {
              setProjectAlert(prev => ({ ...prev, jiraCode: projectCode, hasOpenPR: false }));
            }
          });
        }
        if (data.type === 'project_alert' && data.projectCode === projectCode) {
          queue.push(async () => {
            const hasDiff = await compareTree(projectCode, user);
            setProjectAlert(prev => {
              const next = { ...prev, jiraCode: projectCode, branchDiff: hasDiff };
              if (!next.branchDiff && !next.hasOpenPR) return null;
              return next;
            });
          });
        }
      } catch {}
    });

    return () => { unsub(); queue.clear(); };
  }, [projectCode, page, user]); // eslint-disable-line

  async function refreshAlert() {
    if (!projectCode) return;
    const hasDiff = await compareTree(projectCode, user);
    setProjectAlert(prev => {
      const next = { ...prev, jiraCode: projectCode, branchDiff: hasDiff };
      if (!next.branchDiff && !next.hasOpenPR) return null;
      return next;
    });
  }

  let content;
  if (page === 'docs' && selectedProject) {
    content = <DocEditor project={selectedProject} user={user} authority={authority} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} projectAlert={projectAlert} onDocStateChange={refreshAlert} />;
  } else if (page === 'issues' && selectedProject) {
    content = <IssueList project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} projectAlert={projectAlert} />;
  } else if (page === 'relationMap' && selectedProject) {
    content = <RelationMap project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} projectAlert={projectAlert} />;
  } else if (page === 'satam' && selectedProject) {
    content = <SATAM project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} projectAlert={projectAlert} />;
  } else {
    content = (
      <ProjectMap
        onProjectClick={(p) => navigate('issues', p)}
        onRelationMap={(p) => navigate('relationMap', p)}
        onSATAM={(p) => navigate('satam', p)}
        user={user}
      />
    );
  }

  return (
    <>
      {content}
      <AlertModal />
    </>
  );
}
