# Tecnocondor fork

`github.com/tecnocondor/calnode` runs the booking engine behind `cal.tecnocondor.dev`.
Each release is the upstream tag plus the patches below, cherry-picked onto a branch
`tecnocondor-X.Y.Z` and tagged `vX.Y.Z-tc.N` (the tag triggers
`.github/workflows/tecnocondor-image.yml`, which publishes `ghcr.io/tecnocondor/calnode`).
List the patches of a release with `git log vX.Y.Z..tecnocondor-X.Y.Z`.

## Patches on top of upstream

| Patch | Files | Notes |
|---|---|---|
| Image in the fork's GHCR | `.github/workflows/tecnocondor-image.yml` | Runs on `v*-tc.*` tags only. |
| No NS8 module workflow | `.github/workflows/ns8-module.yml` (deleted) | Upstream's workflow listens on `v*.*.*` and would fire on our tags. Delete it again on every update. |
| Mail `Date` / `Message-ID` | `internal/mailer/smtp.go` | |
| Skip reminders already due | `internal/handler/booking_handler.go` | |
| Mail identity | `internal/mailer/html.go` | Ink, 6 px radii, button with the orange block. |
| Own hours per event type | `internal/handler/{availability,slots_handler}.go`, admin UI | |
| **Public pages identity (overlay)** | see below | Tecnocondor look on `/book`, `/manage`, `/u`, `/team`. |

## Public pages overlay

One stylesheet over upstream's, so updates of `booking.css` and the templates rarely
conflict:

- `internal/handler/tecnocondor/tecnocondor.css` — tokens copied from
  `tecnocondor/web` `src/styles/global.css` (light), fonts, and selector overrides.
  Header comment explains the colour rules.
- `internal/handler/tecnocondor/fonts/` — Funnel Display, Geist and Geist Mono variable
  woff2 (latin + latin-ext) from `@fontsource-variable` 5.3.0, with their SIL OFL 1.1
  licences (`OFL-*.txt`). Self-hosted; CSP `default-src 'self'` already allows them.
- `internal/handler/tecnocondor_assets.go` (+ `_test.go`) — embeds the folder and serves
  `GET /tc/{file...}` (ETag, short cache for the CSS, 30 days for fonts).
- Upstream lines touched (the only likely conflicts on an update):
  - `internal/server/server.go`: one route next to `GET /booking.css`.
  - `internal/handler/templates/{book,manage,directory}.html`: one
    `<link rel="stylesheet" href="/tc/tecnocondor.css">` right after the inline
    `</style>`. It must stay after that block: the overlay wins by source order.

The Branding accent (Settings → Branding) still colours the selected day/slot and the
primary button (`--booking-accent`). The embed widget (`embed.js`) is not themed. The
pages have no dark mode upstream (and the uploaded logo is a PNG), so the overlay is
light only.

If an update renames classes in `booking.css`, the overlay simply stops applying to
them; check `/book/<slug>` and `/manage/<token>` on desktop and at 390 px after each
update.
