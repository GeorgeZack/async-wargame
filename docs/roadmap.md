# Roadmap

What's next, roughly in order. Playtesting comes first and will reorder
everything after it.

## Soon

- **Email notifications.** Web push works on devices people have turned it
  on for; email reaches everyone else (turn open, orders due, results in).
- **Enemy control modes.** The referee writes the opposition's intentions
  as orders on the desk, or asks the assistant to propose them from what the
  infected know, and judges both sides together.
- **Assistant: interpreting orders.** Ask the model what an unclear order
  most likely means, and draft a question back to the player.

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

## Later

- **Two-sided play** with separate fog for each side (a Civil War module
  needs it).
- **AI-versus-AI test runs** through the normal order flow, for balance.
- **Turn history browser:** what each side knew at any turn.
- **Briefs:** AI images of resolved events, approved by the referee.
- **Play-by-post view** over the same record.
- **Playtest metrics:** referee time per turn, missed deadlines,
  corrections.
- A database, once one JSON file stops being enough.
