import { useState, useEffect, useCallback } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';
import AccountManager from './components/AccountManager';
import NoticeManager from './components/NoticeManager';
import NoticePopup from './components/NoticePopup';
import { getAuthUser } from './utils/auth';
import config from './config';
import './App.css';

const API_BASE = config.API_BASE;

function App() {
  const [user] = useState(() => getAuthUser());
  const [authority, setAuthority] = useState(null);
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [selectedProject, setSelectedProject] = useState(() => {
    try { return JSON.parse(sessionStorage.getItem('gojira_project')); }
    catch { return null; }
  });

  // 로그인 시 권한 조회
  useEffect(() => {
    if (!user) return;
    fetch(`${API_BASE}/Account/me?knoxId=${encodeURIComponent(user)}`)
      .then(r => r.json())
      .then(data => setAuthority(data.authority || {}))
      .catch(() => setAuthority({}));
  }, [user]);

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

  const activeNav = ['map', 'issues', 'relationMap', 'satam'].includes(page) ? 'map' : page;

  // USR 권한 15 = CRUD 전체 → Account/Notice 메뉴 접근 허용
  const hasAccountAdmin = authority && (authority.USR || 0) >= 15;
  const hasNoticeAdmin = authority && (authority.NOTMGR || 0) >= 15;

  if (!user) return null;

  // 권한 없이 접근 시도 시 map으로 리다이렉트
  if (page === 'account' && authority !== null && !hasAccountAdmin) navigate('map');
  if (page === 'notice' && authority !== null && !hasNoticeAdmin) navigate('map');

  const navItems = [
    { key: 'map', label: 'Project' },
    ...(hasAccountAdmin ? [{ key: 'account', label: 'Account' }] : []),
    ...(hasNoticeAdmin ? [{ key: 'notice', label: 'Notice' }] : []),
  ];

  let content;
  if (page === 'issues' && selectedProject) {
    content = <IssueList project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'relationMap' && selectedProject) {
    content = <RelationMap project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'satam' && selectedProject) {
    content = <SATAM project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'account' && hasAccountAdmin) {
    content = <AccountManager user={user} />;
  } else if (page === 'notice' && hasNoticeAdmin) {
    content = <NoticeManager user={user} />;
  } else {
    content = <ProjectMap onProjectClick={(p) => navigate('issues', p)} onRelationMap={(p) => navigate('relationMap', p)} onSATAM={(p) => navigate('satam', p)} user={user} />;
  }

  return (
    <div className="app-root">
      <div className="app-titlebar">
        <span className="app-logo">GoJIRA</span>
        <nav className="app-nav">
          {navItems.map((n) => (
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
      <NoticePopup />
    </div>
  );
}

export default App;
