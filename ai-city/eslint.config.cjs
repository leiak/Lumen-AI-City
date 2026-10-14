const ignores = [
  '**/node_modules/**',
  '**/dist/**',
  '**/.next/**',
  '**/build/**',
  '**/target/**',
  '**/.venv/**',
];

module.exports = [
  {
    ignores,
  },
  {
    files: ['**/*.{ts,tsx,js,jsx}'],
    plugins: {
      'react-hooks': require('eslint-plugin-react-hooks'),
    },
    languageOptions: {
      ecmaVersion: 2024,
      sourceType: 'module',
      parser: require('@typescript-eslint/parser'),
    },
    rules: {
      'no-unused-vars': 'warn',
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      'prefer-const': 'error',
      eqeqeq: ['error', 'always', { null: 'ignore' }],
    },
    rules: {
      'react-hooks/exhaustive-deps': 'warn',
    },
  },
];
