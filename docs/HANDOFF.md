# Handoff (2026-10-02)

The previous handoff's remaining features are implemented. Changes from this
continuation are **uncommitted and unpushed** on `master`.

## Follow-up requests completed

- Situation briefs can each have one optional vignette (opening briefing and
  each published turn). Separate `image-api`, `image-model`, `image-size`,
  `image-response-format`, and `AW_IMAGE_KEY` / `image-key-file` settings.
  No model is chosen or enabled in the live config. OpenAI-format base64 PNG/JPEG
  responses only. The worker is independent of text AI and uses no text key.
- Vignettes use reviewed public scene text, private preview, approve/hide/discard,
  replacement, cancellation, and persisted state. No automatic retries after
  failure or interruption. Image bytes are kept beside campaign data; serving
  checks approval and membership each time. Report snapshots include vignettes.
- Objectives are separate scenario/room data, editable in Setup and displayed
  beneath briefing text. Older saved rooms inherit scenario objectives once.
- Global squad orders cover all members without individual exceptions; individual
  orders override only that member. Withdrawal rejoins the current shared order.
  The issuer must command and be able to contact the whole squad. Per-member
  records preserve permissions/readiness, with grouping for the desk/report and
  AI prompts including participating characters. Plan drags preserve a global
  order when all participating members move to the same proposed destination.
- `/units/ID` now opens one character. Map squad overviews explicitly use
  `?squad=1`. Referee Table now includes the order dialog, fixing previously
  nonfunctional order links there.
- During adjudication the normal drag endpoint stages referee moves. Publishing
  applies them atomically and clears temporary plans; corrections restore both
  prior positions and staged plans. The UI no longer requires “Apply my plans”.
  Unapproved player intentions are not turned into authoritative moves.

### Follow-up validation

- Full Go suite, vet, JavaScript syntax, and race checks for imageai/imagework/web
  pass. New coverage includes
  image API validation, independent credentials, draft privacy, cancellation,
  restart handling, shared-order counts/permissions/overrides, individual cards,
  editable objectives, and publish/correction finalization of staged moves.
- BrowserSkill exercised a new disposable room `2WTA7V`: opening-vignette preview
  and approval using a local mock Images API; one shared squad order plus Doyle's
  individual exception; Doyle's single card; two referee outcomes; staging a
  formation move; publishing to turn 2. The token moved from 0509 to 0608 and all
  ghosts/plans cleared. The new situation's prompt contains public outcomes.
- Phone width 390: page width 390, no overflow. Screenshots:
  `/tmp/aw-vignette-preview.png`, `/tmp/aw-individual-card.png`, and
  `/tmp/aw-vignettes-mobile.png`. The mock preview is a gradient fixture, not
  model-generated artwork. No paid image API was called.
- Follow-up test data: `/tmp/async-wargame-vignette-ui-state.json`.

## Completed in this continuation

- Movement guidance: regression tests for reach text, order pin distance, and
  loading rooms recorded against module 3.0.0 with the current 3.1.0 module.
  The Desk's guidance was also checked in the browser.
- Formations: new St. Ines rooms start Lantern Squad as one token. Attached
  members move together; the referee can split/rejoin them through the map
  inspector. Split members have round chits, dashed tethers, and empty pips.
  Squad overviews retain each character's private-brief filtering. Player character
  panels list squad companions. Stale radio contacts remain separate, and
  adjudication uses frozen units for players. Operation restart redeploys the
  scenario's formations. Existing rooms are not silently regrouped.
- Private plans: saved per seat, unit, and turn. Plan mode allows players to
  drag only their own characters or formation, updating their own order pins;
  other members' orders are unaffected. Referee plans now become real moves on publish (see follow-up above). Plans survive reload, expire visually next turn, and
  participate in correction snapshots. Player plans are never sent to another
  seat (normal order-sharing rules still apply to an order's pin).
- Order dialog with inline no-JavaScript fallback. Pin placement closes and
  reopens it; closing/reopening preserves unsent text. New turns clear the
  previous turn's text and pin.
- New-turn announcement keyed by room, seat, and operation, with reduced-motion
  styling. Live refresh copies turn/phase/operation metadata.
- Briefing now establishes the year 2486 and Port Ines as a colony settlement.
  Headings read “Your order” and “Orders this turn”.
- Map toolbar wraps on phones. Counter inspection supports keyboard activation.
- Move batches validate all units and finite coordinates before mutating state;
  conflicting destinations within an attached formation are rejected.
- Removed an unused inline map style that violated the existing CSP.
- Follow-up styling request completed: era-neutral field-manual appearance with
  paper/olive light mode, charcoal/brass dark mode, serif campaign headings,
  square panels and tabs, flat status markers, and readable form borders.
  Squad dialogs use one scrolling surface and a clear heading/close area.
  The visual treatment is shared across settings; no period-specific imagery
  or changes to the current scenario's terminology were introduced.

## Verification

- `mise exec -- go test ./...` passes, including new formation, movement,
  persistence, plan privacy, authorization, and HTTP/squad-card tests.
- `mise exec -- go vet ./...` and `node --check internal/web/static/app.js` pass.
- Browser-skill tested a disposable room on localhost:8612: formation split and
  rejoin, player private planning, referee plan application, four-member cards
  showing only the player's private brief, draft/save/commit, map pin placement,
  Desk movement guidance, and publishing into turn 2 via live refresh. Verified
  “Turn 2 · orders are open”, cleared old draft, and no prior-turn ghosts.
- Styling checked at 390×844 and 1400×900 in both palettes. Core text contrast
  pairs measure 5.48:1 or higher; the styled order dialog was exercised.
  Previews: `/tmp/aw-military-dark-desktop.png`,
  `/tmp/aw-military-light-desktop.png`, `/tmp/aw-military-dark-mobile.png`,
  `/tmp/aw-military-light-mobile.png`, `/tmp/aw-military-order.png`.
- Phone viewport 390×844: no page overflow, all toolbar controls fit, squad card
  width 358px. Screenshots were saved under `/tmp/aw-player-mobile*.png`.
- Browser-skill has no drag command; drag checks used PointerEvents through its
  evaluate command. Clicks, forms, pin placement, and navigation used native
  browser-skill commands.

## Browser-skill setup

Following Tencent's AGENT_INSTALL.md, reused CLI 0.3.2 and the connected browser
extension. Installed the bundled skill for Codex at
`~/.agents/skills/browser-skill/SKILL.md`, loaded it, passed all `bsk doctor`
checks, and navigated/read example.com in a session that was then stopped.
An extension popup initially blocked Chrome automation on a name field; the
user dismissed it after a browser help request, and testing resumed.
Use `bsk` for future browser testing. The sandbox requires escalation to access
its existing daemon and skill directories; do not start a replacement daemon.

## Local state and running

- Never use `data/campaigns.json` for tests. This continuation used only
  `/tmp/async-wargame-ui-state.json`; live campaign data was not modified.
- Pre-existing user changes remain untouched: deleted `.env`, untracked
  `UPDATED_AI_INFO.md`, and the stray `asyncwargame` binary.
- `game.conf` is git-ignored. To run the changes locally:
  `mise exec -- go build -o async-wargame . && ./async-wargame -config game.conf`.
- Browser test sessions and the disposable test server were stopped after use.
- No deployment or live-server restart was performed.
- Broader future work is in `docs/roadmap.md`.
