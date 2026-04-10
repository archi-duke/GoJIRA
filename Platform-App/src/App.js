import React, { useState, useEffect, useCallback, Suspense } from 'react';
import AccountManager from './components/AccountManager';
import NoticeManager from './components/NoticeManager';
import NoticePopup from './components/NoticePopup';
import DocManager from './components/DocManager';
import AlertModal from './components/AlertModal';
import AgentChat from './components/AgentChat';
import { getAuthUser } from './utils/auth';
import { subscribe, onStatus } from './utils/sseManager';
import { modalAlert, modalConfirm } from './components/AlertModal';
import appRegistry from './apps/registry';
import config from './config';
import './App.css';

const API_BASE = config.API_BASE;

function App() {
  const [user, setUser] = useState(() => getAuthUser());
  const [authority, setAuthority] = useState(null);
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [allUsers, setAllUsers] = useState([]);
  const [mapResetCount, setMapResetCount] = useState(0);
  const [sseConnected, setSseConnected] = useState(false);
  const agentWindowRef = React.useRef(null);

  // SSE 연결 상태 모니터링
  useEffect(() => {
    return onStatus(setSseConnected);
  }, []);

  // Agent 창에서 참조할 설정을 window에 노출
  useEffect(() => {
    window.__GOJIRA_CONFIG__ = {
      GOJIRA_API: config.GOJIRA_API || config.API_BASE,
      AGENT_API: config.AGENT_API,
      user: user,
      isMaster: authority && (authority.USR || 0) >= 15,
    };
  }, [user, authority]);

  // Agent 창 열기
  function openAgentWindow() {
    if (agentWindowRef.current && !agentWindowRef.current.closed) {
      agentWindowRef.current.focus();
      return;
    }
    agentWindowRef.current = window.open(
      '/agent-chat.html',
      'GoJIRA_Agent',
      'width=480,height=700,resizable=yes,scrollbars=yes'
    );
  }

  // Agent 창에서 보내는 navigate 메시지 수신
  useEffect(() => {
    function handleMessage(e) {
      if (e.origin !== window.location.origin) return;
      const { type, projectCode, page: targetPage } = e.data || {};
      if (type !== 'agent_navigate' || !projectCode || !targetPage) return;

      // GoJIRAApp 내부 네비게이션을 위해 sessionStorage에 저장 후 페이지 이동
      // GoJIRAApp은 sessionStorage에서 gojira_page, gojira_project를 읽어서 초기 상태를 결정
      // project 정보를 찾아야 함
      fetch(`${config.GOJIRA_API}/Project?jiraCode=${encodeURIComponent(projectCode)}`)
        .then(r => r.json())
        .then(projects => {
          const prj = Array.isArray(projects) ? projects[0] : projects;
          if (prj) {
            sessionStorage.setItem('gojira_page', targetPage);
            sessionStorage.setItem('gojira_project', JSON.stringify(prj));
            // App.js의 page가 GoJIRA 앱이 아닐 수 있으므로 먼저 GoJIRA 앱으로 이동
            navigate('map');
            // 약간의 지연 후 GoJIRAApp 내부 페이지로 이동
            setTimeout(() => {
              setPage(targetPage);
              sessionStorage.setItem('gojira_page', targetPage);
            }, 100);
          }
        })
        .catch(() => {});
    }
    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [navigate]);

  useEffect(() => {
    if (!user) return;
    fetch(`${API_BASE}/Account/me?knoxId=${encodeURIComponent(user)}`)
      .then(r => r.json())
      .then(data => {
        setAuthority(data.authority || {});
        // 마스터 그룹이면 전체 사용자 목록 로드
        if ((data.authority?.USR || 0) >= 15) {
          fetch(`${API_BASE}/Account/users`)
            .then(r => r.json())
            .then(d => setAllUsers(Array.isArray(d) ? d : []));
        }
      })
      .catch(() => setAuthority({}));
  }, [user]);

  // 접속 시 미읽은 알림 확인 (반려 등)
  useEffect(() => {
    if (!user) return;
    fetch(`${API_BASE}/Notification/unread?user=${encodeURIComponent(user)}`)
      .then(r => r.json())
      .then(async (notifs) => {
        if (!Array.isArray(notifs) || notifs.length === 0) return;
        for (const n of notifs) {
          if (n.type === 'pr_rejected') {
            const ok = await modalConfirm(
              `${n.message}\n\n변경 내용을 보관하고 동기화 후 재반영하시겠습니까?`
            );
            if (ok && n.projectCode) {
              const res = await fetch(`${API_BASE}/DocRepo/${n.projectCode}/conflict-resolve`, {
                method: 'POST', headers: { 'X-User-Id': user },
              });
              const result = await res.json().catch(() => ({}));
              if (result.resolved) {
                modalAlert(`${result.applied?.length || 0}개 파일이 재반영되었습니다.`);
              } else {
                modalAlert(result.message || '재반영할 변경사항이 없습니다.');
              }
            }
          }
        }
        // 알림 읽음 처리
        fetch(`${API_BASE}/Notification/read?user=${encodeURIComponent(user)}`, { method: 'POST' });
      }).catch(() => {});
  }, [user]);

  // SSE: PR 반려 실시간 수신
  useEffect(() => {
    if (!user) return;
    return subscribe('doc_update', async (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'pr_review' && data.state === 'changes_requested' && data.prCreator === user) {
          const ok = await modalConfirm(
            `PR #${data.prNumber}이 ${data.reviewer}님에 의해 반려되었습니다.\n사유: ${data.body || '(없음)'}\n\n변경 내용을 보관하고 동기화 후 재반영하시겠습니까?`
          );
          if (ok && data.projectCode) {
            const res = await fetch(`${API_BASE}/DocRepo/${data.projectCode}/conflict-resolve`, {
              method: 'POST', headers: { 'X-User-Id': user },
            });
            const result = await res.json().catch(() => ({}));
            if (result.resolved) {
              modalAlert(`${result.applied?.length || 0}개 파일이 재반영되었습니다.`);
            } else {
              modalAlert(result.message || '재반영할 변경사항이 없습니다.');
            }
          }
        }
      } catch {}
    });
  }, [user]);

  function switchUser(knoxId) {
    document.cookie = `ADSSO_USER=${encodeURIComponent(knoxId)}; path=/; max-age=${60*60*24*365}`;
    setUser(knoxId);
    setAuthority(null);
    setPage('map');
    sessionStorage.setItem('gojira_page', 'map');
  }

  const navigate = useCallback((p) => {
    setPage(p);
    sessionStorage.setItem('gojira_page', p);
    if (p === 'map') setMapResetCount(c => c + 1);
  }, []);

  if (!user) return null;

  // 권한 기반 메뉴 필터링
  const visibleApps = appRegistry.filter(app => {
    if (!app.authCode) return true;
    return authority && (authority[app.authCode] || 0) >= app.authLevel;
  });

  // 현재 활성 탭 판별
  const activeKey = appRegistry.find(app =>
    app.key === page || (app.subPages && app.subPages.includes(page))
  )?.key || 'map';

  // 권한 미달 시 리다이렉트
  const currentApp = appRegistry.find(a => a.key === page || (a.subPages && a.subPages.includes(page)));
  if (currentApp && currentApp.authCode && authority !== null) {
    if ((authority[currentApp.authCode] || 0) < currentApp.authLevel) {
      navigate('map');
    }
  }

  // 컨텐츠 렌더링
  let content;
  if (page === 'account') {
    content = <AccountManager user={user} />;
  } else if (page === 'notice') {
    content = <NoticeManager user={user} />;
  } else if (page === 'repository') {
    content = <DocManager user={user} />;
  } else {
    const appEntry = appRegistry.find(a => a.key === page || (a.subPages && a.subPages.includes(page)));
    if (appEntry && appEntry.component) {
      const AppComponent = appEntry.component;
      content = (
        <Suspense fallback={<div className="app-placeholder">로딩 중...</div>}>
          <AppComponent user={user} authority={authority} resetKey={mapResetCount} />
        </Suspense>
      );
    } else {
      content = <div className="app-placeholder">페이지를 찾을 수 없습니다</div>;
    }
  }

  return (
    <div className="app-root">
      <div className="app-titlebar">
        <img src="/icons/gojira-logo.svg" alt="GoJIRA" className="app-logo-icon" style={{ cursor: 'pointer' }} onClick={openAgentWindow} />
        <span className="app-logo" style={{ cursor: 'pointer' }} onClick={openAgentWindow}>GoJIRA</span>
        <nav className="app-nav">
          {visibleApps.map((app) => (
            <button
              key={app.key}
              className={`app-nav-btn ${activeKey === app.key ? 'app-nav-active' : ''}`}
              onClick={() => navigate(app.key)}
            >
              {app.label}
            </button>
          ))}
        </nav>
        <span className="app-sse-indicator" title={sseConnected ? 'SSE 연결됨' : 'SSE 연결 끊김'}
          style={{display:'inline-block',width:8,height:8,borderRadius:'50%',background:sseConnected?'#27ae60':'#aaa',marginRight:6,verticalAlign:'middle'}} />
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
      <NoticePopup user={user} />
      <AlertModal />
      {/* Agent 채팅은 별도 윈도우로 분리됨 (openAgentWindow) */}
    </div>
  );
}

export default App;
