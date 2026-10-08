# Implementation roadmap

Updated 2026-10-03. This is the implementation sequence, not a delivery-date
estimate. Complete each milestone's validation before expanding its scope.

## Target experience

Players command persistent units, coordinate through the browser client, and
submit free-text orders before scheduled deadlines. An AI referee interprets
those orders, runs the opposition, resolves uncertainty, changes the world,
and publishes the next situation. A host can pause, inspect or correct the
campaign, but routine turns should not require human adjudication.

The reference is TheXPGamers' [Meta Campaign overview](https://www.youtube.com/watch?v=wBhJQaReuHg):
170 players commanding varied units in a cooperative campaign. Relevant examples
are the [scheduled order and enemy-action cycle](https://www.youtube.com/watch?v=wBhJQaReuHg&t=147s),
[unit persistence and customization](https://www.youtube.com/watch?v=wBhJQaReuHg&t=101s),
and [engineers and logistics restoring an airfield together](https://www.youtube.com/watch?v=wBhJQaReuHg&t=424s).
The video establishes the desired experience; it is not a complete combat
rulebook. Our rules and acceptance tests must make those judgments explicit.

The first complete demonstration should be **one small mission played from
briefing through success or failure without a human judging its turns**.
Prove that experience before attempting a campaign with hundreds of players.

## Wayward RPG track

The new goal is asynchronous Wayward play without a human adjudicator, alongside
the wargame campaign. The initial implementation provides human pregens, native
2d6 task checks, precommitted consequences and destinations, all-ready/deadline
locking, saved discoveries and an authored two-player exploration adventure.
It is not yet the full RPG. A real-model two-player run completed the adventure
in nine exchanges, including a restart and failed checks; validator rejections
were recovered automatically.

The next hurdle is mutable character state and asynchronous Hero Point choices,
then the complete server-enforced combat cycle, followed by sorcery, exploration
clocks and persistent NPC/campaign state. See [the detailed Wayward track](wayward.md)
for scope, conventions, source version, validation and the implementation order.
The 100-player wargame target does not imply 100-character Wayward party support.

## Current baseline

| Area | Current state |
| --- | --- |
| Browser campaign client | Rooms, roles, chat, maps, deadlines, objectives and situation reports exist. Discord is not a dependency. |
| Orders | Global squad orders with individual exceptions; individual character cards; private map plans. |
| Turn finalization | Referee-staged moves apply with publication; corrections restore the earlier state. |
| AI referee | Local opt-in prototype with real-model tests, validation feedback on retry, bounded recent-report memory, server dice and atomic publication. |
| AI playtesting | Up to 100 concurrent test seats, fresh-directory continuation, call/time limits, transcripts, timing/prompt metrics and repeated 100-order campaigns. |
| Casualties | Strength-zero counters stay visibly destroyed, leave active formations, and cannot order or roll. |
| Two-player mission | Crossroads Rearguard: gunpowder line infantry and scouts against AI raiders; server-checked hold/extraction objectives and a correctable final result. |
| Recovery | Pause, cancellation, retry, stale-response rejection, persisted stages and correction rollback are implemented. |
| Scale foundation | Persisted ownership batches, smaller repair batches, bounded retries, concurrent rooms, concurrent reads, and coalesced browser refreshes. Shared world context remains; this is not yet independent encounter scheduling. |
| Campaign continuity | Basic survivor and service records exist; requisition and richer progression do not. |
| Images | One optional vignette per brief, using a separate API, model and key with referee review. |

The earlier client work is pushed. At this roadmap's writing,
the automatic-referee prototype is local and not deployed. Go tests, race
checks and browser tests pass. **Real-model testing is in progress and has
exposed invalid formation decisions and slow/truncated provider responses.
The judge is not yet proven reliable.** See [worklog.md](worklog.md) for working-tree status
and test evidence.

The automatic decision format currently supports existing units' positions,
formation splits/rejoins, casualties, status, public reveals, radio state and
threat. It cannot yet implement infrastructure repair, cargo or supply changes,
new entities, arbitrary objective interpretation, or operation transitions.
Scenario-defined hold/extraction conditions now evaluate server-side; they do not
provide a general engineering/logistics action system. Describing one
of those actions in prose does not implement its consequences.

## Design requirements across all milestones

- Keep the player experience in the browser: orders, planning, communication,
  private intelligence, reports and notifications.
- Keep rules and capabilities in scenario/module data. Infantry, cavalry,
  phalanxes, artillery, aircraft and spacecraft should use appropriate module
  rules within the same campaign system. Avoid hard-coding the current setting
  into referee behavior or interface language.
- Let the model interpret intent and propose rulings. Let the server roll dice,
  validate changes, enforce permissions and persist authoritative state.
- Every claimed consequence must correspond to a recorded state change or an
  explicitly informational report. Reports and the map must agree.
- Keep public, command-level and private knowledge separate. Opposition decisions
  must use what that opposition could know, not privileged player orders.
- Preserve human takeover, decision explanations and correction support as the
  referee gains more authority. Retries must not reroll or spend resources twice.
- Keep vignette generation independent from text adjudication. An image failure
  must not block a turn, and images must not establish new gameplay facts.

## Milestone 1 — Dependable small-battle adjudication

**Outcome:** the existing automatic turn loop makes coherent, inspectable
rulings over repeated turns in a small scenario.

### Implementation

- Playtest St. Ines with a small group, first with a human observing the AI's
  proposed decisions and then with automatic publication enabled.
- Define the turn sequence explicitly: simultaneous intentions, movement,
  dependent actions, combat, opposition reactions and publication. Make timing
  decisions part of the rules rather than an accidental consequence of request
  order. Preserve global squad orders and individual exceptions throughout.
- Build a fixed suite of adjudication situations: incompatible movement orders,
  coordinated support, retreats, blocked routes, casualties, missing orders,
  detached members, hidden enemies and loss of contact.
- Add bounded campaign memory: relevant previous rulings, unresolved actions,
  established facts and current objectives. Preserve the source of each fact
  and separate it from model inference.
- Handle materially ambiguous or unsupported orders explicitly. Ask a private
  clarification when time permits; otherwise use a documented conservative
  fallback. Set an answer deadline so one absent player cannot stall the game
  indefinitely. Do not silently invent mechanics for unsupported actions.
- Make failures actionable: show the affected stage and a useful explanation,
  preserve completed work, and expose retry or takeover. Distinguish provider
  failure, invalid output and conflicting game changes.
- Record model/provider and prompt version, latency, token usage, retries,
  validation failures and human corrections. Establish acceptable cost and
  latency from measured playtests before increasing player counts.

### Completion criteria

- Complete multiple small-scenario turn sequences with an actual configured
  model, including deliberate API failures and server restarts.
- Each effective order receives one disposition; shared and individual orders
  neither disappear nor execute twice.
- Automated checks catch illegal destinations, inconsistent formations,
  unsupported changes and missing outcomes before publication.
- Privacy tests show that private inputs/results do not enter shared views or
  the public briefing's input. Human review also checks the model's wording
  for unintended disclosures.
- Human reviewers find the fixed adjudication cases consistent with the stated
  rules. Log disagreements and regress them before claiming the judge reliable.

## Milestone 2 — Referee authority over a living campaign

**Depends on:** Milestone 1's turn sequence, validation and decision record.

**Outcome:** cooperation changes persistent world state, and a small mission
can reach a recorded success or failure without manual adjudication.

### Implementation

- Extend the world model with facilities, terrain conditions, construction or
  repair progress, ownership and operational capabilities. Scenario data defines
  prerequisites and effects: an operational bridge permits crossing; a restored
  airfield permits specified aircraft services.
- Add typed actions for engineering, reconnaissance, transport, supply,
  treatment and supporting fire. Define costs, capacity, duration, range and
  completion conditions in modules. Player orders remain free text.
- Resolve dependencies across orders. An engineer's repair, a logistics unit's
  delivery and a defending unit's cover may contribute to one action. Validate
  the combined plan before applying it, including partial success and the loss
  of a contributing unit.
- Track relevant resources and cargo explicitly. Supplies, ammunition, fuel,
  passengers and wounded must move between recorded holders and cannot be
  duplicated by multiple orders or retries.
- Expand intelligence beyond a public reveal flag: observer-specific discovery,
  uncertain identification, stale reports and sharing through the command or
  communication system. Separate discovering a facility from knowing its
  condition or capabilities.
- Give enemy types scenario-defined priorities, perceptions and reactions.
  Add scenario triggers, reinforcements and bounded authority to create units
  or change terrain. Automate applicable random-event and proposed-fact review
  through the same validated action system.
- Represent objectives as tracked conditions, with mission success, failure,
  extraction and operation completion. Record survivors, losses, rewards and
  progression; add requisition and unit customization where the module uses
  them. Keep final results available before starting the next operation.
- Extend snapshots and corrections to every new resource, entity and objective
  field. Generated prose alone must never be the only record of an effect.

### Completion criteria

- Run an engineering cooperation test: delivery plus repair restores a facility,
  and its new capability becomes usable at the correct time. Missing supplies,
  interruption or casualties produce the corresponding partial/failure state.
- Verify reconnaissance and transport cases: only eligible observers receive
  discoveries; carried units and supplies have one location and one owner.
- Complete a small mission from briefing through recorded success or failure,
  including a meaningful cooperative action and an enemy response, without a
  human deciding routine outcomes or pressing Publish.
- Reopen and correct that mission's final turn; world state, resources and
  objective results return consistently, without duplicate rewards.

## Milestone 3 — Large cooperative campaigns

**Depends on:** a validated small campaign and explicit action dependencies.

**Outcome:** many players can affect the same campaign without forcing every
order and every historical fact into one model request.

### Implementation

- Divide a locked turn into encounters or areas with bounded context. Identify
  cross-area dependencies such as artillery support, transport and shared
  supplies before dispatching adjudication work.
- Reconcile encounter results against one authoritative turn snapshot. Prevent
  double casualties, conflicting destinations and duplicated resource spending.
  Decide how coupled encounters are combined or resolved sequentially.
- Add durable scheduling, bounded concurrency, progress reporting, request
  budgets and fair allocation between rooms. A failed encounter should preserve
  independent completed work without exposing a half-published turn.
- Expand command organization and scoped reporting so commanders can coordinate
  several formations while players retain control of their assigned units.
- Add linked map instances: their own units, places, intelligence and clock, with
  explicit travel between maps. Support a shared campaign clock where actions
  cross instances. Scope reports, facts and messages accordingly.
- Test larger groups incrementally before the roughly 170-player reference
  scale. Measure model context/output limits, total turn time, cost, browser
  responsiveness, persistence and recovery. Current whole-turn requests and
  validation limits must be revisited before that target is supported.
- Introduce a database or durable job infrastructure when measured persistence
  and concurrency requirements justify it; preserve migrations and corrections.

### Completion criteria

- A representative large turn completes within agreed latency and spending
  budgets, with every order accounted for.
- Cross-encounter actions produce one consistent result, including when workers
  retry, finish out of order or restart.
- Privacy, takeover and correction behavior remain correct at the target scale.
- Hosting capacity is documented from measurements, not inferred from the number
  of units the map can display.

## Follow-on and parallel work

- **Two-sided play:** separate player forces, command structures and intelligence,
  building on encounter isolation and linked instances.
- **Email notifications:** complement existing web push for deadlines and reports.
- **Turn history browser:** inspect the world and what each side knew at any turn.
- **AI-versus-AI test campaigns:** exercise the normal order flow for regression
  and stress testing; retain human playtests for judgment quality and usability.
- **Play-by-post view:** another presentation of the same campaign record.
- **Situation vignettes:** select and test an image provider/model when desired;
  retain one optional image per brief and the separate image configuration.

## Immediate next implementation slice

The automatic referee, destroyed counters, consolidated situation report,
AI-player harness and two-player Crossroads mission are implemented locally.
Continue comparing real-model campaigns against the deterministic suite, review
private/public wording, and regress the observed formation and latency failures.
The revised St. Ines run published three turns. After correcting OpenRouter's
reasoning parameter and disabling reasoning for the test, Crossroads completed
all six turns and mission success in about three minutes. Testing now covers
100 clients and 100 AI seats. Real large-room failures have exposed omitted and
misassigned order outcomes, prompting strict ownership checks, explicit batch
tasks and smaller repair groups. A subsequent 100-player turn completed
unattended in about nine minutes, but individual result prose still exposed
cross-group contradictions about enemy movement. Details are in
[worklog.md](worklog.md).
Crossroads provides a small mission with measurable conditions; its objective
engine is an early Milestone 2 foundation, not completion of that milestone.

The fairness audit also found and fixed duplicate opposed rolls, dice visibility
during contest selection, deadline lag, partial-state UI leaks and exact hidden
name leaks in public prose. Paired real-model calibration preserved the direction
of dice advantages but varied casualty severity. The remaining acceptance work
includes shared encounter decisions, explicit visibility/terrain constraints and
repeatable consequence bounds; see the current audit in worklog.md.

For the current 100-player target, next expand the real-model regression set to
dense formations, cross-group combat and longer histories. Measure complete turn
latency and token use, then reduce shared context through encounter dependencies
without losing simultaneous-order consistency. Use those measurements to decide
when persistence and scheduling need replacement. Keep Milestone 1's human
review and failure/restart acceptance criteria open until the actual judge meets
them; one successful synthetic fixture does not establish campaign readiness.

The next gameplay expansion remains one engineering/logistics action through
persistent facility/resource state, dependency validation, adjudication, UI and
correction support.
