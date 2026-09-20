# Scaffolding a new repository

`../templates/` holds a checked skeleton and `../scripts/scaffold.sh` installs it:

- **backend**: builds, passes `go vet`, `go test -race` and golangci-lint v2 (0 issues),
  over 90% statement coverage.
- **repo files**: the CI workflow passes actionlint, the Dockerfiles pass hadolint, and the
  compose file passes `docker compose config`.
- **frontend overlay**: type-checks under strict TypeScript 6; its API client is
  behaviour-tested.

The script copies files and personalises names; it never installs packages. Run all
commands from the repository root. These files define the conventions for all later work;
read them once and skim the rest: `Makefile`, `backend/internal/httpapi/{router,problem,decode,middleware}.go`,
`backend/test/e2e/api_test.go`, `frontend/src/api/{client,errors}.ts`,
`frontend/src/test/render.tsx`, `frontend/src/styles/tokens.css`.

## 1. Repository files

```bash
git init -b main                                   # only if not a repository yet
git config core.hooksPath .claude/githooks         # format on commit
bash .claude/skills/implement/scripts/scaffold.sh repo
```

Creates, never overwriting: `Makefile`, `.github/workflows/ci.yml`, `docker-compose.yml`,
`.gitignore`, `.editorconfig`, `.nvmrc` (installed Node major.minor), `scripts/coverage-report.sh`,
`scripts/dev.sh`,
`docs/adr/README.md`, `docs/adr/0001-architecture-and-dependency-policy.md`.

Then:
- CI: check the current major version of each action (`actions/checkout`,
  `actions/setup-go`, `actions/setup-node`, `golangci/golangci-lint-action`,
  `actions/upload-artifact`) on its GitHub releases page and update the workflow. Set the
  golangci-lint `version:` to the installed minor (`golangci-lint version`).
- Edit ADR 0001 wherever the user decided differently.
- Makefile: `SERVICE := api` matches `backend/cmd/api`; keep the names unless the user asks.

## 2. Backend

```bash
bash .claude/skills/implement/scripts/scaffold.sh backend <module-path>
```

- Module path: derive it from the git remote (`github.com/<owner>/<repo>/backend`) or ask.
- The script copies `templates/backend` to `backend/`, replaces `example.com/service`,
  sets the `go` directive and the Dockerfile's Go version to the installed toolchain, and
  runs gofmt, `go vet`, `go test` and golangci-lint (when installed). Fix anything it reports.
- Then set `info.title` and `info.description` in `backend/api/openapi.yaml` and the
  package comment in `cmd/api/main.go`.
- Features follow [backend-feature-pattern.md](backend-feature-pattern.md).

| File | Responsibility |
|---|---|
| `cmd/api/main.go` | flags (`-healthcheck`), config, logger, signals; `run()` is testable |
| `internal/config` | environment → validated `Config` (all errors reported together) |
| `internal/server` | `http.Server` timeouts, graceful shutdown |
| `internal/app` | composition root (`app.New(cfg, logger) http.Handler`) |
| `internal/httpapi/router.go` | `Deps`, routes, middleware chain, 404/405 as problem details, health |
| `internal/httpapi/decode.go` | strict JSON decoding (415, 413, unknown fields, trailing data, type errors) |
| `internal/httpapi/problem.go` | RFC 9457 problem details, error codes, `writeJSON` |
| `internal/httpapi/middleware.go` | request ID, access log, panic recovery, security headers, CORS, request timeout |
| `test/e2e` | black-box tests over real HTTP against `app.New` |
| `.golangci.yml` | linters, formatters, `depguard` allowing only the standard library and this module |
| `Dockerfile` | static binary on distroless, non-root, `HEALTHCHECK` via `-healthcheck` |

Environment variables (document them in the README):

| Variable | Default | Meaning |
|---|---|---|
| `HTTP_ADDR` | `:8080` | listen address |
| `HTTP_READ_HEADER_TIMEOUT` / `HTTP_READ_TIMEOUT` / `HTTP_WRITE_TIMEOUT` / `HTTP_IDLE_TIMEOUT` | `5s` / `10s` / `15s` / `60s` | server timeouts |
| `HTTP_SHUTDOWN_TIMEOUT` | `15s` | grace period for in-flight requests |
| `HTTP_REQUEST_TIMEOUT` | `10s` | deadline on each request context (below the write timeout) |
| `HTTP_MAX_BODY_BYTES` | `1048576` | request body limit |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | `debug`…`error` / `json` or `text` |
| `CORS_ALLOWED_ORIGINS` | empty | comma-separated allow-list (empty: same-origin only) |

## 3. Frontend

1. Generate the official template. If the CLI prompts, check
   `npm create vite@latest -- --help` for the current non-interactive flags. Never leave a
   dev server running.
   ```bash
   npm create vite@latest frontend -- --template react-ts
   ```
2. Apply the overlay (product name and description come from the spec):
   ```bash
   bash .claude/skills/implement/scripts/scaffold.sh frontend "<Product name>" "<One-line description>"
   ```
   It adds `src/api` (client, errors, guards, provider, `useApi`, tests including the MSW
   canary), `src/app` (shell, error boundary), `src/config.ts`, `src/lib/env.ts`,
   `src/styles` (tokens, global), `src/test` (setup, MSW server, `renderWithProviders`),
   `e2e/smoke.spec.ts`, `vite.config.ts` (proxy `/api` → `API_PROXY_TARGET`,
   default `http://localhost:8080`), `vitest.config.ts` (jsdom, 80% thresholds,
   `../coverage/frontend`), `playwright.config.ts` (starts API on 18080 and web on 14173;
   desktop and Pixel 7 projects), Prettier config, `.env.example`, `nginx.conf`, Dockerfile,
   a neutral `public/favicon.svg` (replace it when the product has its own icon).
   It removes the Vite starter files and sets the npm scripts `typecheck`, `lint`, `format`,
   `format:check`, `test`, `test:watch`, `test:coverage`, `test:e2e`.
3. Install the template's dependencies and the development tools in one command (the
   user approves it):
   ```bash
   cd frontend
   npm install -D vitest @vitest/coverage-v8 jsdom \
     @testing-library/react @testing-library/dom @testing-library/user-event @testing-library/jest-dom \
     msw @playwright/test prettier eslint-config-prettier eslint-plugin-jsx-a11y @types/node
   npx playwright install chromium
   ```
   `@vitest/coverage-v8` must have the same version as `vitest` (`npm ls vitest @vitest/coverage-v8`).
4. Edit the generated TypeScript configs (keep their existing options and comments):
   - `tsconfig.app.json`: add `"noUncheckedIndexedAccess": true`, `"noImplicitOverride": true`,
     `"exactOptionalPropertyTypes": true`; make sure `"types": ["vite/client"]` and
     `"include": ["src"]`.
   - `tsconfig.node.json`: `"types": ["node"]` and
     `"include": ["vite.config.ts", "vitest.config.ts", "playwright.config.ts", "e2e"]`.
   - If `exactOptionalPropertyTypes` clashes with a library's types in a way you cannot fix
     locally, drop that one option and record the assumption in the plan.
5. Replace `eslint.config.js` with the configuration below. If the generated file uses a
   different export name for a plugin (it matches the installed versions), keep the
   generated name.

   ```js
   import js from '@eslint/js'
   import prettier from 'eslint-config-prettier/flat'
   import jsxA11y from 'eslint-plugin-jsx-a11y'
   import reactHooks from 'eslint-plugin-react-hooks'
   import reactRefresh from 'eslint-plugin-react-refresh'
   import { defineConfig, globalIgnores } from 'eslint/config'
   import globals from 'globals'
   import tseslint from 'typescript-eslint'

   export default defineConfig([
     globalIgnores(['dist', 'coverage', 'playwright-report', 'test-results']),
     {
       files: ['**/*.{ts,tsx}'],
       extends: [
         js.configs.recommended,
         tseslint.configs.recommendedTypeChecked,
         tseslint.configs.stylisticTypeChecked,
         reactHooks.configs.flat.recommended,
         reactRefresh.configs.vite,
         jsxA11y.flatConfigs.recommended,
       ],
       languageOptions: {
         ecmaVersion: 2022,
         globals: globals.browser,
         parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
       },
       rules: {
         '@typescript-eslint/consistent-type-imports': 'error',
         '@typescript-eslint/no-misused-promises': ['error', { checksVoidReturn: { attributes: false } }],
       },
     },
     {
       files: ['src/test/**', '**/*.test.{ts,tsx}'],
       rules: { 'react-refresh/only-export-components': 'off' },
     },
     {
       files: ['*.config.ts', 'e2e/**/*.ts'],
       languageOptions: { globals: globals.node },
     },
     prettier,
   ])
   ```
6. Verify, fixing until everything passes:
   ```bash
   npm run format && npm run lint && npm run typecheck && npm test && npm run test:coverage && npm run build
   ```
   - `src/api/client.msw.test.ts` checks that jsdom, MSW and `AbortSignal` work together.
     If it fails with an error like "Expected signal to be an instance of AbortSignal", ask
     the user to approve `happy-dom` as a dev dependency and set `environment: 'happy-dom'`
     in `vitest.config.ts`.
   - If the coverage threshold fails on the bare skeleton, add tests; never lower the threshold.
7. Once the backend exists: `make test-e2e` runs `e2e/smoke.spec.ts` against both apps.
8. Features follow [frontend-feature-pattern.md](frontend-feature-pattern.md).

## 4. Finish the phase

```bash
make fmt lint typecheck test build
```

Start `make dev` as a background task, check that
`curl -s http://localhost:5173/api/v1/nope` returns the 404 problem document through the
Vite proxy and that `http://localhost:5173/` serves the shell, then stop the task.
Commit: `chore: scaffold backend and frontend`.

## Container topology

Default: two images behind `docker-compose.yml`. The web image (nginx) serves the build
and proxies `/api/` to the `api` service; the API image runs the Go binary. Compose waits
for the API health check.

If the user wants **one image for both tiers**, add a root `Dockerfile` that:

1. builds the frontend in a `node` stage;
2. builds the Go binary in a `golang` stage;
3. copies both into the distroless image.

Then give `httpapi` an optional static-file handler, enabled by `STATIC_DIR`, that serves
the build with an SPA fallback to `index.html` and immutable caching for `/assets/`. It must
never shadow `/api/` or the health endpoints. Test it, document it, and record the choice
in an ADR.
