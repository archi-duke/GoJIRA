import React, { useState, useCallback, useEffect, useRef } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';
import DocEditor from './components/DocEditor';
import config from './config';

// Platform-App의 sseManager/eventQueue가 없으면 noop
let subscribe, createEventQueue;
try {
  ({ subscribe } = require('./utils/sseManager'));
  ({ createEventQueue } = require('./utils/eventQueue'));
} catch {
  subscribe = () => () => {};
  createEventQueue = () => ({ push: () => {}, clear: () => {} });
}

export default function GoJIRAApp({ user, authority, resetKey }) {
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [projectAlert, setProjectAlert] = useState(null);

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
  const API_BASE = config.API_BASE || config.PLATFORM_API;
  const GOJIRA_API = config.GOJIRA_API || config.API_BASE;

  useEffect(() => {
    if (!projectCode || page === 'map') { setProjectAlert(null); return; }
    const API = API_BASE + '/DocRepo';

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
  }, [projectCode, page, user, API_BASE, GOJIRA_API]);

  const queueRef = useRef(null);
  if (!queueRef.current) queueRef.current = createEventQueue();

  async function compareTree(code, userId) {
    const API = API_BASE + '/DocRepo';
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

  if (page === 'docs' && selectedProject) {
    return <DocEditor project={selectedProject} user={user} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} projectAlert={projectAlert} onDocStateChange={refreshAlert} />;
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
