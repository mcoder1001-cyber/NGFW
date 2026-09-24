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
          // Review P07a L2: every attribute is checked except the non-text ones below (an allowlist missed `tooltip`,
          // `description`, `message`, …), and all-caps words (`MTU`, `OK`) are no longer exempt — they go through t().
          'jsx-attributes': {
            include: [],
            exclude: [
              'className', 'style', 'sx', 'type', 'key', 'id', 'name', 'htmlFor', 'form', 'labelId', 'width', 'height',
              'to', 'href', 'target', 'rel', 'src', 'component', 'variant', 'color', 'size', 'edge', 'severity',
              'orientation', 'direction', 'position', 'anchor', 'justifyContent', 'alignItems', 'flexWrap', 'gap',
              'autoComplete', 'inputMode', 'role', 'lang', 'dir', 'mode', 'fontFamily', 'underline', 'textColor',
              'indicatorColor', 'labelKey', 'anchorOrigin', 'transformOrigin', 'queryKey', 'paginationMode', 'sortingMode',
              'filterMode', 'density', 'maxWidth', 'fontSize', 'valueLabelDisplay', 'data-.*', 'aria-(controls|labelledby|describedby|haspopup|live|current|hidden|expanded|owns)',
            ],
          },
          words: { exclude: ['[0-9!-/:-@[-`{-~]+', '—', '…'] },
        },
      ],
    },
  },
];
