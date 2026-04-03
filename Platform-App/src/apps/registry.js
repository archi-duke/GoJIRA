import React from 'react';

// GoJIRA 앱 컴포넌트 (동적 import)
const GoJIRAApp = React.lazy(() => import('./GoJIRAApp'));

const appRegistry = [
  {
    key: 'map',
    label: 'Project',
    component: GoJIRAApp,
    authCode: null,          // 전체 접근
    authLevel: 0,
    subPages: ['issues', 'relationMap', 'satam'],
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
    authCode: 'NOTMGR',
    authLevel: 15,
  },
  {
    key: 'docs',
    label: 'Docs',
    component: null,         // 내장 컴포넌트 (DocManager)
    authCode: 'GITRP',
    authLevel: 4,            // 조회 이상
  },
];

export default appRegistry;
