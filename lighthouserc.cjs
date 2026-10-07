const baseURL = 'http://127.0.0.1:18081';

module.exports = {
  ci: {
    collect: {
      startServerCommand: 'FOLIORELAY_WEBUI_LISTEN=127.0.0.1:18081 FOLIORELAY_WEBUI_TEST_ROOT=/tmp/foliorelay-webui-lighthouse bash scripts/webui/start-browser-test-server.sh',
      startServerReadyPattern: 'foliorelayd listening',
      startServerReadyTimeout: 120000,
      numberOfRuns: 1,
      settings: {
        chromeFlags: '--no-sandbox'
      },
      url: [baseURL + '/']
    },
    assert: {
      assertions: {
        'categories:accessibility': ['error', { minScore: 1 }],
        'errors-in-console': ['error', { minScore: 1 }],
        'categories:best-practices': ['error', { minScore: 0.95 }],
        'categories:performance': ['error', { minScore: 0.9 }]
      }
    },
    upload: {
      target: 'filesystem',
      outputDir: './build/lighthouse'
    }
  }
};
