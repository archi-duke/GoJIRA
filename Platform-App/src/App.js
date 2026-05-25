import React, { useState, useEffect, useCallback, Suspense, Component } from 'react';
import AccountManager from './components/AccountManager';
import NoticeManager from './components/NoticeManager';
import NoticePopup from './components/NoticePopup';
import DocManager from './components/DocManager';
import AlertModal from 'gojira-shared-web/components/AlertModal';
import { getAuthUser } from './utils/auth';
import { subscribe, onStatus } from './utils/sseManager';
import { modalAlert, modalConfirm } from 'gojira-shared-web/components/AlertModal';
import appRegistry from './apps/registry';
import config from './config';
import { detectLauncher, requestChat } from './utils/launcher';
import './App.css';

const API_BASE = config.PLATFORM_API;

class RemoteErrorBoundary extends Component {
  state = { hasError: false };
  static getDerivedStateFromError() { return { hasError: true }; }
  render() {
    if (this.state.hasError) {
      return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#888', gap: 12 }}>
          <span style={{ fontSize: 40 }}>⚠</span>
          <span style={{ fontSize: 15, fontWeight: 600 }}>GoJIRA-App에 연결할 수 없습니다</span>
          <span style={{ fontSize: 13 }}>GoJIRA-App (:3001)이 실행 중인지 확인해주세요</span>
          <button onClick={() => this.setState({ hasError: false })}
            style={{ marginTop: 8, padding: '6px 16px', border: 'none', background: '#ddd', borderRadius: 6, cursor: 'pointer', fontSize: 13 }}>
            다시 시도
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

function App() {
  const [user, setUser] = useState(() => getAuthUser());
  const [authority, setAuthority] = useState(null);
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');
  const [allUsers, setAllUsers] = useState([]);
  const [mapResetCount, setMapResetCount] = useState(0);
  const [sseConnected, setSseConnected] = useState(false);

  // SSE 연결 상태 모니터링
  useEffect(() => {
    return onStatus(setSseConnected);
  }, []);

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

  // 로고/타이틀 클릭 — 활성 프로젝트가 있으면 그 로컬 clone 에서, 없으면 일반 repoRoot 에서 에이전트 spawn.
  // 활성 프로젝트는 GoJIRA-App 이 진입 시 sessionStorage.gojira_project 에 저장 (같은 window 공유).
  // 런처 /chat 이 code 가 오면 EnsureRepo 로 clone 없으면 생성한 뒤 그 디렉토리에서 띄운다.
  const launchAgentChat = useCallback(async () => {
    const r = await detectLauncher();
    if (!r?.ready) {
      modalAlert('로컬 런처가 감지되지 않았습니다.\n프로젝트 화면의 "로컬 수정" 버튼을 클릭하면 설치 안내가 표시됩니다.');
      return;
    }
    let code = '';
    try {
      const proj = JSON.parse(sessionStorage.getItem('gojira_project') || 'null');
      code = proj?.jiraCodes?.main?.jiraCode || '';
    } catch {}
    const res = await requestChat({ code, user, token: r.token });
    if (!res.ok) modalAlert(res.error || '에이전트 실행에 실패했습니다.');
  }, [user]);

  // Agent 독립 창(GoJIRA-App :3001)에서 보낸 네비게이션 명령 수신
  useEffect(() => {
    function handleMessage(e) {
      if (e.data?.type === 'gojira-navigate' && e.data.projectCode && e.data.page) {
        navigate(e.data.page);
      }
    }
    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [navigate]);

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
        <RemoteErrorBoundary>
          <Suspense fallback={<div className="app-placeholder">로딩 중...</div>}>
            <AppComponent user={user} authority={authority} resetKey={mapResetCount} />
          </Suspense>
        </RemoteErrorBoundary>
      );
    } else {
      content = <div className="app-placeholder">페이지를 찾을 수 없습니다</div>;
    }
  }

  return (
    <div className="app-root">
      <div className="app-titlebar">
        <img src="/icons/gojira-logo.svg" alt="GoJIRA" className="app-logo-icon app-logo-clickable"
          onClick={launchAgentChat}
          title="에이전트 채팅 띄우기" />
        <span className="app-logo app-logo-clickable"
          onClick={launchAgentChat}
          title="에이전트 채팅 띄우기">GoJIRA</span>
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
    </div>
  );
}

export default App;
