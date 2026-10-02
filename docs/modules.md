# Writing a module

A **module** is a setting written as data: one JSON file with its sides,
skills, unit templates, map art, random events, and the rules text players
read. The server never interprets the setting beyond that: the referee
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
| `id`, `version`, `name`, `intro` | Campaigns lock to the version they started with. `intro` is a line under the rules page title. |
| `scale` | What a `hex` and a `turn` mean in the fiction, in words. |
| `rules` | The rules page: a list of `{title, text: [paragraphs]}`. Write it in the setting's voice. |
| `skills` | `{id, name, note}`. What units can be good at, in percent. Keep the list short: eight or nine is plenty. |
| `advantages` | `{name, value, note}`. Suggested modifiers the referee might grant (`value` in percent, negative for a handicap). They're shown on the rules page and offered at the desk; the referee can type any number. |
| `sides` | `playable` sides are the players'; `enemy` sides are the referee's opposition; anything else is neutral (civilians). |
| `classes` | Unit templates: `side`, `name`, `abbr`, `kind` (`group` for crowds, where strength is a headcount, or a single `soldier`, `special`, `monster`, ...), `symbol` (counter art), `strength` (the usual headcount), `skills` (`{skill id: percent}`), `description`, `kit` (one sentence), and `contact` (`{abbr, label}`: how players see it before it's identified). |
| `terrain` | Map art: one-character `code`, `id`, `name`, `water: true` for water, and an optional `note`. |
| `edges`, `crossings` | Features drawn along hex sides (rivers, ridges, walls) and what can cross them. `blocks: true` makes random events route around an edge. |
| `events` | The random event table (below). |

Units in play can differ from their template: the referee changes skills,
kit, status, and notes on each unit as the story goes, and survivors keep
those changes into the next operation.

## The dice

Built in, and the same for every module: each side's chance is skill plus
advantage, kept between 5% and 95%. Each rolls d100 and succeeds at or under
its chance; the margin is how far under. A tenth of the chance or less is a
critical; 96 or more is a fumble. If one side succeeds it wins; if both do,
the bigger margin wins (a critical beats a plain success); if both fail,
nobody gets what they wanted. A check is one side rolling alone. The dice
say who did better and by how much; the referee says what it means.

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

Rolled events are proposals: nothing changes until the referee applies one.

## Scenarios

`map` (`cols`, `rows` of terrain codes, `edges`, `roads`, `sectors`),
`features` (places: `kind` is one of beachhead, hospital, port, utility,
police, shelter, relay, hive; `hidden` ones only the referee sees), `units`
(`class`, position, optional `strength` and `known`), `briefing` paragraphs,
and `refereeNotes`. Hex sides are written from the hex that owns them
(directions 0 east, 4 south-west, 5 south-east); the loader says which side
to flip if one isn't.
