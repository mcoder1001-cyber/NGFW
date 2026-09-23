// Extends the repository config with the i18n rule: every user-visible string in JSX must go through t().
import i18next from 'eslint-plugin-i18next';
import root from '../../eslint.config.js';

export default [
  ...root,
  {
    files: ['src/**/*.tsx'],
    ignores: ['src/**/*.test.tsx', 'src/test-utils.tsx'],
    plugins: { i18next },
    rules: {
      'i18next/no-literal-string': [
        'error',
        {
          mode: 'jsx-only',
          'jsx-attributes': {
            include: ['label', 'title', 'placeholder', 'helperText', 'alt', 'aria-label', 'aria-description', 'legend', 'primary', 'secondary', 'headerName'],
            exclude: [],
          },
          words: { exclude: ['[0-9!-/:-@[-`{-~]+', '[A-Z_-]+', '—', '…', 'VRX'] },
        },
      ],
    },
  },
];
