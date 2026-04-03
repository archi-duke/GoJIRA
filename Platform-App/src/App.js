import React, { useState, useEffect, useCallback, Suspense } from 'react';
import AccountManager from './components/AccountManager';
import NoticeManager from './components/NoticeManager';
import NoticePopup from './components/NoticePopup';
import DocManager from './components/DocManager';
import { getAuthUser } from './utils/auth';
import appRegistry from './apps/registry';
import config from './config';
import './App.css';

const API_BASE = config.API_BASE;

function App() {
  const [user] = useState(() => getAuthUser());
  const [authority, setAuthority] = useState(null);
  const [page, setPage] = useState(() => sessionStorage.getItem('gojira_page') || 'map');

  useEffect(() => {
    if (!user) return;
    fetch(`${API_BASE}/Account/me?knoxId=${encodeURIComponent(user)}`)
      .then(r => r.json())
      .then(data => setAuthority(data.authority || {}))
      .catch(() => setAuthority({}));
  }, [user]);

  const navigate = useCallback((p) => {
    setPage(p);
    sessionStorage.setItem('gojira_page', p);
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
  } else if (page === 'docs') {
    content = <DocManager user={user} />;
  } else {
    const appEntry = appRegistry.find(a => a.key === page || (a.subPages && a.subPages.includes(page)));
    if (appEntry && appEntry.component) {
      const AppComponent = appEntry.component;
      content = (
        <Suspense fallback={<div className="app-placeholder">로딩 중...</div>}>
          <AppComponent user={user} authority={authority} />
        </Suspense>
      );
    } else {
      content = <div className="app-placeholder">페이지를 찾을 수 없습니다</div>;
    }
  }

  return (
    <div className="app-root">
      <div className="app-titlebar">
        <span className="app-logo">GoJIRA</span>
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
        <span className="app-user">{user}</span>
      </div>
      <div className="app-body">{content}</div>
      <NoticePopup />
    </div>
  );
}

export default App;
