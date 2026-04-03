import { useState, useCallback } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';
import AccountManager from './components/AccountManager';
import { getAuthUser } from './utils/auth';
import './App.css';

const TOP_NAV = [
  { key: 'map', label: 'Project' },
  { key: 'account', label: 'Account' },
  { key: 'notice', label: 'Notice' },
];

function App() {
  const [user] = useState(() => getAuthUser());
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
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

  // 상단 메뉴에서 현재 활성 탭 판별 (프로젝트 하위 페이지도 map으로 표시)
  const activeNav = ['map', 'issues', 'relationMap', 'satam'].includes(page) ? 'map' : page;

  if (!user) return null;

  let content;
  if (page === 'issues' && selectedProject) {
    content = <IssueList project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'relationMap' && selectedProject) {
    content = <RelationMap project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'satam' && selectedProject) {
    content = <SATAM project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'account') {
    content = <AccountManager user={user} />;
  } else if (page === 'notice') {
    content = <div className="app-placeholder">Notice 관리 (준비 중)</div>;
  } else {
    content = <ProjectMap onProjectClick={(p) => navigate('issues', p)} onRelationMap={(p) => navigate('relationMap', p)} onSATAM={(p) => navigate('satam', p)} user={user} />;
  }

  return (
    <div className="app-root">
      <div className="app-titlebar">
        <span className="app-logo">GoJIRA</span>
        <nav className="app-nav">
          {TOP_NAV.map((n) => (
            <button
              key={n.key}
              className={`app-nav-btn ${activeNav === n.key ? 'app-nav-active' : ''}`}
              onClick={() => navigate(n.key)}
            >
              {n.label}
            </button>
          ))}
        </nav>
        <span className="app-user">{user}</span>
      </div>
      <div className="app-body">{content}</div>
    </div>
  );
}

export default App;
