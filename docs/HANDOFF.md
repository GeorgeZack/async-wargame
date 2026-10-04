# Handoff (2026-10-03)

The previous client feature set was committed and pushed to `origin/master`
as **9a3abfe**. This revision includes the automatic AI referee, testing/mission work, fairness
audit and Wayward exploration playtest described below. Live deployment is separate
from publishing the source changes. The existing running
prototype and its campaign data have not been restarted or changed.

## Latest continuation: Wayward asynchronous RPG

User requested asynchronous play without a DM using their canonical Wayward
repository. Implemented a bounded **exploration playtest**, not the full book.
Source repository `/home/admin/source/ouroboros/wayward` was read, not changed;
canonical revision `796f407d250e9953d811a22dba3c0182044c9e87`. Draft rules were
not imported. Attribution and scope are in [wayward.md](wayward.md), the module
metadata and player UI. This revision includes these changes and the previous audit.

### Implementation

- Module `engine: wayward`; legacy modules retain the d10 wargame engine.
- Human narrative-Archetype pregens with six Skills, five starting points,
  Stamina/Defense/Connection-slot formulas and light Armor. Sheets are fixed
  reference values: no character builder, HP changes, Hero Points, combat,
  sorcery, inventory spending, XP or advancement yet.
- `internal/rules/wayward.go`: task 2d6 + Skill, default 8+, supported task targets
  6/8/10/12/14, natural 2 failure and natural 12 critical success.
- `internal/game/wayward.go`: one classification per canonical action before
  server dice, fixed success/failure stakes, private rationale separate from
  player explanation, declared destinations, unsupported-action boundaries.
  Routine actions must explicitly declare their ending position. Unchanged
  coordinates normalize to no move. Failed actions cannot move.
- `AutomaticReply` resolves Wayward rulings locally: no extra model request to
  recopy outcomes or reconsider positions after dice. The usual validated,
  atomic publication pipeline remains. The AI writes the final briefing.
- Persisted `Room.Wayward` stores scenario challenges, discoveries and attempts.
  Fixed Skill/target alternatives, prior discoveries, starting proximity and
  one attempt per challenge per exchange are enforced. Exact repeated failed
  order text is rejected; semantic equivalence remains AI judgment.
- Discovery mission objectives complete from server records, never prose.
  Corrections, scenario reset and operation restart include Wayward state.
- Automatic Referee is enabled when creating this scenario. All committed
  character actions trigger lock on the next clock tick (30 seconds); otherwise
  normal deadlines apply. Missing characters wait. Host access/setup remains.
  Web start rejects a missing text AI configuration.
- Scenario `wayward-tollhouse`: Mara (Merchant) and Rook (Burglar), three authored
  discoveries. Supports 1–6 individual characters; no formations or NPC units
  in this slice. Wargame 100-player fixtures are not Wayward load tests.
- Player UI: actions/ready wording, character sheets, discovery state, scope
  notice and attribution. Secret challenge definitions and private rulings are
  excluded. BrowserSkill still reports zero connected browsers; no interactive
  visual claim. HTTP render/privacy checks pass.
- Playtest bots receive role-filtered character Skills and discoveries, and
  exploration instructions (including when resuming). No image calls needed.

### Testing and live findings

- Exhaustive 36-pair dice tests; default target; invalid/duplicate/spoofed
  rulings; prerequisite and distance checks; all-ready/draft/deadline locking;
  frozen dice through restart/correction; exact consequence and destination
  enforcement; secret/private-rationale filtering; full five-exchange fixture
  with fixed test dice; HTTP character sheets and missing-AI error.
- First real-model adventure, `data/playtests/wayward-20261003-001` plus `002`,
  completed successfully on exchange 9. Total 49 calls, 139,333 tokens. Four
  invalid rulings recovered on automatic retries. This exposed private audit
  language in player explanations; the fields were separated afterwards.
- `003` exposed a check incorrectly combined with travel and stopped on its
  second invalid proposal. Unchanged coordinates now normalize away; actual
  travel during a discovery check remains rejected with more specific feedback.
- `004` exposed a redundant AI resolution inventing movement omitted from its
  own frozen ruling. Fixed by requiring routine destinations and resolving
  directly on the server. These failed artifacts are retained as evidence.
- `005` exposed repeated AI attempts to use a discovery that another character
  might gain in the same exchange. Missing prerequisites now produce a server
  “not yet possible” consequence with scenario-authored public guidance, no dice
  and no whole-turn retry. Old saved scenarios get a generic fallback.
- Final `006` resumed that older saved room under the new engine and published
  **12 additional exchanges**, 51 calls / 142,468 tokens. Five invalid proposals
  (one travel/check combination and four overlong fields) recovered on retry.
  The adventure remained active: bots repeatedly revisited blocked leads after
  early failed checks. Current scenario data gives more specific guidance than
  the generic fallback in this older room. Do not represent this run as a
  completed adventure or proof of reliable autonomous storytelling.
- Across `001`–`006`: 24 published exchanges, 127 model calls, 358,930 tokens.
  Failures are retained; no dice or state were edited to manufacture a win.
- Final full suite: `/tmp/aw-wayward-complete-tests.log`; focused Wayward races:
  `/tmp/aw-wayward-complete-race.log`; earlier broad game/web/rules/assist races:
  `/tmp/aw-wayward-race.log`. All pass. `go vet ./...`, build and diff checks pass.
  The ignored `async-wargame` binary is built from the new code. The binary and
  private playtest artifacts are not included in the source commit.
- Source config, live campaign data and existing running servers were untouched.
  Real tests use `-ai-reasoning-effort none` as an override. No keys were printed.
  `/tmp` hit its build quota; checks use
  `GOTMPDIR=/home/admin/.cache/aw-go-tmp` instead. Do not delete user data to fix it.

### Next work

Follow [Wayward implementation sequence](wayward.md): mutable character state and
asynchronous personal/group Hero Point choices, then complete native combat,
then sorcery and exploration clocks, then NPC/campaign state. Keep AI judgment
separate from server-owned arithmetic and state. Free-form narrative accuracy,
approach equivalence and challenge matching still require semantic audits;
this is neither full Wayward nor a proven autonomous open-world GM.

## Previous continuation: code, UI and fairness audit

The audit produced fixes, deterministic regressions and six real-model paired
calibration cases. The live prototype/config/data remain unchanged; changes are
local. Do not call the AI judge competitively fair or production-ready yet.

### Confirmed defects fixed

| Priority | Finding | Change and evidence |
| --- | --- | --- |
| High | Automatic contests could duplicate an exchange, including reversed sides. The saved 100-player turn actually contains two `e010`/`p095` contests. | One automatic opposed contest per unit pair per turn; reject duplicates, self-opposition and contests unrelated to their order. Both orders can inspect the same recorded dice. Human referees can explicitly create separate contests. |
| High | Later batches could see earlier dice while deciding what contests to add. | Contest-selection prompts keep definitions but omit results/timestamps/outcomes; resolution still receives the original dice. Regression verifies canonical dice remain unchanged. |
| High | Orders remained editable after the displayed deadline until the next 30-second clock tick. | Save, squad save, withdrawal, review and player planning enforce the deadline directly. Forms become unavailable and explain the closed deadline. HTTP regression proves a late replacement cannot change an accepted order. |
| High | Player maps froze at lock, but order slots/roster projections used current units and could reveal unpublished movement or destruction. | All player-facing unit projections use the locked unit state during adjudication, including cards and order panels. Intermediate contact-change logs stay private. Referees still see current state. |
| High | A visible but unidentified contact could reveal its real name and quality through automatic roll text. | Such automatic contests are forced private, as already done for hidden/limited contacts. This guards structured roll output; free-form prose still needs semantic privacy review. |
| High | Live turn 3 named “Wood Raiders” while that unit was an unidentified contact. | Public outcomes and publication now reject canonical names of unrevealed/unidentified units; prompts provide safe public labels. Regression covers both resolution and briefing retries without partial publication. This catches exact normalized names, not every paraphrase or tactical leak. |
| Medium | A stopped AI judge left players seeing an indefinite working message. | Players now receive a generic needs-referee-attention status and an SSE update, without model errors or private batch details. |
| Medium | Manual rerolls/deletions erased prior dice without an audit message. | Private referee log records replaced/deleted results. This is an audit aid, not a tamper-proof log or a restriction on host authority. |
| Low | Generic screens still labelled gunpowder troops “marines”/“infected” and used radio-specific instructions. | Neutral unit counts, order guidance and contact labels; corrected a hidden desk section's dangling ARIA heading reference. |

### Fairness measurements

- Exhaustive dice arithmetic test: equal bonuses across all 100 d10 pairs yield
  45 wins, 45 losses, 10 ties; swapping sides reverses the margin exactly.
  Server dice are mechanically symmetric. The AI still chooses which uncertain
  actions need rolls, situational bonuses and the consequences of those rolls.
- Six real-model calibration calls use two isolated, identical volley exchanges
  with fixed +9/-9 margins. Variants change names, list order, dice, order
  verbosity and an explicit attempted instruction override in the losing order.
  All responses passed state validation; the larger roll retained the advantage,
  reversing dice reversed the advantage, and the instruction override did not
  obtain the requested zero-loss/enemy-destruction result.
- Casualty counts still varied: the losing side lost **3–7 of 12**, the winning
  side **0–2 of 12**. This small sample mixes model randomness (temperature 0.3)
  with prompt perturbations; it does NOT establish a causal name/order/verbosity
  bias, statistical reliability or prompt-injection immunity.
- Calibration: OpenRouter `deepseek/deepseek-v4.1-flash`, reasoning disabled for
  these tests only; six completions, **18,009 reported tokens**, **86.95 seconds**.
  Private evidence: `data/playtests/fairness-20261002-232710/`.
  Reusable opt-in test: `TestModelFairnessCalibration`; see README.
- End-to-end regression: `data/playtests/2026-10-02-fairness-regression`, room
  `8U5F6P`, **three published Crossroads turns, 18 completions, 51,048 reported
  tokens**, about 84 seconds, no rejected stages or provider failures. This is
  three turns, not a completed six-turn mission. Semantic review found the
  unidentified-name leak above despite valid state publication; the new guard
  was added after this run.
- Privacy continuation: `data/playtests/2026-10-02-fairness-privacy` published
  turn 4 in about **59.4 seconds**, six completions, **19,584 reported tokens**,
  no rejected stages or provider failures. Its briefing now calls the still
  unidentified force an “unidentified contact in the trees”, while naming the
  already-identified East Raiders normally. Original evidence was preserved.

### UI and validation scope

- Full Go suite, vet, JavaScript syntax, build and whitespace checks pass.
  Game/assist/web race checks pass. The initial combined race invocation was
  terminated before playtest/web finished; the focused 100-player/restart
  playtest race check subsequently passed in 146.4 seconds.
- HTTP/rendering checks cover deadline errors, escaped order markup, private
  reports, permissions, stopped-judge status and frozen unpublished state.
  Disposable 100-player player/referee pages were fetched and checked for
  duplicate IDs and missing ARIA references. The player map response is about
  839 KiB uncompressed, so payload size remains a scale consideration.
- **Interactive browser audit is incomplete:** BrowserSkill v0.3.2's daemon is
  healthy but reports `no_browser_connected`; the extension did not connect even
  after launching Chromium. The user was asked to connect it. No fresh visual,
  keyboard or mobile-browser verification is claimed for this audit. The user
  acknowledged the connection request, but the final status check still showed
  zero browsers. No BrowserSkill session was created. The disposable server on
  8087 is stopped; Chromium was left open for the user to connect the extension.

### Highest-priority remaining findings

1. **Shared encounters across batches:** one writer per unit prevents conflicting
   state writes, but independent decisions can still invent incompatible effects
   on the other side. Bind both sides of an engagement to one shared resolution
   record and derive public/private prose from the merged state.
2. **Enemy awareness/terrain:** enemy intention inputs include every nearby unit
   within three cells, with exact counts/status; there is no deterministic
   line-of-sight or concealment filter. Movement reach is enforced, but terrain
   traversal and tactical legality remain model judgments. Add explicit awareness
   and route constraints before relying on stealth/ambush balance.
3. **Repeatable consequences:** define scenario-appropriate bounds for casualties,
   suppression and withdrawal based on margins and conditions, while retaining
   the intended free-form referee style. Expand calibration with repeated seeds/
   samples and dense formations; do not grade fairness from one successful turn.
4. **Live UI verification:** reconnect BrowserSkill, test player/referee workflows
   on desktop and phone, including keyboard controls and a stopped judge.

## Earlier continuation: 100-player AI stress testing

All changes remain local and the live prototype is untouched. Preserve the
user's tracked `.env` deletion and untracked `UPDATED_AI_INFO.md`/`asyncwargame`.
The newly built executable is `async-wargame` (with a hyphen). No credentials or
private playtest transcripts belong in a commit.

### Implementation

- Large rooms use persisted adjudication groups targeting eight units, keeping
  formations together and giving every unit/order one owner. Other groups are
  read-only context. Enemy intentions precede contests, server dice and decisions;
  the complete turn is validated and published atomically. Empty enemy groups
  need no API request. The Desk shows progress. Unit-change validation scales
  with actual room units instead of imposing the former 200-edit ceiling;
  duplicate and unknown unit edits remain invalid.
- Invalid model decisions receive exact feedback and the rejected proposal as
  untrusted retry data. Failed resolution groups can split while retaining
  accepted groups and recorded dice. Automatic retries default to one
  (`ai-retries`, 0–2); stale inputs require explicit referee intervention.
- Worker concurrency across rooms is configurable (`ai-concurrency`, 1–16), with
  round-robin claiming and one active stage per room. Graceful shutdown requeues
  interrupted work. Prompt size is bounded; requests are not silently truncated.
- Player test requests run concurrently with a configurable cap. Synthetic
  fixtures support 100 independent commands and ten opposing units. Player
  prompts use filtered views and compact map/history data. `-playtest-from`
  resumes a finished run into a NEW directory, preserving original evidence.
  Stage timings, prompt sizes, usage, call/time budgets and restart audits are
  recorded. See README for repeatable commands.
- Store reads can run concurrently. Browser updates are coalesced and deferred
  in hidden tabs. Private worker steps wake referees/internal watchers only;
  publication wakes everyone. SSE subscription setup catches intervening changes.
- Corrected OpenRouter's reasoning request format. Real tests below explicitly
  disable reasoning through CLI flags; `game.conf` was not changed.
- Tightened limited-contact privacy, secret contests and same-turn radio-loss
  reports. Public narration receives authoritative public movement/casualty facts
  to reduce contradictions with validated state. Semantic prose still needs review.

### Measured validation

- Full Go suite, `go vet`, JavaScript syntax, whitespace checks and build pass.
  Race checks pass for game, assist, playtest and web. Tests cover concurrency,
  cancellation, checkpoint restart, repair ownership, stale replies, privacy,
  dice preservation and atomic publication.
- Controlled 100-player combat soak: **12 turns, 1,200 orders, 1,200 server
  contests**, a destroyed opposing unit, persisted reloads and injected invalid
  decisions. Passed in 91.8 seconds with 1,659 mock completions. This proves
  mechanics under the fixture, not real-model tactical quality.
- HTTP test: **100 authenticated clients with 100 live SSE connections**, each
  posting an order and fetching its filtered page concurrently. All orders
  persisted and privacy checks passed. Local p50 0.917 s, p95 1.091 s, maximum
  1.114 s; race instrumentation is considerably slower.
- BrowserSkill checked the actual 100-player map on desktop and 390×844 mobile.
  A burst of 100 server updates caused one page fetch and caught up to the exact
  server version. No overflow or application JS exception observed; map zoom
  remains useful with many counters. Capture `/tmp/aw-100-ui-debug.json`, images
  `/tmp/aw-100-ui-desktop.png` and `/tmp/aw-100-ui-mobile.png`. Test server 8087
  and the browser session are stopped.

Real provider: configured OpenRouter `deepseek/deepseek-v4.1-flash`, CLI override
`-ai-reasoning-effort none`. All paths below have the prefix
`data/playtests/2026-10-02-`; tokens are provider-reported usage, not billed cost.

| Run suffix | Result | Logical calls | Reported tokens |
| --- | --- | ---: | ---: |
| `crossroads-reasoning-off` | All six turns published; mission success in about 3 minutes. One rejected reply repaired. | 37 | 119,153 |
| `load-100` | All 100 players submitted; invalid batch ownership/missing outcomes blocked publication. | 117 | 1,746,988 |
| `load-100-retry` | Continued saved turn; five private resolution groups completed before another omitted-order failure. | 9 | 223,882 |
| `load-100-repair` | Smaller repair groups completed and turn 1 published, preserving prior work/dice. About 2m15s of remaining work. | 7 | 138,908 |
| `load-100-turn2` | Continued into a fresh second turn: all 100 AI orders and adjudication completed unattended, with no rejected replies or provider failures. 9m04s total. | 139 | 2,084,267 |

The first 100-player turn needed code fixes and continuation runs. It is evidence
of successful recovery, not an uninterrupted first-attempt pass. Its 105 report
entries cover all 100 canonical orders plus five independent results. The public
narrative was inspected against movement/casualty state.

The second 100-player turn took 156.49 seconds for players and 386.32 seconds
for the judge (14 groups, ten model-generated enemy-intention replies, fourteen
contest replies, fourteen resolutions and one publication). It produced exactly
100 canonical order entries plus two independent results, advanced to turn 3
planning, and left no pending/running/failed requests. Six friendly detachments
lost strength, two moved, and three opposing units moved and lost strength.
The public narrative names those changes. Maximum judge prompt was 80,827 bytes;
resolution p50 was 14.01 seconds and p95 46.91 seconds.

**Known semantic defect found in review:** an individual result for Detachment
002 describes Opposition 01 breaking east, but Opposition 01's authoritative
position is unchanged. Groups have strict write ownership, yet can narrate
inconsistent effects on units owned by another group. The aggregate public
narrative follows actual state, but this does not repair individual result prose.
Next test/implementation should tie cross-group engagement outcomes to a shared
resolution/dependency record and validate or generate prose from merged facts.
Do not claim full tactical consistency from the successful publication.

### Remaining scale work

Shared whole-world/order context is still sent to groups; this is not independent
encounter scheduling. Dense formations, long orders, cross-group combat and long
campaign histories need broader real playtests and human tactical review. The
JSON store still rewrites state on updates; sustained large campaigns or multiple
server processes need a measured database/scheduling design. Engineering,
logistics, reinforcements and operation transitions remain roadmap work.

## Earlier continuation: casualties, playtests, and two-player mission

This section records the earlier baseline. The real six-turn Crossroads result
and 100-player work above supersede its provider/scale status.


- Strength-zero units are destroyed: crossed-out, labelled counters remain at
  the casualty location, leave active formations, and cannot order or roll.
  Survivors can move away in the same adjudication that kills a member.
  Referees can mark a unit destroyed on its card or restore strength; stale
  enemy contacts do not expose unpublished casualties.
- Players get one primary situation narrative. Public order results/rolls are
  expandable; private updates stay visible. Original mission and character
  background are separate collapsed sections. Objectives sit below the report.
- **Crossroads Rearguard (2 players)** uses the new Border War gunpowder module.
  Assign Line Detachment and Scout Detachment to two approved players, enable
  the automatic referee, and start. Hold the junction through turn 4, then
  extract survivors of both detachments to West Road during turns 5–6.
- Scenario-defined hold/extract rules are evaluated by the server on publication,
  including enemy presence, positions and strength. A final result stops orders
  and deadlines. Final-turn correction restores objective and unit state and
  pauses automation. A host can start the next operation and retain the result.
- `-playtest NEW_DIRECTORY` creates disposable campaigns and per-seat AI players
  through normal permissions and filtered views. It supports `st-ines` and
  `crossroads`, limits calls/time, records local private transcripts, retries
  a rejected stage once without rerolling, and audits published changes. It
  branches before opening the configured live data file. See README for commands.
- Real replies exposed formation inconsistencies and slow/incomplete provider
  responses. Fixes include precise retry feedback, explicit formation rules,
  bounded recent-report memory (included in the stale-input fingerprint),
  truncated-output rejection, accurate cancellation errors, and token accounting
  across HTTP retries. Optional `ai-reasoning-effort` is recorded for future runs;
  no model/provider settings were changed in the user's `game.conf`.

### Earlier validation

- Full Go suite, race checks for game/assist/playtest/web, `go vet`, JavaScript
  syntax, build, and whitespace checks pass.
  The deterministic AI-player stress tests cover 36 published turns across four
  games, with injected bad judgments, restart, casualty, privacy and budget cases.
  Mission tests cover hold/contested areas, extraction deadlines, annihilation,
  publication atomicity, completion controls, correction and next-operation reset.
- BrowserSkill exercised disposable rooms, including actual scenario creation,
  destroyed counters/cards, report expansion and mission completion. Desktop and
  390×844 phone layouts checked. Latest screenshots:
  `/tmp/aw-crossroads-light.png`, `/tmp/aw-crossroads-mobile.png`,
  `/tmp/aw-situation-final.png`, `/tmp/aw-destroyed-ui.png`.
  Capture: `/tmp/aw-casualty-mission-ui-debug.json`; extension resource failures
  and deliberately restarted SSE connections were observed, no app JS exceptions.
  Task-owned browser sessions and test servers on 8086/8087 are stopped.
- Real-model evidence uses the configured OpenRouter
  `deepseek/deepseek-v4.1-flash`. Transcripts and disposable stores are private,
  git-ignored files under `data/playtests/`; never commit them or `game.conf`.
  Reported tokens below are provider usage, not exact billed totals (cancelled
  calls may omit usage).

| Run directory (2026-10-02 prefix) | Result | Logical calls | Reported tokens |
| --- | --- | ---: | ---: |
| `-a` | Initial two St. Ines games: 0 and 1 turns; formation validation and timeout failures, which prompted fixes. | 24 | 182,796 |
| `-b` | Revised St. Ines: all 3 requested turns published; one timeout recovered, no formation failures. About 13m31s. | 25 | 212,761 |
| `-crossroads` | 2 turns published before a deliberate interruption to add objective evaluation. This was not a completed mission. | 14 | 46,482 |
| `-crossroads-final` | No publication; resolution exceeded the 2-minute limit twice. | 6 | 10,656 |
| `-crossroads-low` | With `-ai-reasoning-effort low`: turn 1 published, turn 2 resolution timed out twice. No partial changes published; mission remains active. About 8m32s. | 12 | 34,299 |

The successful St. Ines run and Crossroads' first turn were inspected for
matching narrative/state, split/rejoined formations, casualties, and hidden
reserves remaining private until revealed. They demonstrate mechanics, not
reliable tactical judgment.
The full six-turn Crossroads mission is verified with deterministic responses,
but **had not yet completed with the real provider at that point**. Low reasoning effort did not
eliminate resolution timeouts. Preserve these failures as a latency baseline;
benchmark a smaller resolution prompt or an explicitly chosen alternative model
before further broad real-model runs. The CLI call cap was 2 minutes; normal
referee worker calls allow 5 minutes. Optional effort metadata was added after
these processes started, so their older summaries omit it.
The harness's `validationFailures` field also includes provider attempt failures.
Current limits remain provider latency, semantic privacy/narrative review,
model-judged terrain/path legality, and unsupported engineering/logistics/new
entities. No automatic operation transitions or large-campaign scheduling yet.

## Future implementation plan

[roadmap.md](roadmap.md) is the prioritized plan for the user's Meta Campaign
reference: dependable small-battle adjudication, broader campaign actions and
mission completion, then large cooperative campaigns. It records current
limitations, dependencies, concrete acceptance criteria and the next small
implementation slice. This roadmap update does not deploy or enable the AI.

## Earlier automatic AI referee baseline (2026-10-02)

- Desk/Setup provide Enable automatic referee, Pause AI / take over, and Retry.
  The existing configured text model handles four persisted stages: enemy
  intentions (no player orders), contests with server dice, a validated complete
  turn decision, and a public-only briefing followed by atomic publication.
- Handles global/individual orders, enemy units, positions, formation separation
  and reunion, casualties, statuses, contact/reveals and threat. Host setup and
  operation lifecycle remain manual. Empty-order turns are supported.
- Proposed changes stay private until final publication. Failure does not apply
  half a decision. Retries preserve dice; concurrent gameplay edits invalidate
  replies. Pause/cancel rejects late work. Correction restores positions and
  statuses before AI changes and pauses automation. Closing or ending an
  operation cancels obsolete automatic work.
- Existing normal assistant behavior stays available when automatic mode is off.
  Image generation remains independent and reviewed, with no real provider used.
- Full Go suite, vet, race checks for game/assist/web, and diff whitespace checks
  pass. Tests cover mock HTTP end-to-end, persisted publication, correction,
  private briefing inputs, complete order coverage, invalid changes, stale replies,
  authorization, retry and cancellation.
- BrowserSkill room `D4HYR9`, state `/tmp/aw-autoref-ui/state.json`: enabled,
  paused, re-enabled, started/locked a turn; mock API failure showed stopped
  state; after fixing mock empty-order handling, Retry finished turn 1 and opened
  turn 2 with all four stages done. Fixed misleading manual-publish guidance
  during automatic work. Debug: `/tmp/aw-autoref-ui-debug.json`. Test processes
  and Agent Windows have been stopped. Live server/config untouched.
- At this earlier baseline: mock-tested, no real-model campaign yet. The model judges terrain
  and tactics; code enforces basic physical/state constraints. Proposed facts,
  random event application, clarification conversations, reinforcements/new
  entities, objective completion, and operation transitions are not automated.
  One whole-turn prompt is unsuitable for hundreds of players without batching
  and bounded scene context. README describes the current scope.
- User now asks how close this is to TheXPGamers' cooperative Meta Campaign:
  https://www.youtube.com/watch?v=wBhJQaReuHg . Original English automatic captions were subsequently retrieved after the
  initial captions 429; read the whole transcript at
  `/tmp/aw-video-reference.en-orig.json3`. Key comparison: 170 players; scheduled
  player movement/actions followed by preplanned enemy behavior (2:27–3:30),
  recon/intelligence (3:35–5:52), coordinated engineer/logistics airfield repair
  (7:04–7:21), mixed infantry/air/orbital combat, persistent units and requisition
  (1:41–2:25). Current automatic schema cannot implement environmental repair,
  supply/refuel/cargo, private map discoveries, or campaign rewards; those need
  structured actions/state, followed by encounter batching for large games.


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
Use `bsk` for future browser testing. The current workspace has full filesystem
access; do not request escalation or start a replacement for the shared daemon.

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
