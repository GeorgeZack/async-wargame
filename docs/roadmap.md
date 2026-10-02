# Roadmap

What's next, roughly in order. Playtesting comes first and will reorder
everything after it.

## First

- **Playtest St. Ines** with a referee and four players, and change what
  the table finds clumsy.

## Soon

- **Email notifications.** Web push works on devices people have turned it
  on for; email reaches everyone else (turn open, orders due, results in).

## Instancing

Several battles on different maps at once, inside one campaign.

- A campaign holds more than one **instance**: its own map, units, places,
  fog, and turn clock (or a shared clock).
- Instances are **linked**: a unit can leave one map from a marked exit and
  arrive on another after a delay the referee sets. Reinforcements reach the
  front after surviving an ambush on the road; a party splits between two
  connected dungeons; the dropship flies from the beach to the hospital
  roof.
- Each player sees the instances their units are in (and what their radios
  reach); the referee sees all of them and adjudicates each on its own desk.
- Reports, messages, and facts can belong to an instance or to the whole
  campaign.

## Two-sided play

Built alongside instancing, since they share the plumbing: each side has its
own players, orders, and fog, and the referee judges both on one desk. A
Civil War module needs it.

## Later

- **AI-versus-AI test runs** through the normal order flow: an AI on each
  side writing free-text orders, and the assistant judging, to see what
  stories come out.
- **Turn history browser:** what each side knew at any turn.
- Situation-brief vignettes are implemented with a separate image API/model
  and referee approval; choose an image provider/model before enabling them.
- **Play-by-post view** over the same record.
- **Playtest metrics:** referee time per turn, missed deadlines,
  corrections.
- A database, once one JSON file stops being enough.
