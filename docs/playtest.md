# Playtesting for Milestone 1

Milestone 1 is "dependable small-battle adjudication": the automatic referee
makes coherent, inspectable rulings over repeated turns in a small scenario,
and people who played agree the rulings were fair. Automated tests already
cover the mechanics. What is missing is people playing and saying where the
judge was wrong. This page is how to get that.

Use **Crossroads Rearguard**: two players, six one-minute turns, objectives
the server checks itself.

## Session 1: alone, about 20 minutes

The point is to learn the screens before inviting anyone.

1. Start the server with an AI model configured (`ai` and `ai-model` in
   `async-wargame.conf`; see the README). Without one you can still play, but
   you judge every turn yourself and learn nothing about the AI referee.
2. Create a Crossroads room. On the Table, work down the checklist: **Set up
   test players**, **Let the AI referee run turns**, **Start the operation**.
3. Open each tester link in its own tab. Each one first shows the briefing;
   press **Join the operation**.
4. In each player tab, write an order and commit it. In the referee tab,
   press **Lock turn** rather than waiting for the deadline.
5. Watch the Desk while the AI works through its four stages. When the report
   appears, read it in a player tab, then open **Order results and rolls**.
6. Play to the end, or at least through turn 4.

Deliberately try each of these once:

| Try | What should happen |
| --- | --- |
| Leave one unit without an order | It holds and defends; the report says so. |
| Order a move far beyond reach | The unit gets part of the way and the report says where it stopped. |
| Give both units contradictory orders ("swap places" and "hold together") | One coherent result; nobody acts twice. |
| Order the scouts into the wood to look for enemies | A discovery, if any, goes to the scouts; the hidden band is not named to everyone. |
| Order something unsupported ("build a barricade") | Reported as not possible or deferred, not silently granted. |
| Press **Pause AI · take over** mid-turn | The turn stays unpublished and you can judge it by hand. |
| Stop and restart the server mid-turn | The turn resumes; dice already rolled are unchanged. |
| **Reopen** a published turn | Positions and strengths return to before that turn. |

## Session 2: two or three people, over a day or two

1. Create a fresh Crossroads room. Share the room code or an invite link from
   People, approve each person, and give each a detachment.
2. Set the turn clock on Setup to something people can keep (one or two locks
   a day), switch on the AI referee, and start.
3. Stay out of the rulings. Only use Pause or Reopen if the game is stuck.
4. Ask players to post in Game Chat whenever a result seems wrong, at the
   time, with the turn number.

## What to write down

One line per disputed ruling is enough:

| Turn | Order (short) | What the judge ruled | What you expected, and why | Kind |
| --- | --- | --- | --- | --- |

Use one of these for **Kind**, since each has a different fix:

- **State**: the report and the map disagree (prose says they fell back; the
  counter didn't move).
- **Rules**: dice or reach were ignored or misapplied.
- **Judgment**: legal, but tactically unreasonable (casualties far too high
  for the margin, an enemy that knew what it couldn't have seen).
- **Privacy**: a player was told something they shouldn't know.
- **Usability**: a player didn't know what to do next, or did the wrong thing
  because the screen misled them.

Also note, per session: how long each turn took from lock to report, whether
any stage stopped and needed **Retry adjudication**, and the model used.

## When Milestone 1 is done

From the roadmap, restated as things you can tick off:

- [ ] Two complete Crossroads games with a real model and real players, one of
      them including a server restart mid-turn and one a provider failure.
- [ ] Every order in those games got exactly one result.
- [ ] No **State** or **Privacy** entries left unfixed.
- [ ] Each **Rules** and **Judgment** entry is either fixed, turned into a
      regression test, or written down as accepted behaviour.
- [ ] A new player completed a turn without being told how.
