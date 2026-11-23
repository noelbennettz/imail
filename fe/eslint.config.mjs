import pluginVue from 'eslint-plugin-vue'

// ESLint 10 only reads flat config; the old .eslintrc.cjs was silently ignored,
// so `yarn lint` stopped linting anything. This keeps the same intent: Vue 3
// essentials plus unused-variable detection.
export default [
  {ignores: ['dist/**', 'node_modules/**', 'public/**']},
  ...pluginVue.configs['flat/essential'],
  {
    files: ['**/*.{js,mjs,cjs,vue}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        window: 'readonly',
        document: 'readonly',
        navigator: 'readonly',
        localStorage: 'readonly',
        console: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        CustomEvent: 'readonly',
        ResizeObserver: 'readonly',
        FileReader: 'readonly',
        MouseEvent: 'readonly',
        URL: 'readonly',
        location: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        process: 'readonly',
      },
    },
    rules: {
      'no-unused-vars': ['error', {argsIgnorePattern: '^_'}],
      'no-undef': 'error',
    },
  },
]
