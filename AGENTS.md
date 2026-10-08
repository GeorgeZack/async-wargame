# Handoff for Codex (2026-10-08)

You are picking up a Go web app: a browser-based, referee-led asynchronous
wargame with an optional AI referee. Standard library only, no dependencies.
The owner's goal right now is **Milestone 1: a small battle that an AI referee
judges dependably, tested by real people**. Everything below serves that.

Read these, in this order, and nothing else until you need it:

1. `docs/HANDOFF.md`: current state and known limits (one page).
2. `docs/playtest.md`: what "Milestone 1 done" means.
3. `internal/game/engine.go`: the rules-engine seam you will build on.

`docs/worklog.md` is 550 lines of history. Search it when you need a past
measurement; do not read it through.

## Where things stand

- `origin/master` holds everything up to the Wayward slice. Its history was
  rewritten today to remove a committed `.env`; old commit hashes in docs and
  in your memory of this repo are dead.
- The latest commit on local `master` adds the engine seam, setup checklist,
  CI workflow, JSON mode and docs. It passes `go vet`, `gofmt` and
  `go test -count=1 ./...`. It is **not pushed**; the owner pushes when ready.
- `UPDATED_AI_INFO.md` is the owner's untracked note. Leave it.

## Tasks, in order

Do them one at a time. Finish and verify each before starting the next.

### 1. Verify the setup checklist in a real browser

It was only checked with static screenshots at desktop width, and its heading
changed afterwards.

- Run a disposable server (see Commands), create a Crossroads room, and click
  through: **Set up test players**, open both tester links, **Start the
  operation**, write and commit an order in each, **Lock turn**.
- Check the same flow at 390 px width.
- Fix what is broken or confusing. Template: `internal/web/templates/sections.html`
  (`setup-checklist`). Test: `TestSetupChecklistSeatsTestPlayers` in
  `internal/web/server_test.go`.

Done when: the flow works at both widths and you have said what you changed.

### 2. Shared engagement record across batches

The top judge defect. In rooms over 12 units the turn is split into ownership
batches (`internal/game/autobatch.go`). Each unit has one writer, but a
batch's prose can claim things about a unit another batch owns ("Opposition 01
broke east" while its recorded position is unchanged).

- Make both sides of an engagement resolve from one shared record, and reject
  or regenerate outcome prose that contradicts merged state.
- Start by writing a failing deterministic test that reproduces the
  contradiction with mock replies. Existing patterns:
  `internal/game/autobatch_test.go`, `internal/assist/autoreferee_test.go`.

Done when: that test passes, and the 100-player soak still passes
(`go test ./internal/playtest -run TestHundred`).

### 3. Enemy awareness filter

`intentionsPrompt` in `internal/game/intentions.go` gives enemy units every
player unit within three cells, with exact strength. There is no line of
sight or concealment, so stealth and ambush cannot be judged fairly.

- Add a deterministic server-side filter for what each enemy unit can know
  (distance, concealing terrain, whether the unit is already revealed), and
  pass only that. Keep it data-driven from the module/scenario, not hard-coded
  to one setting.

Done when: a test shows a concealed unit is absent from the enemy prompt and
present once revealed.

### 4. Continue breaking up `Room` (only if 1 to 3 are done)

`Room` has about 250 methods in one package. The engine seam removed the
wargame/Wayward branching; the remaining step is moving cohesive state
(assistant runs, membership, board) onto their own types. Do it one group at
a time, behaviour unchanged, tests green after each move. Do not start a
sub-package split without asking.

## Out of scope unless the owner asks

- New features, new scenarios, more Wayward rules.
- Replacing the JSON store with a database.
- Removing seat keys from URLs.
- Deploying, or touching any running server.

If you find a bug outside your current task, note it in your final message
and keep going. Do not fix it in passing unless it blocks you.

## Commands

```sh
export GOTMPDIR=$HOME/.cache/aw-go-tmp     # /tmp has a small quota; builds fail without this
mise exec -- go vet ./...
mise exec -- gofmt -l .                    # must print nothing
mise exec -- go test -count=1 ./...        # about 40 s
mise exec -- go test -race ./internal/game ./internal/assist ./internal/web
node --check internal/web/static/app.js

# Disposable server: pick a free port, never the default data file.
mise exec -- go build -o /tmp/aw . && /tmp/aw -addr 127.0.0.1:PORT -no-push \
  -data /tmp/aw-state.json -config /dev/null
```

A seat link `/r/CODE/k/TOKEN` authenticates without cookies, which makes
scripted and multi-tab testing easy. Stop any server you start.

## Hard rules

- Never open, write or test against `data/campaigns.json`. It is live data.
- Never read out, print or commit `game.conf`, `data/`, or anything under
  `data/playtests/`. They hold API keys and private transcripts.
- **Real-model runs cost the owner money.** Anything using `-playtest` with
  `-config game.conf`, or `TestModelFairnessCalibration`, needs the owner's
  go-ahead each time. Use mock clients for everything else.
- Commit or push only when the owner asks. Never force-push.
- Delete nothing you did not create.

## Conventions

- The model proposes; the server rolls dice, validates and persists. Never
  let a model reply set dice, bypass validation, or be the only record of a
  state change.
- New engine-specific behaviour goes behind the `engine` interface. Do not add
  `r.Wayward != nil` checks.
- Player-visible data is filtered on the server (`internal/game/view.go`).
  Any new field needs a test showing a player cannot see what they should not.
- Match the surrounding code: terse, standard library, table-free tests with
  explicit assertions, comments that say why.
- Report honestly. State what you ran and what you did not. A passing mock
  test proves mechanics, not judgment quality; say which one you have.
