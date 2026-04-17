// 바깥 스크롤바 방지 — CRA webpack 이 CSS !important 를 덮어쓰므로 JS 로 주입
(function(){var s=document.createElement('style');s.id='no-outer-scroll';s.textContent='html,body,#root{overflow:hidden!important;height:100%!important;margin:0!important}';document.head.insertBefore(s,document.head.firstChild)})();
import('./bootstrap');
