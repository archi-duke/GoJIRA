const { ModuleFederationPlugin } = require('webpack').container;

module.exports = {
  webpack: {
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
      return webpackConfig;
    },
  },
};
