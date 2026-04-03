import config from '../config';

function getCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

/**
 * 사용자 인증 확인.
 * - USE_SSO=true: ADSSO_USER 쿠키 확인, 없으면 SSO 리다이렉트
 * - USE_SSO=false: 고정 사용자 ID 반환 (개발용)
 * @returns {string|null} 인증된 사용자 ID, 리다이렉트 시 null
 */
export function getAuthUser() {
  if (!config.USE_SSO) {
    return config.DEV_USER;
  }

  const uinfo = getCookie('ADSSO_USER');
  if (!uinfo) {
    window.location.href = `${config.SSO_URL}/Account/ADFSLogin?client=${window.location.origin}`;
    return null;
  }
  return uinfo;
}
