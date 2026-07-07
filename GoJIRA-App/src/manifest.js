// MF './manifest' (계약 01 §2, D5) — 셸이 기동 시 로드하는 제품 메타데이터.
// 순수 데이터만 (컴포넌트/사이드이펙트 금지 — 초경량 청크 유지).
//   name  : 브랜디드 스코프(/gojira)의 타이틀바 브랜드명
//   icon  : remote base 상대 경로 — 셸이 `${base}/${icon}` 으로 로드 (public/ 자산)
//   menus : 셸 타이틀바에 등록할 메뉴. key 는 셸 page prop / onNavigate 보고 키와
//           같은 네임스페이스. pages 는 이 메뉴를 활성으로 유지할 내부 페이지 키들.
//           authCode/authLevel 을 주면 셸이 권한으로 노출을 거른다.
export default {
  name: 'GoJIRA',
  icon: 'icons/gojira-logo.svg',
  menus: [
    { key: 'map', label: 'Project', pages: ['docs', 'issues', 'relationMap', 'satam'] },
  ],
};
