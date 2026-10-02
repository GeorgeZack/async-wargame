# Handoff: work in progress (2026-10-02)

Where things stand at the end of a long session, and what's next.

## State of the repo

- Branch `master`. Everything below "Done" is committed. Nothing pushed
  since `d896c3b` unless you've run `git push` yourself.
- `go test ./...` passes; `go vet` is clean.
- Untouched on purpose (not ours): the deleted `.env`, `UPDATED_AI_INFO.md`,
  and a stray `asyncwargame` binary (from a plain `go build`; delete it).
- Your settings live in `game.conf` (git-ignored, like every `*.conf` except
  `async-wargame.example.conf`). Run with:
  `mise exec -- go build -o async-wargame . && ./async-wargame -config game.conf`

## Done this session (committed)

- Free-placement board: drawn hex map or an uploaded image, hex/square/no
  grid, scale and turn length ("1 hex = 15 m · 1 turn = 2 minutes").
- Private briefs per character; per-player fog (reveal to chosen players).
- One page for players; Table / Desk / People / Setup for the referee;
  sections fold away (remembered per browser); Table and Setup split 50/50.
- New St. Ines map (22×16 hexes, harbor, beach, canal with road bridge and
  lock footbridge, walled hospital, pharmacy, plaza, police post, mast).
- Config file (`async-wargame.conf` or `-config`), including `ai-key-file`,
  `ai-instructions` / `ai-instructions-file`.
- OpenCode Go endpoint (`-ai opencode`, glm-5.2) with session header;
  `opencode-zen` preset; OpenRouter works as a plain base URL.
- Measure tool (M), multi-select and group drag for the referee, referee
  moves noted per turn on the Desk/Table and told to the AI.
- Who's-in check-in list for the referee (works blind).
- Cancel AI requests (one, by kind, or all; cuts off in-flight calls).
- Click-reachability audit passes on every screen at 3 sizes.

## In progress (uncommitted when this was written, then committed with this file)

### 1. Movement rates for the AI — DONE in code, needs a browser look
- `internal/rules/module.go`: `Class.Speed` / `Class.Run` (km/h).
- `content/modules/dark-coast.json` (now 3.1.0): speeds for every class.
- `internal/game/movement.go`: `ReachOf`, `ReachText`, `IntendedMove`
  (pin distance vs. what a unit can do in a turn).
- Prompts: `movePerTurn` on every unit, `wantsToReach` on orders, and a
  movement rule in `settingText`.
- Desk shows "Pin: wants to reach … (too far at a careful pace …)" and
  "Moves careful pace … a turn; at a run …".
- TODO: a unit test for `IntendedMove`/`ReachText`; check a module version
  bump doesn't upset existing rooms (they record `ModuleVersion`).

### 2. Formations (squad as one token) — MODEL ONLY, not rendered yet
Done:
- `internal/game/formation.go`: `Formation{ID, Name, Abbr}`,
  `FormationToken(id)` = `"f-"+id`, `members`, `together`,
  `moveWithFormation` (moving one member moves the whole token),
  `SplitOff`, `Rejoin`, `squadForAI` (sent to the AI as `squad`).
- `Unit.Formation`, `Unit.Detached`; `Room.Formations`; scenario
  `formations` and per-unit `formation`, copied in `newRoom`.
- `EditUnit` (At) and editor `unit-move` call `moveWithFormation`.

To do:
- **Scenario**: add to `content/scenarios/st-ines.json`
  `"formations":[{"id":"lantern","name":"Lantern Squad","abbr":"SQD"}]`
  and `"formation":"lantern"` on m1–m4.
- **Copy formations on EndOperation** (check `campaign.go` rebuilds units
  from the scenario; carry `Formation`, reset `Detached`), and add
  `Formations` to the correction snapshot only if they can change.
- **Map** (`mapview.go` buildMap): members that are `together()` and not
  stale → one `Stack` at the anchor, `Top` a synthetic `UnitView`
  (`ID: FormationToken`, name, abbr, symbol of first member, `Mine` if any
  member is the viewer's), plus `Pips` (one per member: in = filled,
  split off = punched hole) and `Members` (space-separated ids). Detached
  members → `Stack{Chit: true}` (small round punch-out) with a dashed
  tether line to the formation token. Add the synthetic unit to
  `Data.Units` with `members` and `card` (first member id) for JS.
  Needs `UnitView.Formation`/`Detached` filled in `unitView`.
- **Template** (`map.html` counter): `data-members` on the marker; pips row
  instead of the factors text; chit variant; tether layer under units.
- **JS** (`app.js`): `dropPiece` — a piece with `data-members` posts every
  member id at the new spot (`/units/move`, or editor `unit-move` ops);
  `inspect` for a formation lists members (name, class, player, status)
  with Split off buttons (referee); a chit offers Rejoin.
- **Web**: routes `POST /units/{id}/split` and `/units/{id}/rejoin`
  (call `SplitOff` / `Rejoin` through `s.act`).
- **Squad card**: `/units/{id}` for a member renders every member's card
  under the formation's name (wrap the current `<article>` in a loop;
  keep an element with `id="unit-card"` for the dialog). Player page
  `sec-character`: "With Lantern Squad: Okafor (Tester 2), …" and
  together / split off.
- Tests: together members move as one; split/rejoin; map draws one token
  and a chit; players can't split.

### 3. Still to start (from the user's last messages)
- **New-turn splash**: add `data-turn`/`data-phase` to every `<main>`;
  copy `turn` in `refresh()`; in JS, when the turn is higher than the one
  stored for this seat (localStorage), show an overlay "Turn N · orders
  are open" that fades out (static under `prefers-reduced-motion`).
- **Briefing setting**: first paragraph of the St. Ines briefing with the
  year and setting — far future, a colony world (Port Ines), comms went
  dark thirty hours ago. Players should know it's sci-fi, not a modern
  city.
- **Private planning map**: a "Plan" toggle on the map. In plan mode you
  drag only your own tokens (players: their characters / their
  formation; referee: anything); the token shows as a ghost at the planned
  spot with a dashed arrow, saved per member per turn
  (`Room.Plans []*Plan{Turn, Member, Unit, To}`) and shown to nobody else.
  A player's plan becomes their order's pin (and updates the current
  order's `Mark`). The referee gets "Apply my plans" to make them real
  moves (one batch through `MoveUnits`).
- **Order box as a popup**: the player's "Write an order" opens a
  `<dialog>` lightbox (the form stays inline without JS; Drop a pin closes
  the dialog while picking and reopens it after).
- **Headings**: "Your order, turn 1" and "Orders, turn 1" read badly —
  use "Your order" / "Orders this turn" and show the turn number in the
  status bar only.

## How to check things

- Test servers: `~/.claude/jobs/c4996113/tmp/vis/setup.sh` starts a
  throwaway room on 127.0.0.1:8611 (state in that folder, never your
  data/). `audit.mjs` checks every control is reachable; `cdp.mjs` drives
  headless Chromium (port 9333) with real mouse input.
- browser-skill (`bsk`, in `~/.local/bin`) drives your own Chrome;
  `bsk doctor` should pass.
- Never touch `data/campaigns.json` while testing.
