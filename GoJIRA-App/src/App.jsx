import { useState, useEffect, useCallback } from 'react';
import ProjectMap from './components/ProjectMap';
import IssueList from './components/IssueList';
import RelationMap from './components/RelationMap';
import SATAM from './components/SATAM';
import DocEditor from './components/DocEditor';
import AlertModal from '@platform/web-ui/components/AlertModal';
import NoticePopup from '@platform/web-ui/components/NoticePopup';
import { getAuthUser } from '@platform/web-ui/utils/authUser';
import config from './config';
import './App.css';
import { authFetch } from './platform/auth';

const PLATFORM_API = config.PLATFORM_API;

function App() {
  const [user, setUser] = useState(() => getAuthUser());
  const [authority, setAuthority] = useState(null);
  const [allUsers, setAllUsers] = useState([]);
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [selectedProject, setSelectedProject] = useState(() => {
    try { return JSON.parse(sessionStorage.getItem('gojira_project')); }
    catch { return null; }
  });

  // SSO_SESSION 토큰으로 loginid 복원 (standalone :44301 SSO 진입/새로고침 시).
  // 교환(?auth=1&code=)은 Platform-App 이 담당하므로 여기선 기존 토큰 복원만 한다.
  useEffect(() => {
    if (!config.USE_SSO) return;
    if (user && user !== '__pending__') return;
    const m = document.cookie.match(/(^| )SSO_SESSION=([^;]+)/);
    const sessionToken = m ? decodeURIComponent(m[2]) : null;
    if (!sessionToken) return;
    authFetch(`${config.SSO_URL}/Account/introspect?token=${encodeURIComponent(sessionToken)}`)
      .then(r => (r.ok ? r.json() : null))
      .then(data => {
        if (data?.loginid) setUser(data.loginid);
        else {
          document.cookie = 'SSO_SESSION=; path=/; max-age=0';
          window.location.href = `${config.SSO_URL}/Account/ADFSLogin?client=${window.location.origin}`;
        }
      })
      .catch(() => {});
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // 로그인 시 권한 조회
  useEffect(() => {
    if (!user || user === '__pending__') return;
    authFetch(`${PLATFORM_API}/Account/me?knoxId=${encodeURIComponent(user)}`)
      .then(r => r.json())
      .then(data => {
        setAuthority(data.authority || {});
        if ((data.authority?.USR || 0) >= 15) {
          authFetch(`${PLATFORM_API}/Account/users`).then(r => r.json())
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

  // Agent 독립 창에서 보낸 네비게이션 명령 수신
  useEffect(() => {
    const ch = new BroadcastChannel('gojira-navigate');
    ch.onmessage = (e) => {
      const { projectCode, projectName, page: targetPage } = e.data;
      if (projectCode && targetPage) {
        navigate(targetPage, {
          name: projectName || projectCode,
          jiraCodes: { main: { jiraCode: projectCode } },
        });
      }
    };
    return () => ch.close();
  }, [navigate]);

  const onNavigate = useCallback((target) => {
    navigate(target, selectedProject);
  }, [navigate, selectedProject]);

  if (!user) return null;
  if (user === '__pending__') {
    return <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', color: '#888' }}>인증 확인 중...</div>;
  }

  let content;
  if (page === 'docs' && selectedProject) {
    content = <DocEditor project={selectedProject} user={user} authority={authority} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} />;
  } else if (page === 'issues' && selectedProject) {
    content = <IssueList project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'relationMap' && selectedProject) {
    content = <RelationMap project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else if (page === 'satam' && selectedProject) {
    content = <SATAM project={selectedProject} onBack={() => navigate('map')} currentPage={page} onNavigate={onNavigate} user={user} />;
  } else {
    content = <ProjectMap onProjectClick={(p) => navigate('issues', p)} onRelationMap={(p) => navigate('relationMap', p)} onSATAM={(p) => navigate('satam', p)} user={user} />;
  }

  return (
    <div className="app-root">
      <div className="app-titlebar">
        <img src="/icons/gojira-logo.svg" alt="GoJIRA" className="app-logo-icon" />
        <span className="app-logo">GoJIRA</span>
        <nav className="app-nav">
          <button className={`app-nav-btn app-nav-active`} onClick={() => navigate('map')}>Project</button>
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
      <NoticePopup user={user} />
    </div>
  );
}

export default App;
