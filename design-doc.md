# Asynchronous Wargame

**Concept:** friends play a referee-led game through the browser, a turn or
two a day. Players say what their characters do in their own words; the
referee decides what happens. The server keeps the map, the orders, the dice,
and the record. It doesn't keep rules, because there are hardly any.

**The model:** an old-school, Braunstein-style game. The referee is the
rules. Dice only tell the referee how fortune fell, and players help build
the world they play in.

## Principles

- **Say what you do.** Orders are free text: "Okafor crosses the bridge low
  and fast, stacks up on the side door, and if the shamblers turn, tosses a
  grenade and pulls back." There's no list of actions and no stat for every
  piece of gear.
- **The referee decides.** Anything a character could plausibly attempt, they
  can attempt. The referee says what it takes and what happens.
- **One roll, read by a person.** When it's in doubt, each side rolls a d10
  and adds one bonus summing everything that matters. The totals ("Okafor 5,
  the horde 9") inform the referee's call; they don't make it.
- **Build the world together.** Players propose facts about their characters
  and the setting; the referee accepts, changes, or refuses them. Accepted
  facts are true from then on.
- **Asynchronous by design.** Orders lock on a schedule people with jobs can
  keep. Nobody needs to be online at the same time. Notifications say when
  you're needed.
- **Hidden stays hidden.** What a player may see is filtered on the server.
  Nothing secret reaches a browser that shouldn't have it.
- **The AI helps the referee, never replaces them.** It can suggest rolls and
  draft prose. The referee rolls, keeps, or rewrites.

## A turn

1. **Orders.** Each player writes what their character does, optionally
   dropping a pin on the map. A squad leader can approve a player's order or
   send it back with a note before the lock.
2. **Lock.** At the scheduled time, or earlier if the referee locks it,
   orders freeze. A character without an order carries on as before.
3. **Judge.** The referee reads every order. Where an outcome is in doubt,
   they roll. They write what happened, choose who hears about it, and move
   counters and change characters as they go. Players keep seeing the board
   as it stood at the lock.
4. **Publish.** Players get the report. The board catches up, and the next
   turn opens. Until the next lock, the referee can reopen a published turn
   to correct it, with a reason players see.

## Resolution

Each side rolls a d10 and adds one bonus: the unit's quality (0 ordinary, +2
trained, more for the exceptional), +1 per operation survived (up to +2),
and whatever the situation adds or takes away (cover, surprise, numbers,
night, wounds, the right tool). A side can roll alone against the situation.

The result is just the totals. A big gap is decisive; a small one is close
and costs something; a low total rolled alone means the situation won.
That's guidance, not a table: the referee weighs the totals against what
each side was trying to do. The server rolls, or the referee enters dice
rolled at the table.

## People

- **Referee:** runs the game, sees everything, judges every turn. By default
  they're blind to players' orders until the lock, so the opposition is run
  without bias. An admin view shows everything, and players are told when it's
  on.
- **Squad leader** (commander): plays a character, reviews the orders of the
  players under them, and can hand out characters in their command.
- **Players:** each plays one character (or a small unit), writes orders,
  proposes facts, and talks to the others.

Players join with a room code (the referee approves them) or an invite link.
Messages run in channels: everyone, players only, a command, and privately
with the referee.

## The table

Players have one page: what's happened (the latest report, the briefing,
earlier turns), the map, their character (with a private brief), the order
box, and talk. The referee has four screens: Table, Desk, People, and Setup.

The map is the drawn hex map or an uploaded image, with a hex grid, square
grid, or no grid over it. Counters and pins go anywhere; the grid is for
saying where and how far. Every map states its scale and its turn ("1 hex =
50 m · 1 turn = 10 minutes") so orders mean the same thing to everyone.

## Information

- **Private briefs.** Each character can carry a brief only its player and
  the referee read: a secret, an order from above, a personal stake.
- **Fog of war is the referee's call.** They reveal an enemy as a contact,
  to everyone or only to chosen players, and identify it when the
  characters get a good look.
- **Radio contact.** A character whose radio is out keeps their orders to
  themselves. The rest of the squad sees them where they last called in.
- **Reports** show each order's outcome to whoever the referee chose: the
  whole squad, only that player (and their squad leader), or nobody yet.
  Secret rolls stay on the referee's side.

## The assistant (Jev)

An optional AI model, run by the server (a hosted one, or one on the same
machine). It can:

- suggest rolls from the locked orders: who rolls, and what counts for each
  side, with reasons;
- draft what happened from an order and its rolls;
- draft a short opening narrative for each report, from only what players
  were told.

Every suggestion is labeled with the model that made it. Nothing takes
effect until the referee rolls it, keeps it, or approves it. The model sees
game data, never messages or anyone's name.

## Settings and scenarios

A **module** is a setting as data: sides, unit templates (quality, kit, a
description), suggested roll factors, map art, and random events for the
referee to apply or discard. A **scenario** is a map, places, starting units,
a briefing, and the referee's notes. New settings need no code changes (see
`docs/modules.md`).

The starter is **Dark Coast**, with the scenario **St. Ines**: four colonial
marines (a squad leader and three marines) land at Red Beach, cross the
canal, reach the hospital, and bring the survivors out.

## Hosting

One small always-on server, running a single binary that keeps every campaign
in one file. HTTPS comes through a reverse proxy or Tailscale. Web push
notifications are optional, and so is the AI model. See `deploy/`.

## What's next

See `docs/roadmap.md`: email, instancing
(linked battles on several maps in one campaign), two-sided play, and more.

## Terms

- **Referee:** the person who runs the game and decides what happens.
- **Order:** what a player says their character does this turn.
- **Roll:** a d10 plus one bonus, per side.
- **Fact:** something established about the world, proposed by anyone and
  accepted by the referee.
- **Desk:** the referee's screen for judging a locked turn.
- **Brief:** what one character knows that the others don't.
- **Scale:** what one grid cell and one turn are in the world.
- **Jev:** the referee's AI assistant.
