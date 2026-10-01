# Writing a module

A **module** is a ruleset written as data: one JSON file. It holds terrain,
sides, unit classes, orders, combat, sight, and events. The campaign server
reads it but never changes it. A **scenario** is a second JSON file: a map,
places, starting units, and a briefing for one module.

The server knows a fixed vocabulary of operations and conditions, listed below.
The module supplies the numbers and the words. You can write a new game
without touching the server code. `internal/game/module_test.go` contains a
small Civil War skirmish module that runs through the same code as Dark Coast
Rescue.

## Loading your own

```sh
go run . -modules ./my-mods
```

`./my-mods/modules/*.json` and `./my-mods/scenarios/*.json` load alongside the
built-in ones. Every file is validated at startup; an error names the file and
the problem. A module or scenario can't reuse a built-in id.

## Module sections

| Section | What it is |
|---|---|
| `id`, `version`, `name` | Campaigns lock to the version they started with. |
| `scale` | `hex` and `turn` (text), and `slices`: how many steps a turn's movement is resolved in. |
| `sides` | `playable` sides are the players'; `enemy` sides are run by the referee or the enemy AI; anything else is neutral (civilians). |
| `terrain` | One-character `code`, `move` cost (0 = impassable on land), `cover`, and `stack` (stacking points the hex holds). |
| `edges`, `crossings` | Features along hex sides (rivers, ridges, walls). `blocks: true` means ground units cross only at one of the edge's `crossings`. |
| `classes` | Unit types: `side`, `domain` (`land` or `water`), `movement`, `attack`, `range`, `armor`, `stack`, `capacity`, `ammo`, `traits`, `kind` (`group` for crowds and hordes, otherwise a single unit), `symbol` (counter art), `contact` (how the other side sees it), `embarks` (can ride a transport), and `ai`. A playable class with a `capacity` is a transport: it carries that many people. |
| `traits` | Names and rules text, shown on unit cards and the rules page. |
| `orders` | `tasks` (set `destination`, `careful`, `resupply`, `embark`, `disembark`), `risks` (set `stopsShort` and one `default`), and `fallbacks`. A task without `destination`, `embark` or `disembark` can follow a move as its second step. |
| `stacking` | `default` capacity for terrain without its own. |
| `sight` | `base` range, `elevated` range on `elevatedTerrain`, `water` range for water units, `cover` limits (`{cover, range}`), and `traits` limits (e.g. `{"ambush": 1}`). |
| `combat` | See below. |
| `experience` | `levels`: `{name, min, dice}`. `min` is operations survived. |
| `evacuation` | Neutral units on `terrain` next to a `carrierClass` unit leave the map, up to its `capacity` a turn, and count toward the score. |
| `supply` | A `resupply` task refills ammunition at or next to a feature of a kind in `sources`, or alongside a class in `carriers`. |
| `escort` | Neutral units in the same hex as a unit with one of these `tasks` move with it. |
| `comms` | The command net: `nodes` (`{feature, range, manned}` or `{class, range}`) and `relay`, the range unit to unit. Leave it out and every unit is always in contact. |
| `events` | The random event table (see below). |

## Combat

Each side's dice pool is one die per point of `unitDice` (a class stat:
`attack`, `armor`, `movement`, or `range`) for every unit in the fight. Then:

- `groups`: `+dice` per `per` strength in a group unit, up to `max`. Only units
  that have dice of their own get it.
- each unit's experience dice
- `modifiers`, each with a `when` from this list:

| `when` | Applies when | Fields |
|---|---|---|
| `holds_cover` | the side held its ground | `perCover`: dice per point of cover in its hex |
| `across_barrier` | the side moved and is fighting across a blocking hex side | `dice` |
| `unseen` | the side was hidden from the players when the fight began | `dice` |
| `missed_orders` | a unit had no committed order | `stat`: use this stat instead |
| `out_of_ammo` | a unit has no ammunition of any kind left | `halve: true` |
| `trait:<id>` | a unit has that trait | `dice` |

Both pools are rolled. Each die showing `success` or better is a hit. Hits
cancel, and the margin is read on `outcomes`: `{min, max, result, loser,
winner}`, with `max: 0` meaning "or more". `loser` and `winner` effects are
`casualties` (percent), `retreat` (hexes), and `suppressed`. A group loses
people; a single unit is lost only at 100%. `infection` turns a share of
`victims` lost into strength for the winning `side`. `ammo` is spent by every
fighting unit, of each kind it carries.

The referee sees every factor, can change the pools, roll on the server, or
enter dice rolled at the table. The record keeps the suggested pools, the
final ones, and the reason.

## Enemy AI

Give an enemy class an `ai` section:

| `behavior` | What it does |
|---|---|
| `hunt` | Heads for the nearest prey within `sense` hexes along the cheapest path, stopping beside it. `prefers: "isolated"` goes for lone playable units. |
| `ambush` | Waits until prey comes within `sense`. |
| `call` | Stands still and draws units with `answersCall` within `callRange` toward its prey. |
| `hold` | Never moves. |

`wanders: true` shuffles a unit about when it senses nothing.

## Events

Each event has a `weight`, optional `minThreat` and `maxThreat`, `requires`
and `unique` (classes), text for the referee and a `report` for players
(`{dir}`, `{n}`, and `{n:noun}` placeholders), and `effects` built from:

| `op` | Does |
|---|---|
| `spawn` | Places `count` units of `class` (`strength`, `scale`d by threat, `known` to players or hidden) at hexes chosen by `where`: `edge`, `terrain`, `nearFeature`, `nearClass`, `awayFromMarines`. |
| `grow` | Raises `count` units of `classes` by `percent`. |
| `drift` | Moves `count` units of `classes` up to `steps` hexes `toward` `players` or a feature kind. |
| `reveal` | Makes `count` hidden units of `side` known, optionally `within` hexes of players. |
| `threat` | Changes the threat level by `amount`. |
| `jam` | Knocks out the radios of `count` friendly units for `turns` turns. |

## Limits today

- One playable side plays against an enemy side run by the referee or the
  AI. A two-sided game between human players (Union and Confederate, each with
  their own fog of war) needs per-side visibility, which is the next step for
  a Civil War module.
- Sight is a range shortened by cover. There's no line-of-sight tracing.
- `evacuation`, `supply`, and `escort` are the server's built-in versions of
  those ideas. A module can switch them off by leaving the sections out, but
  can't define new kinds of logistics yet.
