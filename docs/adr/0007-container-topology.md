# 0007. Ship two containers behind a static web server

- Status: Accepted
- Date: 2026-09-18

## Context

The deliverable must start with one command and run the way it would in production: the
browser app served as static files, the API as a separate process, no development servers.
Development, however, must keep hot reloading and a same-origin `/api` path.

Options considered: one image in which the Go binary also serves the static build
(rejected: it couples the release cycles of the two tiers and puts an HTML server in the
API); a development image running Vite (rejected: not a production setup).

## Decision

- `compose.yaml` defines two services: `backend` (the Go binary on a distroless static
  image, non-root, `HEALTHCHECK` through the binary's own `-healthcheck` flag, published on
  port 8080 so the README's `curl` examples work) and `web` (the Vite production build
  served by unprivileged nginx on port 3000). `web` depends on `backend` being healthy.
- nginx serves the SPA with a fallback to `index.html`, long-lived caching for the
  fingerprinted `/assets/`, `no-cache` for the shell, gzip, `server_tokens off`, and the
  security headers of spec section 6 at server level (adding
  `Content-Security-Policy: default-src 'self'` and tightening directives for the HTML
  shell). It proxies `/api/` to `backend:8080`, forwards or generates `X-Request-ID`, and
  hides the API's own copies of the security headers so each appears once.
- The API's body limit is 4 KiB; nginx allows 8 KiB so that oversized requests reach the
  API and receive its problem document instead of an nginx error page.
- In development, `make dev` runs both dev servers and Vite proxies `/api` to the Go
  service on port 8080; the browser code always calls the same origin (`VITE_API_BASE_URL`
  empty) with `/api/v1` appended.
- Base image tags follow `go.mod` and `.nvmrc` through build arguments that the scaffold
  keeps in sync; CI builds the images, waits for health and smoke-tests port 3000.

## Consequences

- Same-origin calls everywhere, so CORS stays off by default.
- The two images can be built, scanned and rolled out independently; the API image has no
  shell or package manager.
- Running the stack needs Docker Desktop (or an equivalent daemon); the README's
  troubleshooting section covers a stopped daemon and ports already in use.
