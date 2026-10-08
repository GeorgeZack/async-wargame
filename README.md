# Async Wargame

Browser-based, referee-led asynchronous wargames. See [design-doc.md](design-doc.md).
The interface uses an era-neutral field-manual style: paper and olive in light
mode, charcoal and brass in dark mode, suitable for historical or future settings.

## Try it alone in five minutes

```sh
mise install && go run .        # then open http://127.0.0.1:8080
```

1. Under **Host a campaign**, pick **Crossroads Rearguard (2 players)** and create the room.
2. The checklist at the top of the Table has three steps. Press **Set up test
   players**: each unit gets a puppet player, with a link that opens it in its
   own tab.
3. If the server has an AI model configured, press **Let the AI referee run
   turns**. Without one you judge each turn yourself on the Desk.
4. Press **Start the operation**, write an order in each player tab, then
   **Lock turn** from the referee tab instead of waiting for the deadline.

Running a session with other people, and what to write down while you do:
[docs/playtest.md](docs/playtest.md).

## Wayward exploration playtest

Choose **Wayward: The Silent Tollhouse (2 players)** for asynchronous RPG play
with an AI Referee. Two fixed characters investigate a missing caravan using
native 2d6 Skill checks and persistent discoveries. Submitted actions resolve
when everyone is ready or the scheduled deadline arrives. The host sets up the
room; normal adjudication and publication are automatic.

This first slice covers exploration, not combat, sorcery, Hero Points or
advancement. See [Wayward setup, supported rules and next steps](docs/wayward.md).
The existing wargame scenarios keep their own rules engine.

## Run

```sh
mise install               # installs the Go version pinned in mise.toml
go run .                   # http://127.0.0.1:8080
go run . -addr :8080       # accept connections from other machines on the LAN
go test ./...
```

State is kept in `data/campaigns.json` (`-data` to change).

### A config file instead of a long command

Every flag can go in a settings file, one `name = value` per line, so the
server starts with just `./async-wargame`. It reads `async-wargame.conf`
from the working directory if there is one, or the file named by
`-config`. Flags on the command line still win.

```sh
cp async-wargame.example.conf async-wargame.conf   # then edit it
./async-wargame
```

```ini
addr = 127.0.0.1:8080
data = data/campaigns.json
ai = opencode
ai-key-file = ~/.config/opencode/api-key.env
```

Secrets go in the file rather than on the command line (where the process
list shows them): `ai-key` or `ai-key-file` (the key alone, or
`OPENCODE_API_KEY=...` lines like OpenCode's own file), `host-password`, and
`push-contact`. Environment variables, when set, win over the file.
`async-wargame.conf` is git-ignored; see `async-wargame.example.conf` for
every setting.

### Hosting

Run one copy on an always-on machine: the state lives in one file, and a
clock in the server locks turns on schedule. Put it behind a reverse proxy
or tunnel that provides HTTPS, and tell it so:

```sh
AW_HOST_PASSWORD='something long' ./async-wargame -addr 127.0.0.1:8080 -proxy -data /var/lib/async-wargame/campaigns.json
```

- `-proxy` trusts the proxy's `X-Forwarded-Proto` (so login cookies are
  marked Secure) and `X-Forwarded-For`. Leave it off if the server faces the
  internet directly.
- `AW_HOST_PASSWORD` is needed to create a room; players joining don't need
  it. Ten wrong guesses from one address lock it out for an hour. Without it,
  anyone who can reach the server can create rooms.
- Back up the `-data` file.

[deploy/](deploy/README.md) has a systemd unit, a Caddyfile, nightly
backups, a one-command `deploy.sh`, and step-by-step setup for a small VPS.

### The AI assistant

The referee's assistant can use a language model through any
OpenAI-compatible chat API. Pick one with `-ai`:

```sh
# OpenCode Go (an opencode.ai subscription and API key); defaults to glm-5.2
OPENCODE_API_KEY=... ./async-wargame -ai opencode

# OpenCode Zen (pay as you go, some free models)
OPENCODE_API_KEY=... ./async-wargame -ai opencode-zen -ai-model big-pickle

# Ollama running on the same machine
./async-wargame -ai ollama -ai-model llama3.1

# A model file run by the server itself, via llama.cpp's llama-server
./async-wargame -ai local -ai-local-model /srv/models/qwen3-8b-q4_k_m.gguf

# Anything else that speaks the API: LM Studio, vLLM, a llama-server you run yourself
./async-wargame -ai http://127.0.0.1:1234/v1 -ai-model my-model
```

- **OpenCode** (`-ai opencode`, the Go endpoint `opencode.ai/zen/go/v1`, or
  `-ai opencode-zen` for `opencode.ai/zen/v1`) needs a key in
  `OPENCODE_API_KEY` or `AW_AI_KEY` (to load it from OpenCode's file:
  `set -a; source ~/.config/opencode/api-key.env; set +a`). The server sends a session ID per run
  (`x-opencode-session`) and its own user agent, and at startup checks the
  model is on the provider's list, printing the list if it isn't. Some free
  Zen models log prompts or use them for training (see
  [opencode.ai/docs/zen](https://opencode.ai/docs/zen/)).
- **Cancelling:** every queued or running AI request has a Cancel button
  (next to the busy button that started it, and in the Desk's Assistant
  panel, with Cancel all). A request already waiting on the model is cut
  off, and an answer that arrives anyway is thrown away. Switching the
  assistant off for a room cancels its requests too.
- **Reasoning effort:** `ai-reasoning-effort` optionally selects `none`, `low`,
  `medium`, `high`, etc. Empty preserves the provider default. OpenRouter receives
  its documented `reasoning` object (`none` sends `enabled:false`); other compatible
  endpoints receive `reasoning_effort`. Use only a setting your model supports.
  With reasoning disabled, the tested DeepSeek endpoint completed the six-turn
  Crossroads mission in about three minutes; the earlier default-reasoning runs
  repeatedly timed out. This is a measured test result, not a latency guarantee.
  Reasoning and visible output can share the token budget; see the
  [provider documentation](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens).
- **Strict JSON:** `ai-json-mode = true` sends `response_format: json_object`
  on the prompts that ask for a JSON object (contests, resolutions, intentions),
  which cuts malformed replies and retries. It is off by default because not
  every compatible server or model accepts the field; briefings and drafts,
  which are prose, never get it.
- **Your own instructions to the model:** `ai-instructions` (one line;
  `\n` starts a new one) and/or `ai-instructions-file` are added to every
  prompt the assistant sends, for example "Answer directly. Don't reason at
  length." to cut a thinking model's reasoning. The referee sees them in
  Setup's AI assistant panel.
- **`-ai local`** starts `llama-server` (from [llama.cpp](https://github.com/ggml-org/llama.cpp);
  `-ai-llama-server` if it isn't on the `PATH`) on a private port with your
  GGUF model, waits for it to load, and stops it on shutdown. `-ai-context`
  sets the context size (default 8192). A 7–8B model at 4-bit
  quantization needs about 6 GB of RAM and is slow without a GPU, but the
  assistant works in the background, so slow is fine.
- Nothing is sent until the referee switches the assistant on for their
  room (Setup screen). What's sent: unit names and stats, orders and their
  intent, positions, and report lines. Not messages, and not anyone's name.

### Automatic referee (opt-in)

On the **Desk** or **Setup** screen, choose **Enable automatic referee**.
This uses the configured text model (`ai` / `ai-model`). Host the room, admit
players, set up the scenario and start the operation normally. Thereafter,
each scheduled lock starts a four-stage turn without a manual Publish step:

1. Decide enemy intentions without seeing player orders.
2. Decide uncertain contests; the server rolls and records the dice.
3. Resolve every effective order, including global squad orders and individual
   exceptions, plus enemy actions. Propose movement, formation splits/rejoins,
   losses, status/contact changes, reveals and threat changes.
4. Write the briefing from public outcomes and recorded public state changes, then apply the validated
   changes and publish the report together. The next planning deadline opens,
   unless the scenario’s objective checks complete the mission.

Larger turns are processed in persisted batches, targeting eight units each and
keeping formations together. Each unit/order has exactly one writer; other
orders remain read-only context. Dice relevant to a batch accompany its task.
All batches must pass the full-turn validator before any map change or report
is published. The referee can see batch progress on the Desk.

**Pause AI · take over** cancels pending work and rejects late replies.
`ai-retries` defaults to one automatic retry for a failed, unchanged stage
(0–2 supported). Validation feedback and the previous rejected proposal go to
the retry; an invalid resolution batch can split into smaller groups while
preserving completed batches and dice. Exhausted retries leave the turn
unpublished and offer **Retry adjudication**. Changes made while
an AI request runs invalidate its reply. A correction restores the game before
AI state changes and pauses automation for human editing. Settings, queued work,
dice and unpublished decisions survive reloads; interrupted running requests
can be reissued on startup. Model/provider, request stages, dice and the private
judgment reason remain on record.

`ai-concurrency` controls requests across rooms (default 1; maximum 16). A room
stays sequential so its stages cannot race. Gracefully interrupted requests
return to the queue; pause/cancellation still discard late work. Requests over
256 KB of prompt text stop for review instead of silently dropping context.

This is an initial automatic turn referee with deterministic regression tests and
a disposable real-model playtest runner. Real-model runs have exposed invalid
formation decisions and long provider waits; judgment quality is still experimental. The model judges
terrain and tactics; the server validates destinations, maximum movement reach,
formation consistency, losses, audiences, and complete order coverage. It does
not yet automatically manage proposed facts/random events, ask and wait for
player clarification, replenish forces, create new terrain, or advance between operations. Scenarios may define server-checked hold/extraction
objectives and mission success/failure; Crossroads Rearguard demonstrates this. Images keep their separate review workflow.
The scale harness supports 100 players. The server has been tested with 100
simultaneous authenticated clients, and automated campaigns exercise repeated
100-order turns. A real-model 100-order turn completed unattended in about nine
minutes (2m37s players, 6m26s judge), after earlier failures drove repair fixes.
Private judge steps notify referees only; publication and a stopped-judge
status change notify everyone. Browsers coalesce bursts of updates and defer refreshes in hidden tabs.
Batching still carries shared world/order context; it is not
full encounter isolation. Dense formations, long orders, tactical consistency,
long-range dependencies and long campaign histories need further testing. Result
prose can still contradict another group's unit changes; shared engagement
resolution is needed before treating this as reliable large-campaign judging.
Creating/closing rooms and configuring campaigns remain host actions.

### Two-player test scenario

Choose **Crossroads Rearguard (2 players)** when creating a room. It uses the
Border War gunpowder module: one player holds **Line Detachment** (12 infantry),
and the other holds **Scout Detachment** (8 scouts). Admit both on People,
assign one detachment to each, enable the automatic referee, then start. Players
coordinate through the in-app chat; the AI runs the raiders and adjudicates.

Hold the Crossroads uncontested at each publication through turn 4, then bring
survivors of both detachments to West Road during turns 5–6. The server evaluates
these conditions from recorded positions and strength, shows progress under
Objectives, records success/failure, and stops further orders and deadlines.
The final turn remains correctable. Starting the next operation is a host action;
it records the result and resets the scenario objectives.

A unit at **strength 0 is destroyed**: its crossed-out counter stays on the map,
it cannot receive orders or roll contests, and it no longer moves with its squad.
The referee can use **Mark destroyed** on its card or restore a mistaken loss by
setting strength above zero. Unpublished losses stay private until publication.
Players see one turn narrative; **Order results and rolls** expands the detail.
Private updates remain visible. The original mission briefing is kept separately.

### AI players against the judge

```sh
# Uses the configured text endpoint/model/key, never the live -data file.
# Optionally add -ai-reasoning-effort low if your model supports it.
mise exec -- go run . -config game.conf \
  -playtest data/playtests/crossroads-001 -playtest-scenario crossroads \
  -playtest-games 1 -playtest-turns 6 -playtest-max-calls 48

# Standard squad and a second game starting with a nearby enemy.
mise exec -- go run . -config game.conf \
  -playtest data/playtests/squad-001 -playtest-games 2 -playtest-turns 4

# Synthetic 100-player load fixture; eight player requests in flight.
# "none" is appropriate only when the configured model supports disabling reasoning.
mise exec -- go run . -config game.conf -ai-reasoning-effort none \
  -playtest data/playtests/load-100-001 -playtest-scenario crossroads \
  -playtest-players 100 -playtest-concurrency 8 \
  -playtest-games 1 -playtest-turns 1 -playtest-max-calls 180

# Continue a finished test (including a stopped adjudication) into a NEW directory.
# Source files and recorded dice stay intact; locked orders are not regenerated.
mise exec -- go run . -config game.conf -ai-reasoning-effort none \
  -playtest data/playtests/load-100-002 -playtest-from data/playtests/load-100-001 \
  -playtest-games 1 -playtest-turns 1 -playtest-max-calls 180
```

The output directory **must not exist**. Each player sees only their normal
filtered browser view and submits through the same order permissions. The runner
locks immediately, drives the normal referee worker, retries a rejected stage
once with validation feedback (without rerolling), and audits publication,
movement, casualties and report privacy. Completed missions stop early. The CLI
defaults to four concurrent players (1–16); squad commanders submit first so
subordinates can see shared orders. A 100-player fixture has independent commands
and opposing contacts across ten sectors, not the original mission objectives.

Outputs: `summary.json` (results, failures, calls, tokens, per-turn timings,
stage latency percentiles and largest prompts), `calls.jsonl`
(local prompts/replies/timing, including private referee context), and
`campaigns.json` (disposable state). Keep these local records private. The runner
has an overall `-playtest-timeout` (20m by default) and a per-completion
`-playtest-call-timeout` (2m). `-playtest-max-calls` bounds logical completions;
transient HTTP retries can send up to three requests per completion. No image
requests are made. A nonzero exit means a failed validation, provider call, audit,
or exhausted budget. Generated scenarios are saved under `scenarios/`; use the
run directory as `-modules` when opening a disposable store in the browser.
A clean run verifies mechanics, not the quality of every ruling; human review
still matters. For a provider-free 1,200-order soak test:

```sh
AW_SCALE_SOAK_TURNS=12 mise exec -- go test ./internal/playtest -run TestHundred -v
mise exec -- go test ./internal/web -run TestHundred -v
```

The optional fairness calibration makes six real resolution requests with fixed
dice, identical forces, and changes to names, list order, wording, and an
attempted instruction override. It records private JSON evidence and validates
state changes; a passing test does **not** certify tactical fairness. Set
`AW_FAIRNESS_API`, `AW_FAIRNESS_MODEL`, `AW_AI_KEY`, and `AW_FAIRNESS_OUTPUT`
(a new directory under an existing private directory), then run:

```sh
mise exec -- go test ./internal/game -run '^TestModelFairnessCalibration$' -v -count=1 -timeout=15m
```

Automatic judging permits one opposed exchange per pair of units per turn and
hides already-rolled dice during contest selection. Both affected orders share
those dice. Human referees can create distinct contests; replacing/deleting a
rolled result leaves a private audit entry. Enemy perception and casualty
severity remain model-dependent. See [the fairness audit](docs/worklog.md) for
measured results and unresolved limits.

### Situation vignettes (separate image API)

Each opening briefing and published situation report can have **one vignette**.
The referee reviews the scene description, generates a private preview, and
approves it for players. Hide, discard, or regenerate it from that brief.
No map or character images are generated.

Image generation has its own connection and runs independently of the text
assistant. No image model is selected by default. Once you choose one, configure:

```ini
image-api = https://api.openai.com/v1
image-model = your-image-model-id
image-key-file = ~/.config/async-wargame/image-key.env
image-size = 1024x1024
```

The key file contains the key alone or `AW_IMAGE_KEY=...`; alternatively use
`image-key` in the config or `AW_IMAGE_KEY` in the environment. Image generation
never falls back to `AW_AI_KEY`, `OPENCODE_API_KEY`, or `ai-model`.

The client uses the [OpenAI Images generation format](https://developers.openai.com/api/reference/resources/images/methods/generate):
`POST /images/generations`, one PNG/JPEG returned as `data[0].b64_json`.
GPT Image returns base64 by default; leave `image-response-format` unset for
those models. Compatible gateways that require an explicit request can use
`image-response-format = b64_json`. URL-only image responses are not supported.
Choose a size supported by your model; the default is `1024x1024`.

Suggested descriptions use only shared briefing/objective text and public
report outcomes. No private orders, briefs, notes, or rolls are appended.
The form shows the image provider and model before sending. Jobs run in the
background, one per room at a time, with cancellation and saved previews.
Cancelled, failed, or interrupted requests are never automatically retried;
the provider may already have charged for the work. Generated files live in
`maps/` next to the campaign state, with access checked on every request.

## How it plays

A referee-led, old-school game played over days: players say what their
characters do in their own words, the referee decides what happens, and the
dice decide only how well things went where the outcome is in doubt. The
starting scenario, **St. Ines**, is four colonial marines (a squad leader and
three marines, each played by one person) plus a referee: land at Red Beach,
cross the canal, reach the hospital, and bring the survivors out.

- **Orders are free text.** "Okafor crosses the bridge low and fast, stacks up
  on the side door, and if the shamblers turn, tosses a grenade and pulls
  back." The player can drop a pin on the map, but nothing else is required.
  Orders are drafted, committed, and frozen at the lock; the squad leader can
  approve one or send it back with a note.
- **One page for players:** the situation (latest report, briefing,
  history), the map, their character with a **private brief** only they and
  the referee can read, the order box, and game chat. The referee has four
  screens: Table (map, orders, game chat), Desk (judging, the other side, events,
  facts), People, and Setup (map and scale, scenario, schedule, settings).
- **The map** is the drawn hex map or an image the referee uploads (a
  sketch, a scan, a city plan), with a hex grid, a square grid, or none laid
  over it. Counters and pins go anywhere; the grid is for saying where and
  how far. The scale is on the map: "1 hex = 50 m · 1 turn = 10 minutes".
  Select a counter and tap anywhere to measure; the referee drags counters
  to move them.
- **Formations:** Lantern Squad starts as one counter, with a pip for each
  marine. The referee can split a member off into a round chit and rejoin
  them later. Moving the squad moves its attached members together. Opening
  the squad counter offers the squad overview; individual character links,
  including those in Orders this turn, open only that character's card.
  Private briefs remain restricted to their holder and the referee.
- **Objectives** appear as a numbered section below the briefing and are
  editable on Setup, one objective per line.
- **Global and individual orders:** someone commanding the whole squad can
  send one global instruction. An individual order takes its member out of
  that instruction; changing the global order preserves individual exceptions.
  Withdrawing an individual order returns the member to the existing squad
  instruction. This yields one to as many orders as there are members. The
  desk and report show shared instructions once; readiness still counts each
  member covered. Existing ownership, command, and radio restrictions apply.
- **Private planning:** players use **Plan a move** to sketch a destination;
  their own ghost and order pin show the request for adjudication. A full-squad
  drag preserves a shared order; an individual planned move becomes an exception.
- **Referee staging:** during adjudication, dragging counters stages their final
  positions. **Publish turn** applies all referee-staged moves with the report
  and clears temporary plans. **Clear staged moves** abandons those destinations.
  Player intentions are not automatically made real. Reopening a published turn
  restores its prior positions and staged moves for correction.
- **Writing an order:** opens a dialog; **Drop a pin** closes it while you
  choose a point, then reopens it with your text intact. Without JavaScript,
  the form stays inline. A brief announcement marks each newly opened turn.
- **Unit cards** show a marine's usual bonus (quality, plus veterancy),
  kit, status, notes, and established facts. The card doesn't list
  everything, and doesn't need to: the referee interprets.
- **Facts are co-created.** Players propose something true about their
  marine or the world from a card ("the hospital has a loading dock on the
  canal side"); the referee accepts it, changes it, or turns it down on the
  Desk. Accepted facts show on the cards and in "What we know" (or stay
  private between one player and the referee).
- **The dice:** when a situation is in doubt, each side rolls a d10 plus one
  bonus, everything that matters summed (quality, veterancy, cover, ambush,
  weather). The result is just the totals, "Okafor 5, the horde 9", and the
  referee reads them against what each side wanted, like an old Braunstein
  game. The server rolls, or the referee enters dice from the table.
- **The desk.** At the lock, every order lands on the referee's Desk.
  For each, the referee rolls where needed, writes what happened, and
  chooses who hears about it (the whole squad, only that player, or nobody
  yet). They move counters and change units (status, skills, kit, notes,
  radio) as they go; players keep seeing the board as it stood at the lock
  until the turn is published. Secret rolls stay on the referee's side of
  the report.
- **The other side.** On the Desk the referee notes what each enemy
  unit intends this turn, ideally before reading the players' orders.
  Players never see it; it guides the judging (and the AI's suggestions).
- **Asking about an order.** At the desk, the referee can send a player a
  question about their order as a private message; the AI can draft it.
- **The assistant (optional):** an AI model can suggest rolls from the
  orders (who rolls, and what counts for each side), draft outcomes from the rolls, and
  draft an opening narrative for each report, propose what the enemy
  intends, and draft questions about unclear orders. Suggestions sit unrolled until
  the referee rolls them; drafts until the referee keeps or rewrites them.
- **Fog and radios, by hand.** The referee decides what players know about:
  revealing a unit shows it as a contact, identifying it says what it is,
  and either can be done for everyone or only for chosen players (what one
  marine sees through a window, the others don't). A
  marine whose radio is out keeps their orders to themselves, and the rest
  of the squad sees them where they last called in.
- **Random events** (stragglers, runners, a screamer, the brute, a drone
  overflight, radio static) are proposals for the referee to apply or
  discard, with odds by threat level in the editor.
- **Reports** show each order with its outcome and any rolls players may
  see, plus anything else that happened. Until the next lock, the referee
  can reopen a published turn to correct it, with a reason players see.
- **The rest carries over:** rooms and room codes, referee approval and
  invite links, a squad leader (commander) and the players under them, test
  seats for trying it from several tabs, messages (all hands, players only,
  the command, private with the referee), web push notifications, the
  scenario editor, scheduled locks in the room's time zone, leaving and
  handing on units, and campaigns of several operations where survivors keep
  what happened to them.

A blind referee doesn't see players' orders until the lock (admin view shows
them, and players are told).

What's next: [docs/roadmap.md](docs/roadmap.md). Writing your own setting:
[docs/modules.md](docs/modules.md).

## Layout

```
main.go                       flags, wiring
content/                      embedded module + scenario JSON
internal/rules/               module schema and the dice (d10 + bonus)
internal/game/                campaigns: rooms, orders, resolution, fog, views, store
internal/game/engine.go       the rules-engine seam: d10 wargame and Wayward
internal/web/                 HTTP handlers, templates, static JS/CSS
internal/push/                web push: VAPID and payload encryption
internal/ai/                  OpenAI-compatible model client; managed llama-server
internal/assist/              background worker for the referee assistant
deploy/                       systemd, Caddy, Tailscale, backups, deploy script
```
