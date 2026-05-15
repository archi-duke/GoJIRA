require('dotenv').config();
const { ModuleFederationPlugin } = require('webpack').container;

module.exports = {
  webpack: {
    plugins: {
      add: [
        new ModuleFederationPlugin({
          name: 'platformApp',
          remotes: {
            gojiraApp: `gojiraApp@${process.env.REACT_APP_GOJIRA_APP_REMOTE || 'http://localhost:3001'}/remoteEntry.js`,
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
