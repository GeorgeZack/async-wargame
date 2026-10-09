# Handoff (2026-10-08)

Where the project stands and what to do next. The detailed record of past
work, measurements and failed runs is in [worklog.md](worklog.md); the plan is
in [roadmap.md](roadmap.md).

## State

- Everything through the automatic referee, the 100-player scale tests and the
  Wayward exploration slice is on `origin/master`.
- `go vet`, `gofmt`, `go test -count=1 ./...`, race checks for game/assist/web,
  and `node --check internal/web/static/app.js` pass locally. CI
  (`.github/workflows/test.yml`) runs the Go checks on every push.
- No human group has played a full game yet. Milestone 1 in the roadmap is
  open, and it is the next thing to close.
- The referee UI and original handoff tasks 1–3 are committed locally.
  Assistant run bookkeeping has moved onto `Assistant`; the remaining
  `Room` decomposition is optional follow-up. The owner pushes when ready.

## Changed on 2026-10-08

- **History rewritten** to remove a committed `.env` (an old, inactive key).
  Every commit hash changed; existing clones must re-clone or hard-reset to
  `origin/master`. `.env` files are now git-ignored.
- **Rules-engine seam** (`internal/game/engine.go`): the d10 wargame and
  Wayward are two implementations of one `engine` interface. Room code calls
  `r.rules()` instead of testing `r.Wayward != nil`; the only remaining test
  is the selector itself. A third ruleset is a new implementation, not new
  branches.
- **Failed runs carry `Invalid`** (validation failure versus provider
  failure) instead of the retry logic matching error text.
- **Setup checklist** on the referee's Table before the operation starts:
  players, referee, start. **Set up test players** creates and assigns one
  puppet player per unassigned unit.
- **`ai-json-mode`** (off by default) requests strict JSON on structured prompts.
- Posted forms are capped at 1 MB (the map upload keeps its 15 MB limit).
- Go is pinned in `mise.toml`.
- **Judge workspace:** map and chit selection beside an order-by-order
  ruling queue, explicit roll/move/save steps, enemy actions, and a review
  before publication. Both sides' moves use the map or coordinates and stay
  staged until publication. Edited form drafts survive live refreshes.
- **Solo referee play** works with player units left unassigned. Orders can
  be written on Table or Judge. A full solo turn and the test-player setup,
  order, commit and lock flow were exercised in a disposable browser at
  desktop and 390 px widths; no paid model was used.
- **Shared engagements:** canonical opposed contests and their connected
  participants have one resolution writer. Validated merged state fills a
  shared record for both parties. A separate model reporting pass regenerates
  provisional outcomes against starting/final state; it cannot edit state
  or audiences. Deterministic contradiction and connected-engagement tests
  pass, along with the 100-player soak.
- **Enemy awareness:** module/scenario distances, concealing terrain and
  sight blockers filter contacts on the server. Enemy prompts get estimated
  numbers and no global referee notes. Referee-only `revealedToEnemy` state
  records discoveries independently of player visibility. Privacy, persistence
  and concealed/revealed regression tests pass.
- **Assistant bookkeeping:** queueing, retention, cancellation and restart
  recovery now belong to `Assistant`, within the existing game package.
  Room authorization and worker APIs are unchanged.

## Known limits, most important first

1. **Judgment quality still needs people.** Batch reporting is regenerated
   against authoritative state, but natural-language agreement is not a
   deterministic proof that every sentence is correct. This adds one model
   request to batched turns. A connected engagement may exceed the target
   batch size; the existing prompt-size limit still applies.
2. **Enemy awareness is deliberately simple.** Drawn hex maps use terrain
   and edge sight blockers; image maps have no such geometry and use range.
   Revelation is explicit shared enemy knowledge, not per-observer memory.
   Clearing it when concealment returns is a referee/model decision.
3. **`Room` is still large**: about 250 methods in one package. The engine
   seam removed the cross-engine branching; assistant run bookkeeping is the
   first state decomposition. Membership, board and turn work remain.
4. **Storage.** One JSON file, rewritten and fsynced on every change under a
   global lock, with history never pruned. Fine for a few small rooms.
5. **Seat keys in URLs** (`/r/CODE/k/KEY`) appear in browser history and in
   any proxy access log you enable. The shipped Caddyfile enables none.

## Next

Run the sessions in [playtest.md](playtest.md) and record rulings people
disagree with. Fix what those sessions find before adding features. Do not
claim Milestone 1 complete from mock tests and solo browser checks.

## Working rules

- Never point tests or playtests at `data/campaigns.json`.
- `game.conf`, `data/` and anything under `data/playtests/` hold keys or
  private transcripts and stay out of git.
- Real-model playtests cost money; the commands are in the README.
