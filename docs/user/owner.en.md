# For owners and managers

Everything is managed from the admin panel: `/admin`.

## Registration

1. Open `/admin` → "Register".
2. Enter the organization name, restaurant name, email, and password.
3. Done: a restaurant is created with a slug address (`/{slug}`), a default
   menu, and the **Free** plan (1 restaurant, up to 30 items).

## Menu

**Menu → Items tab.** A restaurant can have several menus (main, bar,
lunch) — each is reachable at `/{slug}/m/{menu}`.

- Category: "+ Category", name, drag to reorder.
- Item: name, description, price, photo (uploaded right in the form),
  allergens (EU-14), option groups (single or multiple choice, price
  delta per option), "available" flag.
- Exactly one menu is the default: it opens from the table QR.

## Appearance

**Menu → Design and design.md tabs.**

- Design: accent color, fonts, banner, CSS variables — applied live.
- design.md: paste a design brief (e.g. from Claude) → "Generate" — the AI
  proposes a theme from the brief. The proposal is **never auto-applied**:
  review the preview and hit "Apply" or "Discard".

## Tables and QR

**Tables**: add tables → each gets its own QR and link `/{slug}/t/{token}`.
Print the QR for the table. "Regenerate" instantly revokes the old link.

## Subscription

**Organization → Subscription**: Free → Pro (unlimited items, custom
domain, themes) → Business (multiple restaurants). Plan changes apply
immediately.

## Guests (CRM)

**Guests**: registered visitors with visit counts and total spend
(including orders placed via waiter codes). Notes and tags ("regular",
"VIP") are visible only to your restaurant.

## AI assistant

**Assistant**: a chat you can tell things like "raise coffee prices by
10%" or drop files into (a PDF menu, photos). The assistant proposes
actions — you confirm each one explicitly; it never changes anything on
its own.
