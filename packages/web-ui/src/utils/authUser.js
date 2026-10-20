import { USE_SSO, DEV_USER, SSO_URL } from '../runtime/env';

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

// SSO 콜백 진입인지 — ADFSLogin 이 <origin>?auth=1&code=... 로 돌려보낸 상태.
// 이 시점엔 아직 쿠키가 없다(교환 전). App 의 콜백 useEffect 가 code 를 교환해
// SSO_SESSION 을 심으므로, 여기서 쿠키 없음을 보고 재로그인시키면 안 된다.
function isSsoCallback() {
  if (typeof window === 'undefined') return false;
  const p = new URLSearchParams(window.location.search);
  return !!(p.get('auth') && p.get('code'));
}

/**
 * 사용자 인증 확인.
 * - USE_SSO=true: SSO_SESSION 토큰 쿠키 확인. 없으면 SSO 리다이렉트, 있으면 '__pending__'
 *   (실제 loginid 는 App 의 introspect 가 세팅). 신원을 쿠키에 평문 보관하지 않는다.
 * - USE_SSO=false: 고정 DEV_USER 반환 (개발용)
 * @returns {string|null} loginid | '__pending__'(introspect 대기) | null(리다이렉트)
 */
export function getAuthUser() {
  if (!USE_SSO) {
    return DEV_USER;
  }

  // 콜백 진입은 쿠키가 없는 게 정상 — 교환을 기다린다. 이 분기가 없으면 렌더 중에
  // ADFSLogin 으로 재이동해 방금 받은 code 를 버리게 된다. 그 재로그인은 사용자
  // 지정(?user=)이 없는 기본 URL 이라, 개발용 사용자 전환이 직전 사용자로 되돌아간다.
  if (isSsoCallback()) {
    return '__pending__';
  }

  const sessionToken = getCookie('SSO_SESSION');
  if (!sessionToken) {
    window.location.href = `${SSO_URL}/Account/ADFSLogin?client=${window.location.origin}`;
    return null;
  }
  // 토큰만 있고 신원 미확정 — App 의 introspect useEffect 가 실제 loginid 로 교체.
  return '__pending__';
}
