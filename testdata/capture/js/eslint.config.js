module.exports = [
  {
    files: ['**/*.js'],
    languageOptions: { sourceType: 'commonjs', globals: { require: 'readonly', module: 'writable', console: 'readonly' } },
    rules: { 'no-unused-vars': 'warn', 'no-console': 'error' },
  },
]
