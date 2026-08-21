# AIVO admin

Restaurant backoffice SPA. Vite + React + TypeScript, react-router. Served by
the Go binary at `/admin` (Vite `base: "/admin/"`).

## Commands

```bash
npm install
npm run dev            # dev server; /api proxied to localhost:8080
VITE_MOCK=1 npm run dev  # force mock mode (no backend needed)
npm run build          # type-check + production build to dist/
```

## Mock mode

The typed client in `src/api/client.ts` implements the `/api/v1` contract from
`docs/PLATFORM.md`. With `VITE_MOCK=1` — or automatically when the API is
unreachable — it runs against `src/api/mock.ts`: a localStorage-persisted
in-browser backend seeded with the Ember & Bone demo tenant
(`src/api/fixtures.ts`).

Demo login: `owner@emberandbone.example` / `firegrill`. Clear the
`aivo-admin-mock` localStorage key to reseed.

## Design

Tokens come from `web/design-system/` (imported in `src/styles.css`, never
forked). Money is integer cents everywhere; format only via
`src/lib/money.ts`. Icons are Lucide only.
