# Wayward asynchronous play

Wayward has its own rules engine alongside the existing wargame engine. The
first playable slice is **The Silent Tollhouse**, a two-character outdoor
investigation with an AI Referee. It is an exploration playtest, not a complete
implementation of the RPG.

## Playing it

1. Configure the normal text AI endpoint, key and model. The separate image API
   remains optional and unchanged.
2. Create a room with **Wayward: The Silent Tollhouse (2 players)**.
3. Approve players and assign Mara and Rook, one each. A solo tester can hold
   both. Character cards show the six Skills, Archetype, equipment and derived
   abilities. The host can use test seats to play without reading private notes.
4. Start the adventure. This scenario enables the automatic Referee by default;
   starting through the web interface requires a configured AI client.
5. Discuss plans in Game Chat and submit one meaningful action per character.
   Drafts are not ready. When everyone has submitted, the next clock tick
   (within 30 seconds) locks the exchange. Otherwise the normal scheduled
   deadline locks it. Missing characters wait safely.
6. Read the published situation, your action's consequence and its dice. The
   server records discoveries and completes objectives. Use a materially new
   approach after failure; move to a location before investigating there.

The host administers access, schedule, pauses, retries and corrections. They need
not adjudicate normal turns. This is not yet a self-service game with no host
account, nor an AI that can repair every provider or interpretation failure.

## What is enforced

- Human pregens spend five Skill points, with no starting Skill above 3.
  This slice uses narrative Archetypes, Lore 0 and Combat at most 1, avoiding
  abilities that have not been implemented. These are playtest restrictions,
  not restrictions in the Wayward rules.
- Stamina is 14 + 3 × Physical; Defense is 8 + rounded-up half Combat;
  Connection slots are 3 + Social. Light armor gives Armor 2. These are
  reference values on a fixed sheet; there is no damage or advancement yet.
- Uncertain task throws use **2d6 + Skill** against 6+, 8+, 10+, 12+ or 14+.
  Omitted targets default to 8+. Natural 2 fails; natural 12 succeeds critically.
  No quality, veterancy, opposed d10 roll, or AI-authored dice enter this path.
- Every submitted action receives a routine/check/impossible/unsupported
  classification before dice are rolled. Destinations are committed during classification; resolution cannot move a different character, change the destination or omit a promised successful move. A check fixes both stakes before
  the result exists. Results persist through reload/retry; the server builds the resolution directly
  from those records without another model call. The AI briefing can summarize it.
- Scenario challenges have fixed allowed Skill/target pairs, a location and
  prerequisite discoveries. The server checks those, prevents duplicate
  attempts in the same exchange, and records success only from server dice.
  An attempted discovery whose prerequisites are missing becomes a blocked action with a player-safe explanation and no dice; other actions still resolve. Undiscovered challenge definitions stay out of player views.
- Generic checks can establish observations in the journal; they cannot award
  authored discoveries. Discoveries and attempt history commit together with
  the published turn. Reopening restores those, the board and the same dice.
- Failed, impossible and unsupported actions cannot move their character.
  Automatic resolution cannot change strength, status, resources or contact.
  Secret rulings stay private, including their rolls and consequences. Private
  audit reasons are separate from explanations shown to players.

The engine supports 1–6 characters for this slice. The existing 100-player
wargame load fixture does not apply to Wayward parties.

## Adaptation choices and remaining work

The sample adventure uses ten-minute outdoor exchanges and a 120m movement cap,
with movement to a challenge and attempting it separated into exchanges. That
asynchronous pacing, party-wide discoveries, fixed challenge difficulties,
one significant action per exchange and all-ready locking are **application /
scenario conventions**, not additional claims about the canonical rules.

The AI still judges plausibility, whether an order matches a challenge, whether
an approach materially changed, and the stakes of generic actions. The server
rejects an exact repeated failed order; semantic paraphrases still require AI
judgment. Free-form narration is not a mechanically proven privacy or
consistency boundary. Host editing remains an administrative override.

Not implemented: character creation/editing, demi-humans, feature Archetypes,
Combat Moves, initiative, attacks, damage, Wounds, Triage, morale, sorcery,
Hero Points and session pools, inventory/resource accounting, Wealth, XP,
advancement, travel/fatigue/light, Horror, Connections in play, factions, NPC
state and dialogue memory, or open-ended adventure generation. The playtest
asks the AI to defer actions requiring those mechanics. Structured mutations
of those resources are unavailable; narrative compliance still needs review.

Next implementation order:

1. Mutable character state and player-owned sheets: inventory, Stamina, Wounds,
   XP and an explicit session boundary. Add declared personal Hero Point
   contingencies and group-pool consent without blocking asynchronous play.
2. Native combat: Surprise, Initiative and action budgets, server attack/damage
   dice, Armor, critical tables, Wounds and Triage. Preserve declarations before
   rolls and publish one consistent encounter result.
3. Sorcery, costs and durations, learning, resistance and mishaps, with explicit
   capability checks before the AI can call any mechanic.
4. Exploration clocks, light, rest, fatigue, travel and encounters; persistent
   NPC attitudes, private knowledge, inventory transfers and Connections.
5. Flexible adventures and campaign memory: evidence-based goals, new scenes,
   NPC creation, factions, treasure and advancement. Replace the bounded
   authored challenge graph with validated state transitions and precedent.
6. Self-service party setup and larger campaigns only after complete-adventure
   AI-player tests, privacy tests and recovery checks pass consistently.

## Testing

Ordinary tests include all 36 2d6 pairs for each task target, natural extremes,
invalid task batches, readiness/deadline behavior, server-owned results,
restart/correction, private rulings, HTTP sheets and a five-exchange adventure
with fixed test dice. Existing wargame tests remain applicable to shared code.

A real-model disposable run uses player-filtered views and the normal worker:

```sh
mise exec -- go run . -config game.conf \
  -playtest data/playtests/wayward-001 \
  -playtest-scenario wayward-tollhouse \
  -playtest-games 1 -playtest-turns 10 -playtest-max-calls 60
```

This makes paid API calls using the configured provider. The output directory
must be new; calls, state and metrics are private test artifacts. It never
opens the live campaign data. No image requests are required.

### Real-model evidence (October 3, 2026)

Two AI players completed one adventure in nine exchanges, including a restart
and failed checks. Later tests exposed travel/check mixing, omitted destinations
and same-exchange dependency assumptions; the engine now fixes movement before
dice, resolves locally and gives blocked dependent actions a public explanation.
The final extended run published 12 more exchanges without stopping adjudication
but did not finish its story: the bots revisited unresolved leads. Five invalid
proposals recovered automatically. Better stalled-party guidance and AI player
planning remain needed; this is a playable exploration prototype, not a proven
open-ended autonomous Referee. All failed runs are retained in private test data.

Full tests, focused race tests, vet and build pass. Interactive visual testing
remains unverified because BrowserSkill has no connected browser.

## Rules source and attribution

Canonical source: [Wayward Core Rules](https://github.com/Ouroboros-Technology-LLC/wayward/blob/796f407d250e9953d811a22dba3c0182044c9e87/Core-Rules.html),
commit `796f407d250e9953d811a22dba3c0182044c9e87` (rules dated September 14, 2026),
read from `/home/admin/source/ouroboros/wayward`. `Draft.md` was not integrated;
the canonical repository was not modified.

Wayward!, by George Zackrison, based on Barbaric! Second Edition by Omer
Golan-Joel and incorporating material by Omer Golan-Joel, Richard Hazlewood,
Josh Peters, and Robert Garitta. Licensed under
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).

The selected rules text, pregens and scenario adaptation in this integration
are distributed under CC BY-SA 4.0 with the changes and omissions documented
above. The source's FTL: Nomad material retains its CC BY 4.0 attribution and
terms; its Wayward adaptations are CC BY-SA 4.0. No artwork, logos, trade dress
or FTL Companion text have been copied. See the upstream
[licensing notice](https://github.com/Ouroboros-Technology-LLC/wayward/blob/796f407d250e9953d811a22dba3c0182044c9e87/LICENSE)
for provenance and exclusions.
