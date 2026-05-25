const path = require('path');
const { ModuleFederationPlugin } = require('webpack').container;

// 양 앱(GoJIRA-App, Platform-App) 공유 UI 프리미티브. webpack alias 로 bare
// specifier 'gojira-shared-web' 를 실제 dir 로 매핑하고, ModuleScopePlugin 제거
// + babel-loader include 확장으로 src/ 밖 JSX 도 transpile 되게 만든다.
const SHARED = path.resolve(__dirname, '../gojira-shared-web');

module.exports = {
  webpack: {
    alias: {
      'gojira-shared-web': SHARED,
    },
    plugins: {
      add: [
        new ModuleFederationPlugin({
          name: 'gojiraApp',
          filename: 'remoteEntry.js',
          exposes: {
            './GoJIRAApp': './src/GoJIRAApp',
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
