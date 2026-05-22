require('dotenv').config();
const { ModuleFederationPlugin } = require('webpack').container;

// Module Federation remote URL 을 빌드시점에 inline 하지 않고 런타임 결정.
//
// "promise " 접두사로 시작하는 remote 문자열은 webpack 5 MF 가 JS 코드로 평가해
// container proxy({get, init}) 를 resolve 해주는 표준 패턴이다.
// 빌드된 번들은 host-agnostic — window._env_.REACT_APP_GOJIRA_APP_REMOTE 로
// remote 호스트를 결정하므로 폐쇄망에서 .env 만 바꿔도 즉시 반영.
//
// publicPath: 'auto' 가 양쪽에 이미 설정돼 있어 remote 의 chunk/asset 도
// remoteEntry.js 가 로드된 origin 기준으로 자동 추론된다 (chunk path inline X).
const dynamicRemote = `promise new Promise(function(resolve){
  var url = (typeof window !== 'undefined' && window._env_ && window._env_.REACT_APP_GOJIRA_APP_REMOTE)
    || (typeof process !== 'undefined' && process.env && process.env.REACT_APP_GOJIRA_APP_REMOTE)
    || 'http://localhost:3001';
  var s = document.createElement('script');
  s.src = url.replace(/\\/$/, '') + '/remoteEntry.js';
  s.onload = function(){
    resolve({
      get: function(req){ return window.gojiraApp.get(req); },
      init: function(arg){
        try { return window.gojiraApp.init(arg); }
        catch(e) { /* container 이미 초기화됨 — MF 표준 패턴 */ }
      }
    });
  };
  s.onerror = function(){
    console.error('[MF] gojiraApp remoteEntry.js 로드 실패:', s.src);
    resolve({
      get: function(){ return Promise.reject(new Error('gojiraApp remote unavailable')); },
      init: function(){}
    });
  };
  document.head.appendChild(s);
})`;

module.exports = {
  webpack: {
    plugins: {
      add: [
        new ModuleFederationPlugin({
          name: 'platformApp',
          remotes: {
            gojiraApp: dynamicRemote,
          },
          shared: {
            react: { singleton: true, requiredVersion: '^19.2.4' },
            'react-dom': { singleton: true, requiredVersion: '^19.2.4' },
          },
        }),
      ],
    },
    configure: (webpackConfig) => {
      webpackConfig.output.publicPath = 'auto';
      return webpackConfig;
    },
  },
};
