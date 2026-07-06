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

// remote 를 MF 런타임에 등록하고 lazy 컴포넌트를 만든다.
// vite.config.ts 에 선언된 entry 는 초기값일 뿐, 여기서 force 재등록해 덮어쓴다.
function makeRemote(name, base) {
  registerRemotes(
    [{ name, entry: `${base}/remoteEntry.js`, type: 'module' }],
    { force: true }
  );
  return React.lazy(() => loadRemote(`${name}/App`).then((m) => ({ default: m.default ?? m })));
}

// GoJIRA 앱 — 1호 remote. dev 폴백 :3001.
const gojiraBase = remoteBaseFromEnv('REACT_APP_GOJIRA_APP_REMOTE', 'http://localhost:3001');
const GoJIRAApp = makeRemote('gojiraApp', gojiraBase);

// RAGaaS 앱 — 2호 remote (온보딩 B1/B2). env 미설정이면 탭 자체를 노출하지 않는다 —
// RAGaaS 스택이 배포되기 전의 셸이 죽은 탭을 보여주지 않게 하는 게이트.
// 활성화: 컨테이너 env REACT_APP_RAGAAS_APP_REMOTE (또는 dev .env / public/env.js) 설정.
const ragaasBase = remoteBaseFromEnv('REACT_APP_RAGAAS_APP_REMOTE');
const RagaasApp = ragaasBase ? makeRemote('ragaasApp', ragaasBase) : null;

const appRegistry = [
  {
    key: 'map',
    label: 'Project',
    component: GoJIRAApp,
    authCode: null,          // 전체 접근
    authLevel: 0,
    subPages: ['docs', 'issues', 'relationMap', 'satam'],
    // 브랜디드 스코프 (D5): /<product> 진입 시 이 제품만의 타이틀바를 구성.
    // productLabel 은 manifest(name) 로드 전/실패 시의 브랜드명 폴백.
    product: 'gojira',
    productLabel: 'GoJIRA',
    mfName: 'gojiraApp',
    base: gojiraBase,
  },
  // RAGaaS — env 로 remote 가 지정된 경우에만 등록 (미배포 환경에서 탭 숨김)
  ...(RagaasApp
    ? [{
        key: 'ragaas',
        label: 'RAGaaS',
        component: RagaasApp,
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
  if (!entry || !entry.mfName) return null;
  try {
    const m = await loadRemote(`${entry.mfName}/manifest`);
    return (m && (m.default ?? m)) || null;
  } catch {
    return null;
  }
}
