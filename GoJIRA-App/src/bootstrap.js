import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';

// 바깥 스크롤바 방지 — CRA/webpack 이 index.html 수정을 무시하므로 여기서 주입
(function() {
  var s = document.createElement('style');
  s.id = 'no-outer-scroll';
  s.textContent = 'html,body,#root{overflow:hidden!important;height:100%!important;margin:0!important}';
  document.head.insertBefore(s, document.head.firstChild);
})();

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
