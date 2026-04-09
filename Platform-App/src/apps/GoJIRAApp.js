import React, { useState, useCallback, useEffect } from 'react';
import ProjectMap from './gojira/ProjectMap';
import IssueList from './gojira/IssueList';
import RelationMap from './gojira/RelationMap';
import SATAM from './gojira/SATAM';
import DocEditor from '../components/DocEditor';
import { subscribe } from '../utils/sseManager';
import config from '../config';

export default function GoJIRAApp({ user, authority, resetKey }) {
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [projectAlert, setProjectAlert] = useState(null); // { branchDiff, hasOpenPR }

  // 타이틀바 Project 클릭 시 map으로 리셋
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

  const onNavigate = useCallback((target) => {
    navigate(target, selectedProject);
  }, [navigate, selectedProject]);

  const projectCode = selectedProject?.jiraCodes?.main?.jiraCode || '';

  // 선택된 프로젝트의 alert 초기 로드 (서버 alerts + 프론트 트리 비교)
  useEffect(() => {
    if (!projectCode || page === 'map') { setProjectAlert(null); return; }
    const API = config.API_BASE + '/DocRepo';

    // 1. 서버 alerts (PR 리뷰어 등)
    const p1 = fetch(`${config.GOJIRA_API}/Project/alerts?user=${encodeURIComponent(user || '')}`)
      .then(r => r.json())
      .then(arr => {
        if (Array.isArray(arr)) return arr.find(a => a.jiraCode === projectCode) || null;
        return null;
      }).catch(() => null);

    // 2. 프론트 트리 비교 (main vs user branch)
    const p2 = Promise.all([
      fetch(`${API}/${projectCode}/tree?ref=main`).then(r => r.ok ? r.json() : null).catch(() => null),
      fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent('docs/' + user)}`).then(r => r.ok ? r.json() : null).catch(() => null),
    ]).then(([mainTree, userTree]) => {
      if (!mainTree?.tree || !userTree?.tree) return false;
      const mainMap = {};
      mainTree.tree.forEach(f => { if (f.type !== 'tree') mainMap[f.path] = f.sha; });
      const userMap = {};
      userTree.tree.forEach(f => { if (f.type !== 'tree') userMap[f.path] = f.sha; });
      // 파일 수가 다르거나 SHA가 다르면 diff
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
  }, [projectCode, page, user]);

  // SSE: 프로젝트 알림 실시간 갱신
  useEffect(() => {
    if (!projectCode || page === 'map') return;
    const unsub = subscribe('doc_update', (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'project_alert' && data.projectCode === projectCode) {
          // main 변경 → 트리 재비교
          const API = config.API_BASE + '/DocRepo';
          Promise.all([
            fetch(`${API}/${projectCode}/tree?ref=main`).then(r => r.ok ? r.json() : null).catch(() => null),
            fetch(`${API}/${projectCode}/tree?ref=${encodeURIComponent('docs/' + user)}`).then(r => r.ok ? r.json() : null).catch(() => null),
          ]).then(([mainTree, userTree]) => {
            let hasDiff = false;
            if (mainTree?.tree && userTree?.tree) {
              const mainMap = {};
              mainTree.tree.forEach(f => { if (f.type !== 'tree') mainMap[f.path] = f.sha; });
              const userPaths = {};
              userTree.tree.forEach(f => { if (f.type !== 'tree') userPaths[f.path] = f.sha; });
              const mk = Object.keys(mainMap), uk = Object.keys(userPaths);
              hasDiff = mk.length !== uk.length || mk.some(p => mainMap[p] !== userPaths[p]);
            }
            setProjectAlert(prev => {
              const next = { ...prev, jiraCode: projectCode, branchDiff: hasDiff };
              if (!next.branchDiff && !next.hasOpenPR) return null;
              return next;
            });
          }).catch(() => {});
        }
        if (data.type === 'pr_update' && data.projectCode === projectCode) {
          const isReviewer = (data.reviewers || []).includes(user);
          if (['opened', 'reopened', 'review_requested'].includes(data.action) && isReviewer) {
            setProjectAlert(prev => ({ ...prev, jiraCode: projectCode, hasOpenPR: true }));
          } else if (data.action === 'closed' || data.state === 'closed') {
            // 리뷰어만 해제
            if (!isReviewer || data.action === 'closed') {
              setProjectAlert(prev => {
                const next = { ...prev, hasOpenPR: false };
                if (!next.branchDiff && !next.hasOpenPR) return null;
                return next;
              });
            }
          }
        }
      } catch {}
    });
    return unsub;
  }, [projectCode, page, user]);

  if (page === 'docs' && selectedProject) {
    return <DocEditor project={selectedProject} user={user} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} projectAlert={projectAlert} />;
  }
  if (page === 'issues' && selectedProject) {
    return <IssueList project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} projectAlert={projectAlert} />;
  }
  if (page === 'relationMap' && selectedProject) {
    return <RelationMap project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} projectAlert={projectAlert} />;
  }
  if (page === 'satam' && selectedProject) {
    return <SATAM project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} projectAlert={projectAlert} />;
  }

  return (
    <ProjectMap
      onProjectClick={(p) => navigate('issues', p)}
      onRelationMap={(p) => navigate('relationMap', p)}
      onSATAM={(p) => navigate('satam', p)}
      user={user}
    />
  );
}
