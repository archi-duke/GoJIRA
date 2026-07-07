import React from 'react';
import { registerRemotes, loadRemote } from '@module-federation/runtime';

// remote URL 은 런타임 결정: window._env_ (컨테이너 env.js) → Vite dev .env → 폴백.
// (플랫폼 통합 계약 docs/platform-contract/01 §5)
function remoteBaseFromEnv(key, fallback = '') {
  const v =
    (typeof window !== 'undefined' && window._env_ && window._env_[key]) ||
    (import.meta.env && import.meta.env[key]) ||
    fallback;
  return v ? v.replace(/\/$/, '') : '';
}

// registerRemotes(force) 는 MF 런타임이 공유 스코프(react 싱글턴 등) 협상을 위해
// remoteEntry.js 를 즉시 fetch 한다 — 즉 "등록 = 연결" 이다 (D6). 그래서 등록을 모듈
// 로드 시 전부 해버리면, 플랫폼 스코프(/) 방문만으로 모든 제품이 조용히 연결된다.
// 대신 제품별로 지연 등록: 그 제품이 실제로 필요해지는 시점(스코프 진입/manifest 조회)에만
// 최초 1회 registerRemotes 한다.
function makeRemoteEntry(name, base) {
  let registered = false;
  let Comp = null;
  return {
    mfName: name,
    base,
    ensureRegistered() {
      if (registered) return;
      registerRemotes([{ name, entry: `${base}/remoteEntry.js`, type: 'module' }], { force: true });
      registered = true;
    },
    getComponent() {
      this.ensureRegistered();
      if (!Comp) Comp = React.lazy(() => loadRemote(`${name}/App`).then((m) => ({ default: m.default ?? m })));
      return Comp;
    },
  };
}

// GoJIRA 앱 — 1호 remote. dev 폴백 :3001.
const gojiraBase = remoteBaseFromEnv('REACT_APP_GOJIRA_APP_REMOTE', 'http://localhost:3001');
const gojiraRemote = makeRemoteEntry('gojiraApp', gojiraBase);

// RAGaaS 앱 — 2호 remote (온보딩 B1/B2). env 미설정이면 탭 자체를 노출하지 않는다 —
// RAGaaS 스택이 배포되기 전의 셸이 죽은 탭을 보여주지 않게 하는 게이트.
// 활성화: 컨테이너 env REACT_APP_RAGAAS_APP_REMOTE (또는 dev .env / public/env.js) 설정.
const ragaasBase = remoteBaseFromEnv('REACT_APP_RAGAAS_APP_REMOTE');
const ragaasRemote = ragaasBase ? makeRemoteEntry('ragaasApp', ragaasBase) : null;

const appRegistry = [
  {
    key: 'map',
    label: 'Project',
    isRemote: true,          // 컨텐츠 렌더링이 remote.getComponent() 로 지연 연결 (D6)
    remote: gojiraRemote,
    authCode: null,          // 전체 접근
    authLevel: 0,
    subPages: ['docs', 'issues', 'relationMap', 'satam'],
    // 브랜디드 스코프 (D5): /<product> 진입 시 이 제품만의 타이틀바를 구성.
    // productLabel 은 manifest(name) 로드 전/실패 시의 브랜드명 폴백.
    product: 'gojira',
    productLabel: 'GoJIRA',
    mfName: 'gojiraApp',
    base: gojiraBase,
    // 플랫폼 스코프 홈(D6) 카드 아이콘 — 셸 자체 자산이라 remote 연결 없이 표시 가능.
    localIcon: '/icons/gojira-logo.svg',
  },
  // RAGaaS — env 로 remote 가 지정된 경우에만 등록 (미배포 환경에서 탭 숨김)
  ...(ragaasRemote
    ? [{
        key: 'ragaas',
        label: 'RAGaaS',
        isRemote: true,
        remote: ragaasRemote,
        authCode: null,      // 알파: 전체 접근. 권한 게이트가 필요해지면 authCode 신설(온보딩 B6)
        authLevel: 0,
        product: 'ragaas',
        productLabel: 'RAGaaS',
        mfName: 'ragaasApp',
        base: ragaasBase,
      }]
    : []),
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

/**
 * remote 의 './manifest' (선택 expose, 계약 01 §2) 로드 — { name, icon, menus } 메타.
 * 미노출/로드 실패는 null (셸은 registry 의 productLabel/단일 탭으로 폴백).
 */
export async function loadAppManifest(entry) {
  if (!entry || !entry.remote) return null;
  try {
    entry.remote.ensureRegistered();
    const m = await loadRemote(`${entry.mfName}/manifest`);
    return (m && (m.default ?? m)) || null;
  } catch {
    return null;
  }
}
