---
paths:
  - "frontend/**"
---

# React + TypeScript frontend rules

Reference code: the overlay in `.claude/skills/implement/templates/frontend/`, the setup steps in
`.claude/skills/implement/references/scaffold.md` and the feature slice in
`.claude/skills/implement/references/frontend-feature-pattern.md`.

## Stack and dependencies

- Vite (official `react-ts` template) + React + TypeScript. Keep the versions the scaffolder
  and `npm install` resolve; don't pin to what you remember.
- Runtime dependencies: `react`, `react-dom`. Nothing else without a written justification
  and the user's approval (no UI kits, CSS frameworks, state or data-fetching libraries,
  axios, lodash, moment).
- Dev dependencies are fine when they serve quality: Vitest, Testing Library, MSW,
  Playwright, ESLint plugins, Prettier.
- npm only (`package-lock.json` committed). Node version pinned in the repo-level `.nvmrc`.

## Structure

```
src/
  main.tsx              bootstrap only (root render, global CSS)
  app/                  App shell, providers, error boundary, layout
  api/                  http client, ApiError, DTO types, parsers, endpoint functions, ApiProvider
  features/<feature>/   components, hooks, pure logic (model.ts), styles, tests
  components/           shared presentational components (Button, TextField, Alert, Spinner)
  lib/                  pure utilities (formatting, parsing) with unit tests
  styles/               tokens.css (custom properties), global.css (reset, base typography)
  test/                 setup.ts, msw handlers/server, render helpers
```

- One component per file, named exports, file name = component name (`ResultPanel.tsx`),
  styles beside it (`ResultPanel.module.css`), tests beside it (`ResultPanel.test.tsx`).
- Components render; hooks orchestrate; pure functions decide. Business/UI rules that can
  be expressed without React live in `model.ts` (pure functions or a reducer) and are unit
  tested without rendering.

## TypeScript

- Keep the template's strict settings and add `noUncheckedIndexedAccess`,
  `noImplicitOverride` and `exactOptionalPropertyTypes` when they don't fight the libraries.
- The template enables `erasableSyntaxOnly` and `verbatimModuleSyntax`: no enums, no
  namespaces, no constructor parameter properties; use union types and `import type`.
- No `any`, no `@ts-ignore`/`@ts-expect-error` without a linked reason, no non-null `!`
  unless an invariant is proven next to it. API data is `unknown` until a parser narrows it.
- TypeScript 6+ defaults `types` to `[]`: import test APIs (`import { describe, it, expect } from 'vitest'`)
  instead of relying on globals, and list ambient types explicitly where needed.

## State and data

- Local state first; `useReducer` for multi-step or interdependent state, with the reducer
  exported and unit tested. Derive values instead of storing them. Context only for
  dependency injection (API client) and truly app-wide state.
- All HTTP goes through `src/api` (`createHttpClient` + endpoint functions + parsers).
  Components get the API through `useApi()` (context), so tests can inject fakes — never call
  `fetch` from components.
- Every request is cancellable: abort on unmount and when a newer request supersedes it;
  ignore stale results; guard against double submission; show loading, success, empty and
  error states explicitly.
- Map errors by `ApiError.kind` / `ApiError.code` to user-facing copy; show server field
  errors next to their fields; never show stack traces or raw JSON.

## Forms and input

- Controlled inputs with visible `<label>`s. Validate on change/blur for guidance and again
  on submit; the server remains the authority and its field errors are displayed.
- Choose `type`/`inputMode`/`autoComplete` deliberately. Parse numbers explicitly — beware
  `Number('')` is `0`, `Number(' 1 ')` is `1`, `parseFloat('1abc')` is `1`, and locale
  decimal separators. Trim where the spec allows it.
- Disable submit while a request is pending; keep the user's input on errors.

## Accessibility (WCAG 2.2 AA) and responsive design

- Semantic HTML first (`button`, `form`, `output`, headings in order, landmarks).
  Every interactive element is reachable and operable by keyboard with a visible
  `:focus-visible` style; no keyboard traps.
- Announce async results with `aria-live="polite"` (or `<output>`), errors with
  `role="alert"`; link errors to inputs with `aria-describedby`; set `aria-invalid`.
- Colour contrast ≥ 4.5:1 for text; never convey meaning by colour alone; honour
  `prefers-reduced-motion` and `prefers-color-scheme`.
- Mobile-first CSS; layouts work from 320 px to wide desktop without horizontal scrolling;
  touch targets ≥ 44×44 px; fluid type/spacing via tokens; test at 375 px and 1280 px.

## Styling

- CSS Modules for components; design tokens as CSS custom properties in `styles/tokens.css`
  (colour, spacing scale, radius, typography, shadows, focus ring, breakpoints documented).
- No inline styles except for truly dynamic values; no `!important`; class names in camelCase.
- Form controls use `--color-control-border` (at least 3:1 against the background, WCAG
  1.4.11); `--color-border` is for decorative lines only. Focus stays visible through the
  global `:focus-visible` outline; never remove it without an equally visible replacement.

## Robustness, security, performance

- An error boundary wraps the app and renders a recoverable fallback.
- Configuration only via `import.meta.env.VITE_*`, read once in `src/config.ts` with
  defaults and documented in `.env.example`; never put secrets in frontend code. Real
  `.env*` files belong to the user: never create or read them. No `dangerouslySetInnerHTML`.
- Memoise (`useMemo`/`useCallback`/`memo`) only to fix a measured problem or to keep a
  dependency stable; keep the bundle lean.

## Tooling

- ESLint flat config extending the template with type-aware `typescript-eslint`
  (`recommendedTypeChecked`, `stylisticTypeChecked`), `react-hooks`, `react-refresh`,
  `jsx-a11y`, and `eslint-config-prettier` last. Zero warnings (`--max-warnings=0`).
- Prettier owns formatting (formatted automatically on commit). Tests:
  `.claude/rules/testing.md`.
