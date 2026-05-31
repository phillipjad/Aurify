import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import prettierRecommended from 'eslint-plugin-prettier/recommended'
import tseslint from 'typescript-eslint'

// Flat ESLint config (ESLint 9) for the React + TypeScript app.
//
// Prettier runs *inside* ESLint via eslint-plugin-prettier: formatting issues
// surface as the `prettier/prettier` lint rule (and are fixed by `eslint --fix`),
// and eslint-config-prettier turns off any stylistic rules that would conflict.
// So `pnpm lint` is the single check — there is no separate `prettier` step.
// The recommended config is listed LAST so its rule-disabling wins.
export default tseslint.config(
  { ignores: ['dist', 'src/routeTree.gen.ts', 'src/lib/api/schema.ts'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2023,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
    },
  },
  prettierRecommended,
)
