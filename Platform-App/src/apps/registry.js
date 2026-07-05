import React from 'react';
import { registerRemotes, loadRemote } from '@module-federation/runtime';

// GoJIRA 앱 — Module Federation 동적 로딩 (Remote: gojiraApp)
// remote URL 은 런타임 결정: window._env_ (컨테이너 env.js) → Vite dev .env → localhost 폴백.
// vite.config.ts 에 선언된 entry 는 초기값일 뿐, 여기서 force 재등록해 덮어쓴다.
// (플랫폼 통합 계약 docs/platform-contract/01 §5)
const remoteBase = (
  (typeof window !== 'undefined' && window._env_ && window._env_.REACT_APP_GOJIRA_APP_REMOTE) ||
  (import.meta.env && import.meta.env.REACT_APP_GOJIRA_APP_REMOTE) ||
  'http://localhost:3001'
).replace(/\/$/, '');

registerRemotes(
  [{ name: 'gojiraApp', entry: `${remoteBase}/remoteEntry.js`, type: 'module' }],
  { force: true }
);

const GoJIRAApp = React.lazy(() =>
  loadRemote('gojiraApp/App').then((m) => ({ default: m.default ?? m }))
);

const appRegistry = [
  {
    key: 'map',
    label: 'Project',
    component: GoJIRAApp,
    authCode: null,          // 전체 접근
    authLevel: 0,
    subPages: ['docs', 'issues', 'relationMap', 'satam'],
  },
  {
    key: 'account',
    label: 'Account',
    component: null,         // 내장 컴포넌트 (AccountManager)
    authCode: 'USR',
    authLevel: 15,
  },
  {
    key: 'notice',
    label: 'Notice',
    component: null,         // 내장 컴포넌트 (NoticeManager)
    authCode: 'NOTICE',
    authLevel: 15,
  },
  {
    key: 'repository',
    label: 'Repository',
    component: null,         // 내장 컴포넌트 (DocManager)
    authCode: 'GITRP',
    authLevel: 4,            // 조회 이상
  },
];

export default appRegistry;
