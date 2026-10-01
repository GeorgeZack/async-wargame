# Async Wargame

Browser-based, referee-led asynchronous wargames. See [design-doc.md](design-doc.md).

## Run

```sh
mise install               # installs the pinned Go toolchain (see mise.toml)
go run .                   # http://127.0.0.1:8080
go run . -addr :8080       # accept connections from other machines on the LAN
go test ./...
```

State is kept in `data/campaigns.json` (`-data` to change).

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

## What works so far

- **Rooms:** a referee hosts a campaign and gets a six-character room code
  (`ABC-DEF`; no 0/O/1/I). Players enter the code and a display name. They wait
  as *pending* until the referee approves or declines them, then read a briefing.
  Membership is a per-room HttpOnly cookie holding a random token; only its
  hash is stored.
- **Dashboard:** next-action banner, access requests, members, unit assignment
  (referee), your force (player), known forces, activity log, referee notes.
- **Map:** server-rendered SVG hex map (pointy-top, odd-row offset, hex IDs
  `CCRR`), with pan, zoom, pinch, keyboard cursor, a hex inspector, and a terrain
  key and place list as text fallbacks.
- **Hidden state:** `game.BuildView` is the only route from canonical state to a
  browser. Players never receive unknown units or hidden places. They see
  detected enemies only as unidentified contacts with estimated strength.
- **Live updates:** open pages subscribe to `/r/{code}/live` (SSE). The
  stream carries only a version number. The page then refetches its own
  filtered HTML and swaps the `data-live` regions.
- **Progressive:** every action is a plain HTML form; script only enhances.
- **Infected and random events:** the module defines shambler hordes, runners,
  crawlers, bloaters, screamers, brutes, stalkers, brood nests, and the Brood
  Mother, each with traits. A weighted event table covers stragglers, growth,
  noise drift, waves, brood hatches, new nests, monsters, and respites. Each
  table entry is built from five declarative ops (`spawn`, `grow`, `drift`,
  `reveal`, `threat`). The referee rolls, or picks, an event. The server
  records a concrete proposal with its seed, outlines the affected hexes on the
  referee's map, and changes nothing until the referee applies it. Players get
  only the event's sitrep text. The threat level (1–5) scales spawn sizes and
  unlocks bigger events.

- **Map model:** each hex has one terrain. Rivers, streams, ridges, cliffs,
  and walls run along hex *sides*, with bridges, fords, passes, and gates as
  crossings. Roads join hex centers. The edge and crossing types are module
  data. Each room keeps its own copy of the map, so editing never touches the
  scenario template. Infected drift routes around barriers to crossings.
- **Scenario editor** (`/r/{code}/edit`, referee only): paint terrain, draw
  edges and roads by dragging, place crossings, add/move/edit/delete units and
  places, edit the briefing and referee notes, and read the random event
  tables with each event's odds at every threat level. Each gesture is one
  atomic batch of ops. During setup, players see a holding page. **Start the
  operation** releases the map and opens turn 1. **Reset to template**
  restores the scenario.
- **Seats:** every room URL also works as `/r/{code}/k/{key}/…`, where the key
  identifies the member instead of the cookie. "Separate seat for this tab"
  on the join form, and the referee's **test seats**, let one browser act as
  several players. Every member also gets a private access link for another
  device.
- **Look:** a printed mapboard (ink-drawn terrain, hex numbers, paper grain)
  and die-cut cardboard counters with NATO-style symbols and
  attack-armor-move factors. Unidentified contacts show face down.

- **Chain of command:** the referee promotes players to **commander**, and
  sets who each player reports to. A commander can hand out the units in
  their command (their own units plus their players') among themselves and
  their players. They can't take units the referee holds, or give units
  outside their command.
- **Invites:** the referee creates invite links for commanders or players.
  A player invite can name a commander. Links are 1, 5, or 20 uses, expire
  after 7 days, and can be revoked. Accepting an invite counts as the
  referee's approval. Joining by room code still goes through the approval
  queue.
- **Turn clock:** the referee sets a time zone, daily lock times (e.g. 6 PM
  and midnight Central), and lock days. Each turn locks at the next slot
  that leaves at least an hour to plan. The referee can defer a lock (by
  1/3/12/24 hours, to the next slot, or to an exact time) or lock now.
  Deadlines are shown in the room's zone and in each viewer's own zone. A
  background clock in the server checks every 30 seconds.
- **Enemy AI** (rules-based, no model): at lock it plans one turn of
  infected movement. Each class has a doctrine from its traits: hordes
  chase noise and people, runners pursue, stalkers hunt isolated squads,
  crawlers wait in ambush, screamers call hordes, and nests and the Brood
  Mother hold. Moves spend real movement points on terrain, edge, and
  crossing costs, along cheapest paths that respect rivers and ridges.
  Units ending near marines are spotted, and units ending beside people are
  flagged for the referee as encounters. The plan is a normal proposal. If
  the referee hasn't opened the next turn by the takeover time (never,
  immediately, or 1–48 hours after lock), the AI applies the plan and opens
  the next turn itself.

- **Orders:** a player picks one of their units ("Give order", from the
  orders panel or the hex inspector). They choose a task from the module
  (move, clear and attack, recon, escort, evacuate, resupply, hold, dig in)
  and, for tasks that need one, click a destination hex. The hexes the unit
  can reach this turn are highlighted with their movement cost. Reach is the
  unit's Movement stat spent on terrain, edge, and crossing costs. Dropships
  stay on water. Hexes holding visible enemies are off limits. The route is
  the cheapest path, drawn as an arrow. Each order also records intent,
  acceptable risk, and a fallback. Orders save as drafts or commit, and keep
  a version history. Commanders can write orders for their players' units,
  and the referee for any friendly unit. Committed orders are visible to all
  friendly players; drafts only to their author's chain. At lock, committed
  orders freeze and drafts lapse. Units without orders hold and take no
  offensive action. A "friendly movement" plan moves units along their
  routes, stopping where they run into the infected (seen or not), and
  flags the encounter for the referee. Without JavaScript, the destination
  can be typed as a hex number.
- **Fog of war:** players share what any friendly unit can see. Sight is 3
  hexes, 4 from hills or for dropships. Cover shortens it: 2 in
  urban/industrial/woods, 1 in dense urban, and ambushers are seen only when
  adjacent. Unseen hexes are fogged. Contacts that leave sight become "last
  seen, turn N" markers. Newly sighted contacts and found survivors produce
  sitreps by sector. Contact ids are opaque to players. The referee can
  preview the players' fog.
- **Sectors:** a lettered and numbered grid (A1, B3, …; 4×3 hexes by
  default) drawn along hex sides, which the referee can resize and name
  ("C3 · Hospital"). Sectors appear in the inspector, order destinations,
  and sitreps, so commanders share a vocabulary.
- **Closing a room:** the referee can close a room (play, the clock, joins
  and invites stop; members see that it ended), reopen it, or delete it
  permanently by typing the room code.

- **Stacking:** friendly units use stacking points (squads 1–2, the
  transport 3, dropships 4). Each terrain holds so many: open/urban/beach 6;
  dense, woods, and hills 4; marsh 3; open water 8. Civilians and the
  infected don't count. Units may pass through a full hex but not end in
  one. Orders are checked against everyone's planned positions. Resolution
  backs a unit up along its route if its hex is still full. The editor
  refuses to overstack. A whole stack can be given one order.
- **Rules page** (`/r/{code}/rules`): generated from the campaign's locked
  module version, covering the turn, orders, movement and terrain, hex
  sides, stacking, sight, units, combat dice procedure, experience, and
  sectors. The infected bestiary and enemy doctrine are referee-only.
- **Unit cards** (`/r/{code}/units/{id}`): stats, supply, traits,
  experience and service record, commander, and this turn's order. They open
  in a dialog from any unit name. Players get contact cards (signature only)
  for enemies they can see, by contact id.
- **Campaigns and experience:** the referee ends an operation; every
  friendly unit still on the map survives it, gains an operation of
  experience (Green, Seasoned 1+, Veteran 2+, Elite 4+, worth +0–3 dice),
  and a service-record line. The next operation starts from the scenario
  template, with survivors in their start hexes and green replacements for
  lost units. Chevrons on counters show experience. The referee can also set
  it in the editor.

- **Blind referee:** during planning the referee doesn't see player orders,
  routes, or order log lines, only how many are in. They can run the infected
  without bias. Admin view shows everything, and switching it on during
  planning is announced to players. The referee can lock the turn as soon as
  every player has committed.
- **Turn resolution:** at lock, players' orders and the enemy AI's paths are
  walked together through the turn's time slices. Opposing units that come
  within a hex of each other stop and fight. The referee gets a resolution
  desk with every fight as an encounter card: units, situation, suggested dice
  pools with each factor, and controls to roll them, roll changed pools, or
  enter table dice, with a reason. Then they publish. If the referee is away,
  the rules settle the fights and publish.
- **Consequences:** fights cause casualties, falling back, suppression,
  destroyed units, ammunition spent, and civilians lost (some rise with the
  horde). Escort and evacuate orders bring civilians along. Civilians on the
  beach board dropships offshore, and the header keeps the evacuation score.
  Resupply refills ammunition at the beachhead or alongside transports.
- **Reports:** each published turn produces a report in two versions: players
  never see what the infected are, or fights they couldn't know about. Moves
  and fights in a report have a Replay link: a close-up of the board plays
  the move back, or for a fight the approach, the contact, both dice pools,
  and the result. Players' replays show the enemy only as contacts, appearing
  at contact. Nothing animates on the live map. Ending an operation records
  an after-action review (evacuated, lost, fights, casualties).
- **Transports:** units whose class `embarks` can board a friendly transport
  with an Embark order (from its hex on land, or beside it from the shore),
  ride along without counting toward stacking, and get off with a Disembark
  order. A transport that's destroyed sets its passengers down hurt, or loses
  them at sea.
- **Commander review:** a commander can approve their players' committed
  orders or return them with a note; a returned order goes back to draft.
- **Standing and two-step orders:** an order can name a task to take up on
  arrival ("move to the bridge, then dig in") and can be marked standing.
  Each new turn brings these back as drafts: an unfinished move continues
  from where the unit stands, an arrived one becomes its follow-on task.
  The player (or the referee, for units they hold) confirms them every
  round, one at a time or with "Confirm all". One nobody confirms still runs
  at the lock, but defensively only: the unit fights as a unit without
  orders would, and the order keeps coming back until someone deals with it.
- **Screens:** the room is an application with screens, each with its own
  URL. Referee: Map, Turn (a plan → lock → settle → publish procedure with the
  next action, the clock, and the resolution desk), Random events
  (threat, proposals, and the event odds table), Forces (a ledger with
  supply, experience, commander, and order status, assignable in place),
  People, Messages, Reports, Activity, and Room. Players: Map, Orders, Forces,
  Messages, Reports, Activity, People.
  The Editor and Rules sit at the end of the tab strip. Panels are windows
  with title strips, and a status bar along the bottom always shows the turn,
  a countdown to lock, orders in, the evacuation score, and whether live
  updates are connected. Links from other screens open the map on a hex
  (`?hex=0708`) or a unit's order form (`?order=m1`).
- **Communications:** if the module has comms rules, each friendly unit is
  on the command net (green light) or off it (red): within range of a node
  (the beachhead, a dropship, a manned relay) or relayed by other units'
  radios. Off the net, a unit's orders stay with the player holding it: their
  commander can't see or change them, and other players can't see them. The
  referee can mark a unit lost or force contact (Forces screen), switch the
  rules off for the room (Room screen), and a "Comms blackout" event jams
  radios. Lights show on counters, the orders list, the forces ledger, and
  unit cards. Only units on the net feed the shared picture: a unit off the
  net still sees, but only the player holding it gets that picture (marked
  on their map), and everyone else sees the unit where it last reported
  from, faded.
- **Corrections:** until the next turn locks, the referee can reopen the
  last published turn with a reason players see. The game goes back to just
  before publishing (fights settled as they were, ready to re-settle);
  members and messages are untouched, and orders already written for the new
  turn are set aside. The republished report is marked corrected.
- **Leaving:** a player can leave from the People screen, and the referee
  can remove anyone, choosing who gets their units: their commander, another
  player, or the referee. Units keep their orders and drafts, so the new
  holder can confirm or change them before the lock. Access ends at once.
- **Notifications:** web push, turned on per device from the People screen
  (the Room screen for the referee). Players hear when a turn opens or its
  results are in, two hours before the lock if their orders aren't in, when
  a commander returns an order, when units are handed to them, and when
  someone writes to them. The referee hears when every order is in, when a
  turn locks, and if the rules published a turn while they were away. Needs
  HTTPS (or localhost); on iPhone, add the site to the Home Screen first.
  The server keeps its push key in `vapid.pem` next to the `-data` file
  (`-push-key` to move it, `-no-push` to turn notifications off), and
  `AW_PUSH_CONTACT` is the address push services can reach you at.
- **Messages:** discussion channels on the Messages screen, with unread
  badges: All hands (everyone), Players only, one per commander's command,
  and a private channel between each member and the referee. A blind referee
  can't read the players' channels; admin view opens them read-only.
- **Rules are data:** `internal/rules` holds the module schema and the combat
  engine, and the server depends on it, never the reverse. Sight, AI
  doctrine, combat, stacking, evacuation, supply, escort, and order behavior
  all come from the module file. `-modules DIR` loads more modules and
  scenarios; see [docs/modules.md](docs/modules.md).

## Layout

```
main.go                       flags, wiring
content/                      embedded module + scenario JSON
internal/rules/               module schema and the rules engine (combat pools, sight)
internal/game/                campaigns: rooms, orders, resolution, fog, views, store
internal/web/                 HTTP handlers, templates, static JS/CSS
```
