import config from '../config';

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

/**
 * 사용자 인증 확인.
 * - USE_SSO=true: SSO_SESSION 토큰 쿠키 확인. 없으면 SSO 리다이렉트, 있으면 '__pending__'
 *   (실제 loginid 는 App.js 의 introspect 가 세팅). 신원을 쿠키에 평문 보관하지 않는다.
 * - USE_SSO=false: 고정 DEV_USER 반환 (개발용)
 * @returns {string|null} loginid | '__pending__'(introspect 대기) | null(리다이렉트)
 */
export function getAuthUser() {
  if (!config.USE_SSO) {
    return config.DEV_USER;
  }

  const sessionToken = getCookie('SSO_SESSION');
  if (!sessionToken) {
    window.location.href = `${config.SSO_URL}/Account/ADFSLogin?client=${window.location.origin}`;
    return null;
  }
  // 토큰만 있고 신원 미확정 — App.js 의 introspect useEffect 가 실제 loginid 로 교체.
  return '__pending__';
}
