# Handoff (2026-10-08)

Where the project stands and what to do next. The detailed record of past
work, measurements and failed runs is in [worklog.md](worklog.md); the plan is
in [roadmap.md](roadmap.md).

## State

- Everything through the automatic referee, the 100-player scale tests and the
  Wayward exploration slice is on `origin/master`.
- `go vet`, `gofmt` and `go test ./...` pass. CI (`.github/workflows/test.yml`)
  runs them, plus race checks for game/assist/web, on every push.
- No human group has played a full game yet. Milestone 1 in the roadmap is
  open, and it is the next thing to close.

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

## Known limits, most important first

1. **Judge consistency across batches.** In large rooms one batch's prose can
   describe an enemy doing something its recorded state doesn't show. Needs a
   shared engagement record. Small rooms (12 units or fewer) run as one batch
   and are not affected.
2. **Enemy awareness.** Enemy intentions see every unit within three cells,
   with exact strength; there is no line-of-sight or concealment filter.
3. **`Room` is still large**: about 250 methods in one package. The engine
   seam removed the cross-engine branching; splitting membership, board, turn
   and assistant state into their own types is not done.
4. **Storage.** One JSON file, rewritten and fsynced on every change under a
   global lock, with history never pruned. Fine for a few small rooms.
5. **Seat keys in URLs** (`/r/CODE/k/KEY`) appear in browser history and in
   any proxy access log you enable. The shipped Caddyfile enables none.

## Next

Run the sessions in [playtest.md](playtest.md) and record rulings people
disagree with. Fix what those sessions find before adding features. Then
limits 1 and 2 above.

## Working rules

- Never point tests or playtests at `data/campaigns.json`.
- `game.conf`, `data/` and anything under `data/playtests/` hold keys or
  private transcripts and stay out of git.
- Real-model playtests cost money; the commands are in the README.
