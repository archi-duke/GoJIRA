import { useState, useEffect, useCallback } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';
import DocEditor from './components/DocEditor';
import AlertModal from './components/AlertModal';
import AccountManager from './components/AccountManager';
import NoticeManager from './components/NoticeManager';
import NoticePopup from './components/NoticePopup';
import AgentChat from './components/AgentChat';
import { getAuthUser } from './utils/auth';
import config from './config';
import './App.css';

const PLATFORM_API = config.PLATFORM_API;

function App() {
  const [user, setUser] = useState(() => getAuthUser());
  const [authority, setAuthority] = useState(null);
  const [allUsers, setAllUsers] = useState([]);
  const [showAgent, setShowAgent] = useState(false);
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [selectedProject, setSelectedProject] = useState(() => {
    try { return JSON.parse(sessionStorage.getItem('gojira_project')); }
    catch { return null; }
  });

  // 로그인 시 권한 조회
  useEffect(() => {
    if (!user) return;
    fetch(`${PLATFORM_API}/Account/me?knoxId=${encodeURIComponent(user)}`)
      .then(r => r.json())
      .then(data => {
        setAuthority(data.authority || {});
        if ((data.authority?.USR || 0) >= 15) {
          fetch(`${PLATFORM_API}/Account/users`).then(r => r.json())
            .then(d => setAllUsers(Array.isArray(d) ? d : []));
        }
      })
      .catch(() => setAuthority({}));
  }, [user]);

  function switchUser(knoxId) {
    setUser(knoxId);
    setAuthority(null);
    setAllUsers([]);
    setPage('map');
    setSelectedProject(null);
    sessionStorage.setItem('gojira_page', 'map');
    sessionStorage.removeItem('gojira_project');
  }

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

  const activeNav = ['map', 'issues', 'docs', 'relationMap', 'satam'].includes(page) ? 'map' : page;

  // USR 권한 15 = CRUD 전체 → Account/Notice 메뉴 접근 허용
  const hasAccountAdmin = authority && (authority.USR || 0) >= 15;
  const hasNoticeAdmin = authority && (authority.NOTICE || 0) >= 15;

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
  if (page === 'docs' && selectedProject) {
    content = <DocEditor project={selectedProject} user={user} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} />;
  } else if (page === 'issues' && selectedProject) {
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
        <img src="/icons/gojira-logo.svg" alt="GoJIRA" className="app-logo-icon" style={{ cursor: 'pointer' }} onClick={() => setShowAgent(true)} />
        <span className="app-logo" style={{ cursor: 'pointer' }} onClick={() => setShowAgent(true)}>GoJIRA</span>
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
        {allUsers.length > 0 ? (
          <select
            className="app-user-switch"
            value={user}
            onChange={e => switchUser(e.target.value)}
            title="개발용: 사용자 전환"
          >
            {allUsers.map(u => (
              <option key={u.knoxId} value={u.knoxId}>{u.knoxId} ({u.name})</option>
            ))}
          </select>
        ) : (
          <span className="app-user">{user}</span>
        )}
      </div>
      <div className="app-body">{content}</div>
      <AlertModal />
      <NoticePopup />
      {showAgent && <AgentChat user={user} context={{ page }} onClose={() => setShowAgent(false)} />}
    </div>
  );
}

export default App;
