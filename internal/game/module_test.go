package game

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"asyncwargame/content"
)

// A small Civil War skirmish ruleset, written only as data. If the server
// can run a turn of it, rules really are separate from the server.
const civilWarModule = `{
  "id": "acw-skirmish", "version": "0.1.0", "name": "Civil War Skirmish", "engineApi": "0",
  "scale": {"hex": "200 yards", "turn": "20 minutes", "slices": 4},
  "sides": [
    {"id": "union", "name": "Union", "playable": true},
    {"id": "confederate", "name": "Confederate", "enemy": true}
  ],
  "terrain": [
    {"code": ".", "id": "field", "name": "Open field", "move": 1, "cover": 0, "stack": 4},
    {"code": "w", "id": "woods", "name": "Woods", "move": 2, "cover": 1, "stack": 3},
    {"code": "h", "id": "ridge", "name": "High ground", "move": 2, "cover": 1, "stack": 3}
  ],
  "edges": [{"id": "creek", "name": "Creek", "blocks": false, "cost": 1, "crossings": ["ford"]}],
  "crossings": [{"id": "ford", "name": "Ford", "cost": 0}],
  "traits": [{"id": "volley", "name": "Volley fire", "text": "Fires as a block."}],
  "classes": [
    {"id": "brigade", "name": "Infantry brigade", "abbr": "INF", "side": "union", "domain": "land", "movement": 3, "attack": 4, "range": 2, "armor": 3, "stack": 2, "symbol": "infantry", "traits": ["volley"], "ammo": {"cartridges": 4}},
    {"id": "cavalry", "name": "Cavalry regiment", "abbr": "CAV", "side": "union", "domain": "land", "movement": 6, "attack": 2, "range": 1, "armor": 1, "stack": 1, "symbol": "mech"},
    {"id": "reb_brigade", "name": "Confederate brigade", "abbr": "INF", "side": "confederate", "domain": "land", "movement": 3, "attack": 4, "range": 2, "armor": 3, "symbol": "infantry",
     "ai": {"sense": 6, "behavior": "hunt"}}
  ],
  "orders": {
    "tasks": [{"id": "advance", "name": "Advance", "destination": true}, {"id": "hold", "name": "Hold the line"}],
    "risks": [{"id": "steady", "name": "Steady", "default": true}],
    "fallbacks": [{"id": "rally", "name": "Rally on the colors"}]
  },
  "stacking": {"default": 4},
  "sight": {"base": 4},
  "combat": {
    "die": 10, "success": 6, "unitDice": "attack",
    "modifiers": [{"when": "holds_cover", "label": "Behind cover", "perCover": 2}, {"when": "trait:volley", "label": "Volley", "dice": 1}],
    "outcomes": [
      {"min": 0, "max": 1, "result": "The lines hold.", "loser": {}, "winner": {}},
      {"min": 2, "max": 0, "result": "The loser's line breaks and falls back.", "loser": {"casualties": 30, "retreat": 1}, "winner": {"casualties": 10}}
    ]
  },
  "experience": {"levels": [{"name": "Green", "min": 0, "dice": 0}, {"name": "Veteran", "min": 2, "dice": 1}]}
}`

const civilWarScenario = `{
  "id": "creek-crossing", "name": "Creek Crossing", "module": "acw-skirmish",
  "summary": "Hold the ford.", "briefing": ["Hold the ford."],
  "map": {"cols": 8, "rows": ["........", "...ww...", "...ww...", "..h.....", "........", "........"],
    "edges": [{"col": 4, "row": 3, "dir": 0, "type": "creek", "crossing": "ford"}]},
  "units": [
    {"id": "u1", "name": "Iron Brigade", "class": "brigade", "col": 1, "row": 3, "strength": 1800},
    {"id": "u2", "name": "1st Michigan Cavalry", "class": "cavalry", "col": 1, "row": 4, "strength": 400},
    {"id": "c1", "name": "Stonewall Brigade", "class": "reb_brigade", "col": 5, "row": 3, "strength": 1600}
  ]
}`

func TestAnotherRulesetRunsUnchanged(t *testing.T) {
	mods := fstest.MapFS{
		"modules/acw.json":     {Data: []byte(civilWarModule)},
		"scenarios/creek.json": {Data: []byte(civilWarScenario)},
	}
	cat, err := LoadCatalog(content.FS, mods)
	if err != nil {
		t.Fatal(err)
	}
	sc, mod := cat.Scenarios["creek-crossing"], cat.Modules["acw-skirmish"]
	r := newRoom("CWCWCW", "Gettysburg-ish", sc, mod, "Ref", HashToken("ref"))
	ref := r.Referee()
	r.Members = append(r.Members, &Member{ID: "p", Name: "Meade", Role: RolePlayer, Status: StatusApproved})
	r.Unit("u1").AssignedTo = "p"
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := r.StartOperation(ref, mod, now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SaveOrder(r.Member("p"), mod, OrderInput{Unit: "u1", Task: "advance", Dest: HexID(3, 3), Risk: "steady", Fallback: "rally", Commit: true}, now); err != nil {
		t.Fatal(err)
	}
	r.Tick(mod, r.Deadline)
	if r.Resolution == nil || len(r.Resolution.Encounters) == 0 {
		t.Fatalf("no fight between the brigades: %+v", r.Resolution)
	}
	e := r.Resolution.Encounters[0]
	if e.Suggested[FriendSide].Dice < 5 { // attack 4 + volley 1, before cover
		t.Errorf("Union pool %+v", e.Suggested[FriendSide])
	}
	publishTurn(t, r, mod, ref, r.Deadline)
	if r.Turn != 2 || len(r.Reports) != 1 {
		t.Errorf("turn %d, %d reports", r.Turn, len(r.Reports))
	}
}

func TestModsCantReplaceBuiltInRulesets(t *testing.T) {
	twin := strings.Replace(civilWarModule, `"id": "acw-skirmish"`, `"id": "dark-coast-tactical"`, 1)
	dup := fstest.MapFS{"modules/dup.json": {Data: []byte(twin)}}
	if _, err := LoadCatalog(content.FS, dup); err == nil || !strings.Contains(err.Error(), "already loaded") {
		t.Errorf("a valid mod with a built-in id: %v", err)
	}
}
