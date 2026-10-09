package game

import (
	"encoding/json"
	"strings"
	"testing"
)

func awarenessRoom(t *testing.T) (*Room, *Module, *Unit, *Unit) {
	t.Helper()
	r, mod, _, _, _ := orderRoom(t)
	a, b := r.Unit("z1"), r.Unit("m1")
	r.Units = []*Unit{a, b}
	r.Map = &HexMap{Cols: 6, Rows: []string{"......", "......", "......"}}
	r.Board.Background, r.Board.Cell = BackgroundHex, hexW
	a.X, a.Y = center(1, 1).X, center(1, 1).Y
	b.X, b.Y = center(2, 1).X, center(2, 1).Y
	return r, mod, a, b
}

func noticedBy(t *testing.T, r *Room, mod *Module, enemy string) ([]noticedUnit, string) {
	t.Helper()
	_, body, ok := r.intentionsPrompt(mod, &Run{Kind: RunIntentions, Turn: r.Turn})
	if !ok {
		t.Fatal("missing intention prompt")
	}
	var in struct {
		Enemies []struct {
			ID     string
			Senses []noticedUnit `json:"whatItCouldNotice"`
		}
	}
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatal(err)
	}
	for _, e := range in.Enemies {
		if e.ID == enemy {
			return e.Senses, body
		}
	}
	t.Fatal("missing enemy")
	return nil, body
}

func TestConcealedUnitAbsentFromEnemyPromptUntilRevealed(t *testing.T) {
	r, mod, enemy, player := awarenessRoom(t)
	r.Map.SetTerrain(2, 1, 'f')
	r.RefereeNotes = "SECRET: the scout is planning an ambush."
	player.Notes, player.Brief = "PRIVATE KIT", "PRIVATE BRIEF"
	contacts, body := noticedBy(t, r, mod, enemy.ID)
	if len(contacts) != 0 || strings.Contains(body, player.ID) || strings.Contains(body, "SECRET") || strings.Contains(body, "PRIVATE") {
		t.Fatal("concealed unit or private referee context reached the enemy")
	}
	yes := true
	if err := r.EditUnit(r.Referee(), mod, player.ID, UnitEdit{RevealedToEnemy: &yes}); err != nil {
		t.Fatal(err)
	}
	contacts, body = noticedBy(t, r, mod, enemy.ID)
	if len(contacts) != 1 || contacts[0].ID != player.ID || contacts[0].Numbers != "one contact" {
		t.Fatal("revealed contact is not present with estimated numbers")
	}
	var in map[string]json.RawMessage
	json.Unmarshal([]byte(body), &in)
	var enemies []map[string]json.RawMessage
	json.Unmarshal(in["enemies"], &enemies)
	var observations []map[string]json.RawMessage
	json.Unmarshal(enemies[0]["whatItCouldNotice"], &observations)
	if _, exact := observations[0]["count"]; exact {
		t.Fatal("enemy received an exact headcount")
	}
	if _, bonus := observations[0]["usualBonus"]; bonus {
		t.Fatal("enemy received private quality")
	}
	if strings.Contains(body, "PRIVATE") {
		t.Fatal("enemy received private unit notes")
	}
	no := false
	if err := r.EditUnit(r.Member("h"), mod, player.ID, UnitEdit{RevealedToEnemy: &no}); err != ErrForbidden {
		t.Fatal("player changed enemy knowledge")
	}
	card, err := BuildUnitCard(r, mod, r.Member("h"), player.ID)
	if err != nil || card.RevealedToEnemy {
		t.Fatal("player card exposes enemy knowledge")
	}
	card, err = BuildUnitCard(r, mod, r.Referee(), player.ID)
	if err != nil || !card.RevealedToEnemy {
		t.Fatal("referee cannot inspect revelation")
	}
	view, _ := json.Marshal(BuildView(r, testScenario(t), mod, r.Member("h")))
	if strings.Contains(string(view), "revealedToEnemy") || strings.Contains(string(view), "RevealedToEnemy") {
		t.Fatal("player view exposes enemy knowledge")
	}
	clone := r.cloneAuto()
	if !clone.Unit(player.ID).RevealedToEnemy {
		t.Fatal("reload lost revelation")
	}
	if err := clone.EditUnit(clone.Referee(), mod, player.ID, UnitEdit{RevealedToEnemy: &no}); err != nil {
		t.Fatal(err)
	}
	contacts, _ = noticedBy(t, clone, mod, enemy.ID)
	if len(contacts) != 0 {
		t.Fatal("regained concealment is still visible")
	}
}

func TestEnemyRangeConcealmentAndSightUseModuleAndScenarioData(t *testing.T) {
	r, mod, enemy, player := awarenessRoom(t)
	if !r.enemyNotices(mod, enemy, player) {
		t.Fatal("nearby exposed unit is absent")
	}
	r.Awareness = &Awareness{RangeCells: 0.5}
	if r.enemyNotices(mod, enemy, player) {
		t.Fatal("scenario observation range ignored")
	}
	r.Awareness = &Awareness{RangeCells: 3, ConcealedCells: 1.1}
	r.Map.SetTerrain(2, 1, 'f')
	if !r.enemyNotices(mod, enemy, player) {
		t.Fatal("scenario close observation of cover ignored")
	}
	r.Awareness = nil
	player.RevealedToEnemy = true
	player.X, player.Y = center(3, 1).X, center(3, 1).Y
	if r.enemyNotices(mod, enemy, player) {
		t.Fatal("revealed target seen through blocking woods")
	}
	r.Map.SetTerrain(2, 1, '.')
	if !r.enemyNotices(mod, enemy, player) {
		t.Fatal("clear sight line remained blocked")
	}
	side, ok := r.Map.SideBetween(1, 1, 2, 1)
	if !ok {
		t.Fatal("bad sight fixture")
	}
	if err := r.Map.SetEdge(side, "wall", ""); err != nil {
		t.Fatal(err)
	}
	if r.enemyNotices(mod, enemy, player) {
		t.Fatal("wall did not block sight")
	}
	r.Map.Edges = nil
	player.X, player.Y = center(5, 1).X, center(5, 1).Y
	if r.enemyNotices(mod, enemy, player) {
		t.Fatal("revealed target seen beyond range")
	}
	mod.Awareness = &Awareness{RangeCells: 5}
	if !r.enemyNotices(mod, enemy, player) {
		t.Fatal("module observation range ignored")
	}
	player.Strength = 0
	if r.enemyNotices(mod, enemy, player) {
		t.Fatal("destroyed unit is an active contact")
	}
}

func TestScenarioAwarenessPersistsAndGroupContactsHaveEstimatedNumbers(t *testing.T) {
	cat := testCatalog(t)
	sc := *cat.Scenarios["crossroads"]
	mod := cat.Modules[sc.Module]
	sc.Awareness = &Awareness{RangeCells: 6, ConcealedCells: 0}
	sc.Units = append([]ScenarioUnit(nil), sc.Units...)
	sc.Units[0].RevealedToEnemy = true
	r := newRoom("ABCDEF", "Awareness", &sc, mod, "Ref", HashToken("ref"))
	r.Phase, r.Turn = PhasePlanning, 1
	r.Map = &HexMap{Cols: 6, Rows: []string{"......", "......", "......"}}
	enemy, player := r.Unit("east"), r.Unit("line")
	r.Units = []*Unit{enemy, player}
	enemy.X, enemy.Y = center(1, 1).X, center(1, 1).Y
	player.X, player.Y = center(4, 1).X, center(4, 1).Y
	copy := r.cloneAuto()
	contacts, _ := noticedBy(t, copy, mod, enemy.ID)
	if len(contacts) != 1 || contacts[0].Numbers != estimate(player.Strength) || !copy.Unit(player.ID).RevealedToEnemy || copy.Awareness.RangeCells != 6 {
		t.Fatal("scenario range, revelation or estimated group numbers lost on reload")
	}
	sc.Awareness.RangeCells = 1
	if r.Awareness.RangeCells != 6 {
		t.Fatal("room shares mutable scenario awareness")
	}
}
