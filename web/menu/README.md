# AIVO diner menu

Diner-facing SPA (Vite + React + TypeScript). Two entry modes:

- `/{restaurant_slug}/t/{table_token}` — full ordering experience. Loads
  restaurant, table, theme, and `menus` (multi-menu, default first) from
  `GET /api/v1/t/{table_token}`; drives the Landing → Menu → Item → Cart →
  Sent → Service flow from `docs/prototypes/aivo-menu-prototype.dc.html`.
  A menu switcher pill row appears above the categories when the restaurant
  has more than one menu.
- `/{restaurant_slug}/m/{menu_slug}` — public browse mode (shareable link,
  no table). Loads one menu from `GET /api/v1/m/{restaurant_slug}/{menu_slug}`;
  menu is read-only: no cart, no ordering, no service buttons. Unknown slugs
  render a not-found screen.

## Commands

```bash
npm install
npm run dev            # dev server, proxies /api to localhost:8080
VITE_MOCK=1 npm run dev  # mock mode: Ember & Bone fixtures, no backend
npm run build          # typecheck + production build (dist/)
npm test               # vitest unit tests
```

In mock mode any URL works (a bare `/` uses a demo token). Without
`VITE_MOCK=1` the app talks to the real API and falls back to the mock
fixtures if the API is unreachable.

## Accounts & cart handoff

- Optional customer account (anonymous flow unchanged): "Sign in" in the
  Landing/Menu header opens a login/register sheet; signed in, the first name
  links to an account screen with order history and sign-out. Backed by
  `/api/v1/customer/*` (cookie session). Mock credentials:
  `guest@ember.test` / `embertest1`.
- Cart handoff: "Show to waiter" on the cart posts the lines to
  `POST /api/v1/t/{token}/handoff` and shows a full-screen pickup code
  (QR + 6-char code, 15 min countdown). The cart clears — the code holds the
  lines — and a sessionStorage backup restores the cart with a notice if the
  code expires unused. A refresh mid-handoff returns to the code screen.
  In mock mode the QR is a deterministic decorative SVG (not scannable);
  the real backend serves a PNG at `qr_url`.

## Notes

- Cart is client-side only (sessionStorage per table token); it never touches
  the server until "Send to the kitchen".
- The 90-second resend cooldown is enforced client-side and re-synced from the
  server's 429 `retry_after_seconds` / `Retry-After` when they disagree.
- Theme JSON (accent, bold, brand name, banner, `css_vars`) is applied as CSS
  custom properties over the design-system tokens (`src/theme.ts`), same
  mapping as the prototype's `themeVars`.
- Design tokens come from `web/design-system/styles.css` — imported directly,
  not forked.
