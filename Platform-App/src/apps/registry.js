import React from 'react';

// GoJIRA 앱 — Module Federation 동적 로딩 (Remote: gojiraApp@:3001)
const GoJIRAApp = React.lazy(() => import('gojiraApp/GoJIRAApp'));

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
