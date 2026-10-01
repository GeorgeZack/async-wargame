# Asynchronous Wargame Platform

**Concept:** Friends play referee-led wargames—and, if they choose, RPG-like
scenarios—through a browser. The server handles maps, scenarios, units, orders,
sitreps, adjudication, and campaign records. A referee assistant bot—working
name: **Jev**—helps process play at an asynchronous pace.

**Design status:** Platform and starter game outlined; module format and core
resolution procedure need prototyping.

## Platform and game modules

The platform is a shared play environment, not one fixed setting or ruleset.
The referee creates or loads a game module, chooses a scenario, configures its
scale and time, and invites players. A module defines its mechanics, unit
classes, orders, map layers, player roles, and how many units each player
commands. The same browser client and server support the different games.

Possible modules include:

- **Fantasy dungeon:** one player commands one character, perhaps with a
  hireling, and gives orders while exploring a hex-mapped dungeon.
- **Napoleonic battle:** one player may be Napoleon, another a corps commander,
  with more players filling lower command roles in a battle such as Austerlitz.
- **American Civil War raid:** a player commands Stonewall Jackson's force on a
  raid against Union flanks, with subordinates added as the group grows.
- **Science-fiction rescue:** space marines evacuate civilians from a zombie-
  overrun colony in the starter scenario below.

These examples have very different unit counts, command structures, rules,
and game clocks. A small fantasy turn might cover a few minutes; an operational
or historical game turn may represent an hour or much longer. The referee and
module choose the map scale, clock, and player-to-unit mapping. Hexes are the
current shared map foundation, with each game defining what one hex represents.

The intended experience is asynchronous play with friends: players submit
orders when available, the referee and Jev process the turn, and results wait
in the dashboard for players to read and respond to. The service provides the
play space; modules supply the game.

## Design references

These games solve different parts of the problem; they are references for
design patterns, not rules to copy.

- [*Rangers of Shadow Deep*](https://modiphius.net/en-us/collections/rangers-of-shadow-deep)
  is a solo/co-operative miniatures campaign. It is a useful reference for
  scenario-led cooperative play with a human-scale party.
- [*Aliens: Another Glorious Day in the Corps*](https://www.gf9games.com/aliens/)
  is a cooperative survival game. Its hidden contacts, threat pressure, and
  enemy activation phases are good patterns for suspense and a readable PvE
  loop.
- [*Five Parsecs From Home: Tactics*](https://modiphius.net/products/five-parsecs-from-home-tactics)
  supports solo/co-op battles, varied scenarios, and campaigns where characters
  and outcomes carry forward. It is a useful model for connecting battle
  modules to campaign progression.
- [*Combat Mission: Shock Force 2*](https://www.slitherine.com/product/combat-mission-shock-force-2)
  is a detailed tactical simulation with simultaneous WeGo planning, fog of
  war, morale, command, terrain, and battlefield effects. It shows the
  simulation depth a computer can resolve, though its individual-unit scope
  would need aggregation to serve very large player groups.

Together these suggest separating the **campaign**, **orders and command**,
**scenario**, and **resolution mechanics**. The browser app and Jev should
depend on stable interfaces between these parts, not on one combat ruleset.

## The play cycle

1. **Join and organize.** Players join a campaign, choose or receive a role,
   and join a command group. Groups can represent formations, sectors, or staff
   functions.
2. **Read the situation.** The server publishes each player the reports their
   role is allowed to see. The referee holds the full, authoritative state.
3. **Plan and review.** Players draft orders. Commanders and other ranks above
   them review subordinate orders and return, edit, or approve them before the
   shared deadline. The referee also prepares enemy orders, directly or with
   Jev, without seeing player orders unless the scenario allows it.
4. **Lock orders.** Player and enemy orders lock at the same deadline. A
   tactically foolish order that passed through the chain stays in force; Jev
   does not silently repair it.
5. **Resolve simultaneously.** Jev evaluates the locked plans over the same
   time interval. Movement, contact, attacks, communications, and other events
   interact in time; neither side takes a complete I-go-then-you-go turn. The
   referee reviews issues and approves the resulting state.
6. **Publish results.** The server records the approved state changes and
   delivers role-appropriate reports. A new planning window begins.

Turns use a low-frequency cadence rather than real-time play. The server can
run background analysis while the referee is away. Players return to the
browser dashboard when they have time; there is no need to be online together
or wait for the referee to resolve every action live. Play-by-post may be added
later, but it is outside the first version.

## Browser game and hosted campaign

One player or organizer hosts a campaign server and shares a room ID. Players
need only a current web browser—no desktop client, app-store install, powerful
GPU, or command-line setup. For the first version, a player enters the room ID,
chooses a display name, and requests access. The referee approves each member
before they can see the map, reports, discussion, or orders. After approval,
the player gets a short briefing and can join the operation. Keep this flow
simple while making membership decisions explicit to the referee.

The same responsive web app serves phones, tablets, low-powered computers, and
large desktops. Design the core experience around readable reports, text
orders, simple forms, and lightweight maps. Maps should have a list or grid
fallback; core play must work without WebGL, animation, a fast connection, or
large downloads. Use touch-friendly controls, keyboard navigation, screen
reader labels, clear contrast, and layouts that do not require wide screens.
Save drafts on the server, recover cleanly after disconnects, and keep the
current phase and next action obvious.

A campaign contains its rules version, map, units, groups, reports, orders,
deadlines, and turn history. Hosting can start as a private, single-server
game; the rules should not depend on one particular hosting provider.

The server is the campaign's shared workspace and record keeper. It should
support:

- room IDs, pending membership requests, referee approval, and access roles;
- groups with a leader, members, assigned units, and private discussion/orders;
- public, group-only, and referee-only information;
- order drafts, revisions, validation, commit, and deadline lock;
- turn status, notifications, reports, and an audit history;
- queued referee-assistant analysis that can pause for human decisions;
- referee tools for changing the hidden state, editing scenarios, inspecting
  unit statistics, resolving encounters, and publishing approved results;
- optional generated visual Briefs attached to approved combat results (later
  feature; ordinary text sitreps remain the baseline).

The server stores the official state. Players may only see the state and reports
permitted by their role. Every committed order and referee-approved change is
timestamped and retained, so the group can reconstruct what happened.

### Technology principles

- **Browser-first, single origin:** campaign, player views, and referee
  dashboard live in one web app. Do not require separate chat or mapping
  accounts to play; optional integrations can come later.
- **Thin client:** run rules resolution, AI calls, permissions, and hidden-state
  filtering on the server. Keep the client usable on modest devices and avoid
  exposing secrets or model credentials to browsers.
- **Progressive interface:** ordinary HTML forms and links are the baseline;
  JavaScript can make maps and updates smoother, but losing it should not block
  reading reports or submitting an order.
- **Low-bandwidth by default:** mostly text, compressed small maps, incremental
  updates, and no need for constant polling or a live connection. Notify players
  when their action is needed; let them return later.
- **Simple hosting first:** one deployable web service and a durable database
  are enough for the first campaign. Keep background analysis in a resumable
  job queue. Add separate services only when measured load requires them.
- **AI behind a server adapter:** a campaign selects an available model or
  local provider; the browser never needs direct model access. Store model,
  prompt/configuration, and rules versions with the analysis record.

Use standard responsive web technology rather than making the stack depend on
a specific framework. A progressive web app may be offered for optional
home-screen access, but installation must never be required.

### Briefs: later feature

A **Brief** is an AI-generated image that illustrates a resolved combat or
other notable event. Jev supplies the approved referee judgement, relevant
scenario context, and involved unit classes and conditions to an image model.
The referee chooses a visual style for the game (for example, field sketch,
comic panel, or period illustration), and can request, edit, approve, or skip a
Brief from the dashboard.

Briefs illustrate the story; the map, unit state, and referee-approved report
remain authoritative. Generate images only from the facts the intended readers
are allowed to see, and publish them with the same visibility as the report.
The first playable version can omit image generation entirely while keeping
the data model able to attach a Brief to a resolved event.

## Players, groups, and orders

For the first game, the referee assigns each approved player their force
elements. In larger games, players join command groups and receive roles.
Membership changes take effect at the next turn unless the referee approves an
urgent change.

On departure, the referee reassigns the player's units, open drafts, and
private reports. A player who leaves cannot keep access to future group
information. A late joiner receives only the access needed for their new role.
If a player misses the order deadline, their assigned units have no offensive
capability for that turn. They follow any still-active standing order and act
only to survive or maintain it; they cannot initiate attacks or begin new
offensive tasks. This absence rule lasts for that turn and does not change the
unit's class or permanent capability.

An order says:

- **who** acts and **what** they intend to accomplish;
- **where** and **when** they act, and for how long;
- acceptable risk, constraints, and requested support;
- a fallback if the plan becomes impossible.

An order remains a draft until committed. A player may revise or withdraw it
before lock; the server records versions but shows the active version to the
appropriate command group. At lock, committed orders become immutable for that
turn. Missing, late, invalid, and conflicting orders follow published rules.

Orders are visible to friendly players with access to the relevant command
network by default: players can inspect one another's orders while they have
access to that network. Server submission records an order, but does not mean
other units have received or seen it in-world. Terrain, distance, weather,
damaged relays, jamming, and other communications conditions can delay, garble,
or prevent in-game order sharing. A superior can review an order only after it
reaches their command link; the lock deadline still applies. The game records
what each unit and commander could actually know at each point in the turn.
Players may coordinate through outside channels, but the simulation only
models communication represented in the campaign. Hidden enemy positions
remain concealed unless detected; friendly order visibility does not reveal
what the enemy knows.

## Jev, the referee assistant

Jev is an AI-assisted referee bot attached to the campaign server. It works
through a locked turn in stages, then gives the referee a manageable queue of
issues rather than demanding one long, uninterrupted adjudication session. For
the first version, players and referee work entirely in the browser dashboard:
map, orders, review queue, decisions, and reports are all in one place.
Play-by-post can later be added as another view over the same campaign record.

### Turn analysis

After orders lock, Jev builds a versioned snapshot of the relevant orders,
friendly and enemy state, environment, reports, and mechanics module. It then
works through the turn at a configurable pace:

1. **Order review:** crawl every order for completeness, authority, resource
   availability, conflicting assignments, timing, reach, and feasibility.
2. **Encounter building:** group overlapping orders and likely contacts into
   encounters; identify ambushes, support, crossfire, reinforcements, and
   competing objectives.
3. **Resolution:** calculate outcomes allowed by the selected mechanics module.
   Deterministic calculations use the rules engine; embedded AI can interpret
   intent and surface interactions the formal rules do not settle.
4. **World update:** assess environmental situations such as weather, terrain,
   visibility, fires, hazards, supply, and communications, then assess their
   effects on units and objectives.
5. **People and force state:** update morale, suppression, cohesion, casualties,
   readiness, and command effects according to the module; flag uncertain or
   consequential judgments for the referee.
6. **Issue creation:** turn unresolved questions, surprising outcomes, and
   consequential choices into review items, with proposed resolutions and
   draft reports.

The analysis may run as a job queue, checkpointing progress so it can pause,
resume, or be re-run after a referee ruling. Independent encounters can be
analyzed in parallel, while dependencies—such as whether a unit reaches an
objective before an enemy does—must be resolved in the module's order. The
referee can set a target pace and prioritize urgent issues; there is no need to
simulate a real-time frame loop.

### Referee issue dashboard

Each issue should be a compact decision card containing:

- what happened and why the issue needs human attention;
- the involved orders, units, locations, reports, and rules/module version;
- what facts are known, uncertain, or hidden from which players;
- Jev's proposed resolution, alternatives, and likely consequences;
- controls to accept, edit, reject, ask for more analysis, or return for player
  clarification.

Issues are prioritized, for example: **blocking** (later resolution depends on
it), **referee judgment**, and **informational** (review if desired). After a
decision, Jev resumes dependent work and drafts player-facing reports. The
referee reviews and publishes them from the dashboard with the right visibility.

**Jev can:**

- remind players about deadlines and missing orders;
- check order format, authority, assigned units, and available resources;
- flag duplicate, conflicting, impossible, or underspecified orders;
- group orders by location, time, unit, and likely interaction;
- calculate rule-defined outcomes and compare plausible interpretations;
- analyze combat encounters and their likely outcomes;
- assess environment, weather, morale, logistics, and cascading effects;
- explain which rules, facts, and assumptions support its proposals;
- draft referee issues, state changes, turn summaries, and player reports;
- in a later release, prepare image prompts for optional Briefs from approved
  combat judgments and visibility-filtered scenario and unit information.

**Jev should not silently:**

- invent player orders or change a player's committed order; enemy orders are
  generated only when the referee has explicitly delegated that turn's enemy
  control mode;
- reveal hidden information to an unauthorized player;
- decide ambiguous, disputed, or high-impact outcomes without referee review;
- directly change canonical state or publish a turn result before the referee
  approves it.

Each result should be marked as a rule-engine calculation, an AI assessment, or
a referee decision. Show the exact inputs and module version used. Preserve the
original proposal when the referee changes it, record the final ruling and
reason, and re-run dependent calculations from the saved snapshot. AI can help
interpret messy orders and reason across many interacting conditions; the
versioned mechanics module remains the source of formal rules, and the referee
remains the source of final rulings.

## Scale and command structure

The module defines the player-to-unit relationship and command ladder. One
player might command one character and a hireling; another module might place
Napoleon above corps commanders, corporals, and squad leaders. A small group
can directly command all assigned units. As players join, add command roles
and review links appropriate to that game's scale.

| Players | Example role mapping (module-defined) |
|---|---|
| 1–2 | Each player directly commands their character, squad, or formation |
| 3–12 | Add commanders and subordinate leaders as needed by the setting |
| 13–40 | Delegate sectors, corps, or parallel objectives to player teams |
| 40+ | Multiple fronts, command layers, and referee staff |

Where a module has a hierarchy, leaders can review orders from subordinate
roles within their command. Reviews can catch contradictions, coordinate
support, or return an order for revision before lock. The module says who may
edit or approve which orders; the server keeps the author, reviewer, and
revision history. A module can make its lowest military command role control
multiple squads, while an RPG module can make one character the smallest
player-controlled unit.

Review is an opportunity, not a guarantee of good orders. Jev can flag a
potentially infeasible or dangerous plan, but warnings do not automatically
block a syntactically valid order. If no superior catches a ludicrous order
before lock, it executes as written and its consequences stand. This makes the
chain of command meaningful under a deadline.

## Rules and adjudication

### Starting resolution model: layered matchups and dice pools

The common starting mechanic is an elaborate rock-paper-scissors model. An
action's effectiveness depends on interacting types and circumstances rather
than a single universal attack score. The module defines matchup relations
between the actor's class, weapon or approach, target class or posture, and
conditions such as terrain, cover, range, weather, surprise, morale, and
communications. Matchups establish which factors favor each side; unit quality,
quantity, readiness, and supply set the strength of the result.

For the first tactical module, translate those factors into opposed dice pools.
For example, the referee might set one side to 10d6 and the other to 20d6
because of battlefield conditions and the relative quality or numbers of the
units. A simple first procedure to playtest is: each pool rolls d6, each die
showing 4 or higher scores one success, and opposing successes cancel. The
remaining margin maps to an outcome table in the module, from no meaningful
change through a decisive result. A matchup may change pool size, success
threshold, or available outcomes. These numbers and the outcome table are
starting defaults to tune through playtests, not universal rules for every
genre. The server should show which factors produced each pool and the proposed
outcome, not hide the calculation behind one opaque AI answer.

The referee dashboard can calculate pools from the module, then let the
referee change them, add or remove conditions, choose a die type, roll through
the server or enter external roll results, and record a reason. This allows
manual adjudication when the referee disagrees with Jev or wants a ruling the
module does not encode. Preserve both the suggested and final values in the
turn record. Jev may recommend a ruling; the referee can resolve the encounter
directly.

### Unit classes, weapons, and supply

Units are instances of reusable classes, not individually customized pieces.
A class defines its standard movement, weapons, ammunition capacity, armor,
sensors, communications, and supply needs. The campaign places class instances
on the map and tracks their current location, condition, remaining ammunition,
and other temporary state. Players choose tasks and routes; they do not design
every soldier, aircraft, or ship's loadout.

The tactical starter should include a small roster such as a marine rifle
squad, heavy machine-gun squad, engineer or medic squad, and transport/support
elements. Future modules can define attack aircraft, fighters, or destroyer-
class spacecraft, each with a standard class profile and engagement range.
Weapon type matters because it determines effective range, targets, damage,
and ammunition use. Track ammunition by weapon or supply type at the class
instance level; make resupply require time, access to a source, and a viable
route. Logistics should matter without requiring players to count every
individual cartridge.

### Hex map and time

Hexes are the map primitive throughout the game engine. A hex has terrain,
elevation, infrastructure, visibility, weather exposure, ownership or
control, and other module-defined properties. Movement, range, adjacency,
line-of-sight, communications, and area effects refer to hexes. Units and
contacts occupy hexes or move along paths through them.

Scenarios set the scale of a hex. A planetary map can use large operational
hexes; a city operation can use denser tactical hexes. A campaign may link a
city map to its planetary parent hex, so local outcomes update campaign-level
control, intelligence, and objectives. Quick modules can bundle adjacent
hexes into named sectors for display or orders, while the underlying state
remains hex-based.

Each turn represents a shared interval of time. Both sides issue orders before
that interval begins. For the first game, use **one in-game hour per turn** as
a starting target, divided internally into short simulation slices so a unit
can move, communicate, and fight during the hour. The slice size and hex scale
need playtesting together: one hour should be long enough for meaningful
movement and rescue activity but short enough that orders still feel like
immediate tactical decisions. Players do not submit a second order after
seeing the enemy's full turn.

The engine resolves events against one timeline, with initiative, simultaneous
arrival, detection, and attack timing handled inside the interval. An order can
specify a route, task, posture, and fallback. Player and enemy actions progress
over the same slices; neither side takes a complete turn while the other waits.

Fog of war and communications friction are core mechanics, not presentation
effects. Each unit has local knowledge; command receives reports through
available links, with timestamps and possible delay. Weather, terrain,
distance, damage, and enemy interference can isolate squads, hide movement,
delay a correction, or leave a commander acting on stale information. Standing
orders and local initiative let units continue when cut off. The referee and
Jev must distinguish what happened from what each side knew when it acted.
Reports after resolution can reveal the gap between the two.

### Replaceable mechanics modules

Each campaign selects a versioned mechanics module. A module defines the rules
used to resolve play; the server owns accounts, groups, order submission,
deadlines, permissions, history, and reports. Scenarios define maps, objectives,
forces, and starting conditions. Keeping these separate lets a referee choose a
fast abstract ruleset or a detailed simulation without changing the campaign
server.

Start with **JSON for declarative rules and data**, validated against a schema.
Do not treat arbitrary JSON expressions as executable code. The game engine
should interpret a documented set of operations (comparisons, modifiers,
tables, state changes, and random draws). If referees later need genuinely new
resolution logic, add a separately versioned, sandboxed extension interface;
do not silently evaluate uploaded scripts on the server.

A module should declare:

- its ID, version, scale, phase order, and supported engine/API version;
- player-to-unit mapping, command hierarchy, game clock, and map scale;
- unit attributes, states, resources, actions, and legal targets;
- order fields and validation rules;
- matchup relations, dice-pool modifiers, roll procedures, and referee override
  points;
- resolution steps, timing, modifiers, and outcome tables;
- uncertainty/randomness rules and any enemy behavior procedures;
- state changes and reportable facts, including visibility rules;
- example scenarios and test cases.

The engine exposes a stable input/output contract: validated orders plus the
campaign state go in; proposed events, state changes, explanations, and
player-visible facts come out. The module should identify which steps are
exact calculations, which permit referee discretion, and what information an
AI assistant may use or reveal. Jev executes explicit rules, helps interpret
unstructured intent, organizes results, and flags exceptions. The referee
approves the proposed turn before it becomes canonical. Store the module ID and
exact version with each campaign and turn so old results remain reproducible.

Changing modules mid-campaign can change the meaning of existing units,
orders, and state. Initially, lock a campaign to its selected module version.
Later, allow a change only through an explicit migration or a new campaign
phase with a recorded conversion.

### Starter module: Dark Coast Rescue

The first scenario is a tactical rescue operation on a colony world. A populated
city and its surrounding infrastructure have gone dark. Space marines deploy
to investigate and discover that a zombie outbreak has spread through the
city. They must reach civilians, stabilize routes and facilities, and extract
as many survivors as possible to a defended beachhead. Large dropships wait
offshore to carry evacuees away.

The scenario map uses hexes for the city, ports, roads, utilities, hospitals,
evacuation routes, coastline, and nearby terrain. The beachhead is literally a
beach on the water's edge. Large space-going dropships can hover over open
water, where they wait to take on evacuees, but cannot safely hover over land.
They cannot move inland to pick people up or ignore blocked coastal approaches.
This makes the shoreline and access from the city operationally important:
marines must secure a usable beach, move civilians there, and keep the approach
clear while the ships embark them. The scenario should represent the ships as
offshore hexes and the beach as adjacent land hexes; precise embarkation and
transfer rules belong to the tactical module.

Civilian groups have locations, size estimates, needs, and evacuation status.
Extraction depends on reaching them, protecting or guiding their movement,
keeping routes or transport functioning, and delivering them to the beachhead.
The primary measure of success is how many civilians are evacuated; combat is
in service of the mission rather than a kill-count goal.

Dark Coast Rescue is one operation in a possible planet-wide campaign. Later
operations can eradicate or contain the outbreak, locate and destroy regional
hives, rescue other settlements, secure sensitive military or biological
information, restore infrastructure, and decide what to do with infected
zones. The initial rescue outcome changes what forces, intelligence, and
options are available for the planetary campaign.

For the starter rules, zombies are the primary enemy. The module should leave
room for bug enemy profiles and mixed outbreaks later, using the same hex map,
order, and resolution interfaces but distinct movement, detection, resilience,
and behavior rules. Model hordes as groups or uncertain contacts, not as
individual orders for every infected person.

The first game uses the **tactical** profile. The module format may permit
other resolution profiles over time:

- **Quick:** formations and broad areas; few attributes; one roll or table per
  contested action; coarse casualties and fast turns.
- **Tactical:** hex-level squad positions, line of sight, cover, weapon classes,
  ammunition, morale, and simultaneous orders.
- **Simulation:** explicit time, range, movement, visibility, ammunition,
  logistics, command delays, and detailed effects, with the server calculating
  repetitive interactions.

These can be separate modules that implement the same server contract. Avoid
one module full of complexity toggles until testing proves that the variants
remain understandable and compatible. Automation can make a detailed ruleset
feasible, but players still need to understand what they can order and why an
outcome occurred. Offer a concise player-facing explanation for every
automated resolution.

The starter module can begin with four familiar unit characteristics:
**Movement**, **Attack**, **Range**, and **Armor**. Define these for its scale.
Other modules may replace them or add detection, readiness, morale, supply, and
command/control as needed.

The referee resolves relevant movement, detection, attacks, defense, and
logistics using orders, timing, terrain, preparation, information, and unit
capabilities. Comparable situations should use the same procedure. Keep a
record of state changes and the reason for major rulings. Each module may map
its matchup and dice-pool results to outcomes in its own way; the server's
dashboard, order history, and referee controls remain common across modules.

### Enemy control

The enemy has objectives, capabilities, and limited information. Each turn,
the referee chooses how much control to delegate, subject to the campaign's
published rules:

- **Directed:** the referee writes and commits the enemy orders.
- **AI-assisted:** Jev proposes orders from the enemy doctrine, goals, and
  information; the referee reviews, edits, and commits them.
- **Hands-off:** Jev generates and commits the enemy orders without turn-by-turn
  referee input. The referee can still pause the game, inspect the plan, or
  intervene if the campaign permits it.

Enemy orders are prepared during the same planning window and lock
simultaneously with player orders. A referee can mix modes by faction or turn—
for example, directly command a boss while delegating routine swarms. By
default, the enemy AI acts on enemy-available information and does not inspect
player drafts or committed orders before lock. Scenarios may deliberately
change that rule. Enemy orders remain hidden from players until reports reveal
their effects.

Store enemy doctrine, AI model/configuration, information available, generated
orders, and any referee edits with the turn record. The mode should be visible
to the referee and available to players if the campaign's chosen level of
transparency calls for it.

The rules also need a clear process for clarifications, disputes, corrections,
missed deadlines, referee conflicts of interest, and changes between turns.

## First playable version

Test the smallest complete version before building a large-scale platform:

- three people total: one referee and two players, all using the browser
  dashboard; the referee approves both players' room access;
- one city-and-coast hex map with marine formations, civilian groups,
  infrastructure, zombies, and an offshore beachhead;
- one rescue mission with optional intelligence or hive objectives over 3–5
  turns;
- a small tactical class roster with distinct weapons, movement, ammunition,
  and resupply needs; no individual loadout customization;
- a provisional one-hour in-game turn, tested with simultaneous movement and
  combat resolution;
- browser-only access through a room ID, pending referee approval, and a
  responsive dashboard for orders, map, and referee issue review;
- a shared lock followed by simultaneous resolution of player and enemy plans;
- Jev crawls every order, groups encounters, calculates rule-defined effects,
  and queues uncertain combat or environmental judgments for the referee;
- review those issues in the dashboard, then approve the turn before publishing
  reports;
- a state ledger, situation report, and after-action review.

After the three-person version works, add players in stages using the command
ladder. Measure referee time, player planning time, correction requests, missed
deadlines, and how many players made meaningful decisions. Add automation where
it removes repeated clerical work; simplify rules that create more exceptions
than useful choices.

### AI-versus-AI test runs

The server should support test campaigns where one AI controls the human-side
force and another controls the enemy. They receive only their side's permitted
information, use the same order interface as human players, and commit through
the normal turn phases. The referee can watch live, pause at any turn, or run
many seeded campaigns unattended and inspect them afterward.

Record the scenario and mechanics versions, random seed, AI model and
configuration, each side's information snapshot, orders, resolutions, and
state changes. This makes runs replayable and allows fair comparisons between
rules modules or AI policies. Track more than win/loss: objective progress,
casualties, turn length, stalemates, order legality, resource use, and whether
one side exploits a rule loophole. These runs help find bugs and balance
problems; human playtests are still needed to judge clarity, agency, and fun.

## Decisions to make next

1. What are the city, surrounding infrastructure, beachhead, and extraction
   routes on the first hex map?
2. How do civilians move, shelter, panic, and consume transport capacity?
3. What hex size and internal time slices make a one-hour turn tactically
   readable?
4. Which weapons, ammunition types, class stats, and resupply rules belong in
   the starter roster?
5. Which supported JSON rule operations are enough for the starter module?
   What would require a separately sandboxed extension?
6. Which Jev findings should block progress, and which can be informational?
7. Which decisions can Jev resolve from module rules, and which always need
   human review?
8. What information is private to a player, a group, the referee, or everyone?
9. How should room IDs be distributed and membership approval shown to players?
10. Which visual styles should Briefs support, and should they be generated per
    combat, per turn, or only when the referee requests one?

## Terms

- **Campaign server:** the hosted service and shared record for one game.
- **Group:** players who coordinate around a command, formation, sector, or
  staff function.
- **Commit:** lock in an order for the current turn, subject to the deadline.
- **Canonical state:** the referee's authoritative game state, including
  information hidden from players.
- **Jev:** working name for the referee assistant bot.
- **Brief:** optional AI-generated image illustrating a referee-approved event.
- **Adjudication:** applying the rules and state to decide what actions achieve.
