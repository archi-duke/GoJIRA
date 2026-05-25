require('dotenv').config();
const path = require('path');
const { ModuleFederationPlugin } = require('webpack').container;

// 양 앱(GoJIRA-App, Platform-App) 공유 UI 프리미티브. GoJIRA-App/craco.config.js
// 와 동일한 설정 — alias + ModuleScopePlugin 제거 + babel-loader include 확장.
const SHARED = path.resolve(__dirname, '../gojira-shared-web');

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
    alias: {
      'gojira-shared-web': SHARED,
    },
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
      webpackConfig.resolve.plugins = (webpackConfig.resolve.plugins || []).filter(
        (p) => p.constructor.name !== 'ModuleScopePlugin'
      );
      const oneOf = webpackConfig.module.rules.find((r) => Array.isArray(r.oneOf));
      if (oneOf) {
        const babelRule = oneOf.oneOf.find(
          (r) => r.loader && r.loader.includes('babel-loader') && r.include
        );
        if (babelRule) {
          const inc = Array.isArray(babelRule.include) ? babelRule.include : [babelRule.include];
          babelRule.include = [...inc, SHARED];
        }
      }
      return webpackConfig;
    },
  },
};
