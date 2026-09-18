import js from "@eslint/js";
import eslintConfigPrettier from "eslint-config-prettier";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import globals from "globals";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist", "coverage"] },
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
      ...tseslint.configs.stylisticTypeChecked,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
      jsxA11y.flatConfigs.recommended,
    ],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message:
            "Render server HTML only through RichTextView (src/features/tasks/components/rich-text-view.tsx), which sanitizes it first.",
        },
      ],
    },
  },
  {
    // The one place that may inject HTML: it sanitizes with DOMPurify first.
    files: ["src/features/tasks/components/rich-text-view.tsx"],
    rules: {
      "no-restricted-syntax": "off",
    },
  },
  {
    // Generated shadcn primitives keep upstream shape for easy re-generation.
    files: ["src/components/ui/**/*.tsx"],
    rules: {
      "react-refresh/only-export-components": "off",
      "@typescript-eslint/array-type": "off",
    },
  },
  {
    // The public, unauthenticated parent-statement route must never pull in
    // the authenticated app's plumbing: a 401/404 there is a normal outcome
    // (a wrong token), not a session problem, and must render the neutral
    // error page rather than trigger a refresh attempt or a redirect.
    files: ["src/features/statement/**/*.{ts,tsx}", "src/layouts/public-layout.tsx"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["@/features/auth", "@/features/auth/*"],
              message:
                "The public statement route must not depend on the authenticated app's auth feature.",
            },
            {
              group: ["@/lib/api/client"],
              message: "Use @/lib/api/public-client instead — this route carries no session.",
            },
          ],
        },
      ],
    },
  },
  {
    // src/lib/kanban is a headless core meant to be extracted into its own
    // package later (see apps/web/src/lib/kanban/README.md); its only
    // allowed dependency is `react`.
    files: ["src/lib/kanban/**/*.{ts,tsx}"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["@/features/*", "@/features"],
              message:
                "src/lib/kanban must stay UI/transport-agnostic — it cannot depend on app features.",
            },
            {
              group: ["@/components/*", "@/components"],
              message: "src/lib/kanban must not depend on the design system — it is headless.",
            },
            {
              group: ["@/lib/api", "@/lib/api/*"],
              message:
                "src/lib/kanban knows nothing about HTTP — the app's KanbanDataSource adapter owns that.",
            },
            {
              group: ["@tanstack/*"],
              message:
                "src/lib/kanban must not depend on TanStack — the app owns caching/optimistic updates.",
            },
            {
              group: ["zod"],
              message:
                "src/lib/kanban does not validate external input — that belongs to the adapter layer.",
            },
            {
              group: ["axios"],
              message: "src/lib/kanban must not perform HTTP calls directly.",
            },
            {
              group: ["@dnd-kit/*"],
              message:
                "src/lib/kanban is drag-layer-agnostic — the app's use-board-dnd adapter owns dnd-kit.",
            },
          ],
        },
      ],
    },
  },
  eslintConfigPrettier,
);
