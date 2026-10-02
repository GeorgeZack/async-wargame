# Writing a module

A **module** is a setting written as data: one JSON file with its sides,
unit templates, suggested roll factors, map art, and random events. The server never interprets the setting beyond that: the referee
does. A **scenario** is a second JSON file: a map, places, starting units,
and a briefing for one module.

`content/modules/dark-coast.json` and `content/scenarios/st-ines.json` are
the built-in examples.

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
| `id`, `version`, `name`, `intro` | Campaigns record their starting version; rooms with the same module ID load using the installed module, even after a version bump. `intro` is a line describing the setting. |
| `factors` | `{name, value}`. Things that often count toward a roll's bonus ("Ambush or surprise" +3, "Night" -1). They're offered at the desk and to the AI; the referee can type anything. |
| `sides` | `playable` sides are the players'; `enemy` sides are the referee's opposition; anything else is neutral (civilians). |
| `classes` | Unit templates: `side`, `name`, `abbr`, `kind` (`group` for crowds, where strength is a headcount, or a single `soldier`, `special`, `monster`, ...), `symbol` (counter art), `strength` (the usual headcount), `quality` (a bonus to its rolls: 0 ordinary, +2 trained, more for the exceptional), `description`, `kit` (one sentence), optional `speed` and `run` (km/h at a careful pace and a run), and `contact` (`{abbr, label}`: how players see it before it's identified). |
| `terrain` | Map art: one-character `code`, `id`, `name`, `water: true` for water, and an optional `note`. |
| `edges`, `crossings` | Features drawn along hex sides (rivers, ridges, walls) and what can cross them. `blocks: true` makes random events route around an edge. |
| `events` | The random event table (below). |

Units in play can differ from their template: the referee changes quality,
kit, status, and notes on each unit as the story goes, and survivors keep
those changes into the next operation.

## The dice

Built in, and the same for every module: when a situation is in doubt, each
side rolls a d10 and adds one bonus, everything that matters summed into a
single number (a unit's quality, +1 per operation survived up to +2, the
situation, surprise, the weather). The result is just the two totals:
"Okafor 5, the horde 9". There's no table: the referee compares them with
what each side was trying to do and decides. A side can roll alone, too.

## Events

Each event has a `weight`, optional `minThreat` and `maxThreat`, `requires`
and `unique` (classes), text for the referee and a `report` for players
(`{dir}`, `{n}`, and `{n:noun}` placeholders), and `effects` built from:

| `op` | Does |
|---|---|
| `spawn` | Places `count` units of `class` (`strength`, `scale`d by threat, `known` to players or hidden) at spots chosen by `where`: `edge`, `terrain`, `nearFeature`, `nearClass`, `awayFromMarines`. |
| `grow` | Raises `count` units of `classes` by `percent`. |
| `drift` | Moves `count` units of `classes` up to `steps` grid cells `toward` `players` or a feature kind. |
| `reveal` | Makes `count` hidden units of `side` known, optionally `within` grid cells of players. |
| `threat` | Changes the threat level by `amount`. |
| `jam` | Knocks out the radios of `count` friendly units for `turns` turns. |

Rolled events are proposals: nothing changes until the referee applies one.

## Scenarios

`board`, `map` (`cols`, `rows` of terrain codes, `edges`, `roads`),
`features` (places: `kind` is one of beachhead, hospital, port, utility,
police, shelter, relay, hive; `hidden` ones only the referee sees), `units`
(`class`, position, optional `strength`, `known`, and `brief`), `briefing`
paragraphs, an optional `objectives` array of goal strings (displayed below the
briefing), and `refereeNotes`.

`board` says what the map is and what it measures:

```json
"board": {
  "background": "hex",
  "grid": "hex",
  "distance": {"amount": 50, "unit": "m"},
  "turn": {"amount": 10, "unit": "minutes"}
}
```

`background` is `hex` (the drawn map below) or `image` (the referee uploads
one in the room). `grid` is `hex`, `square`, or `none`, laid over either;
`cell` is its size in board units (one drawn hex is 31.2). Distance units
are ft, yd, m, km, mi; turn units are seconds, minutes, hours, days, weeks.
Players see it as "1 hex = 50 m · 1 turn = 10 minutes", with a scale bar.

Units and places sit anywhere: give a hex as `col` and `row` (zero-based;
they're put at its center) or an exact point as `x` and `y`. A unit's
`brief` is private to whoever plays it (and the referee): secrets, orders
from above, what they know that the others don't. Hex sides are written from the hex that owns them
(directions 0 east, 4 south-west, 5 south-east); the loader says which side
to flip if one isn't.

## Formations and movement

A scenario can give several characters one map counter:

```json
"formations": [{"id": "lantern", "name": "Lantern Squad", "abbr": "SQD"}]
```

Set `"formation": "lantern"` on each member unit. Attached members start at
its first member's position, move together, and retain individual orders,
players, cards, and private briefs. The referee can split and rejoin them.
Formation IDs must be unique; unit IDs cannot reuse `f-` plus a formation ID.

Class `speed` and `run` values combine with the board's distance scale and
turn duration to describe how far a unit can reach. If `run` is omitted, it
is three times `speed`. These are guidance for the referee and AI, rather
than enforced movement limits. Without a scale or speed, no reach is given.
