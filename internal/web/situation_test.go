package web

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"

	"asyncwargame/internal/game"
)

func TestSituationShowsOneSummaryAndExpandablePublicResults(t *testing.T) {
	ts, store, ref, player, code, p := setupSquad(t)
	ref.post("/r/"+code+"/start", nil)
	err := store.Update(code, func(r *game.Room) error {
		r.Reports = []*game.TurnReport{{Turn: 1, Operation: 1, Narrative: "The squad secured the bridge.", NarrativeStatus: game.NarrativeApproved, Entries: []game.ReportEntry{
			{Title: "Bridge", Text: "Public result detail", Seen: game.SeenByAll, Rolls: []string{"Public roll"}, Secret: []string{"Secret roll"}},
			{Title: "Discovery", Text: "Your private discovery", Seen: game.SeenByHolder, Holder: p.ID},
			{Title: "Hidden enemy", Text: "Referee secret", Seen: game.SeenByReferee},
		}}}
		r.Turn = 2
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := ts.Config.Handler.(*Server)
	render := func() string {
		var b bytes.Buffer
		store.Read(code, func(r *game.Room) {
			v := game.BuildView(r, store.Cat.Scenarios[r.ScenarioID], store.Cat.Modules[r.ModuleID], r.Member(p.ID))
			if err := srv.pages["screen-map"].ExecuteTemplate(&b, "sec-situation", v); err != nil {
				t.Fatal(err)
			}
		})
		return b.String()
	}
	body := render()
	if strings.Count(body, "The squad secured the bridge.") != 1 {
		t.Fatal("duplicate narrative")
	}
	i := strings.Index(body, `<details class="report-details">`)
	j := strings.Index(body, "Public result detail")
	k := strings.Index(body, "Your private discovery")
	if i < 0 || j < i || k < 0 || k > i {
		t.Fatal("public detail not collapsed or private update buried")
	}
	mustContain(t, body, "Order results and rolls", "Original mission briefing", "Objectives", "Public roll")
	mustNotContain(t, body, "Secret roll", "Referee secret", `class="report-details" open`)
	// An absent/unapproved narrative must leave the actual report visible.
	store.Update(code, func(r *game.Room) error { r.Reports[0].NarrativeStatus = game.NarrativeDraft; return nil })
	body = render()
	mustNotContain(t, body, "The squad secured the bridge.", `class="report-details"`)
	mustContain(t, body, "Public result detail", "Your private discovery")
	_, body = player.get("/r/" + code)
	mustNotContain(t, body, `class="brief" open`)
}

func TestDestroyedControlAndPublicationOverHTTP(t *testing.T) {
	_, store, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	_, body := ref.get(base + "/units/m2")
	mustContain(t, body, "Mark destroyed")
	if res, _ := player.post(base+"/units/m2/edit", url.Values{"strength": {"0"}}); res.StatusCode != 403 {
		t.Fatal("player destroyed unit")
	}
	ref.post(base+"/turn/lock", nil)
	if res, body := ref.post(base+"/units/m2/edit", url.Values{"strength": {"0"}}); res.StatusCode != 200 {
		t.Fatalf("destroy: %d %s", res.StatusCode, body)
	}
	_, body = player.get(base + "/units/m2")
	mustNotContain(t, body, "DESTROYED")
	if err := store.Update(code, func(r *game.Room) error { return r.PublishTurn(r.Referee(), store.Cat.Modules[r.ModuleID], time.Now()) }); err != nil {
		t.Fatal(err)
	}
	_, body = player.get(base + "/units/m2")
	mustContain(t, body, "DESTROYED", "destroyed-cross")
	_, body = ref.get(base + "/units/m2")
	mustContain(t, body, "set strength above zero to restore")
}

func TestCompletedMissionRendersFinalResultsAndCorrection(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	res, _ := ref.post("/rooms", url.Values{"campaign": {"Two player mission"}, "name": {"Ref"}, "scenario": {"crossroads"}})
	if res.StatusCode != 200 {
		t.Fatal("create mission")
	}
	codes := store.Codes()
	if len(codes) != 1 {
		t.Fatal(codes)
	}
	code := codes[0]
	base := "/r/" + code
	ref.post(base+"/start", nil)
	err := store.Update(code, func(r *game.Room) error {
		mod := store.Cat.Modules[r.ModuleID]
		r.Unit("line").Strength = 0
		r.Unit("scouts").Strength = 0
		if err := r.LockNow(r.Referee(), mod, time.Now()); err != nil {
			return err
		}
		return r.PublishTurn(r.Referee(), mod, time.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	_, body := ref.get(base)
	mustContain(t, body, "Mission failure", "failed", "Reopen turn 1")
	mustNotContain(t, body, "Lock turn 1 now", "Publish turn 1 and open turn 2", "data-plan")
	_, body = ref.get(base + "/setup")
	mustContain(t, body, "The mission is complete.")
	mustNotContain(t, body, "It waits for you")
	if res, _ := ref.post(base+"/turn/lock", nil); res.StatusCode != 400 {
		t.Fatal("completed mission locked again")
	}
}
