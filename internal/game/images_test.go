package game

import (
	"strings"
	"testing"
	"time"
)

func TestVignettesUseOnlyPublicBriefAndReportText(t *testing.T) {
	r, mod, ref, _, player := orderRoom(t)
	r.Briefing = []string{"shared opening"}
	r.RefereeNotes = "secret referee note"
	r.Reports = []*TurnReport{{Operation: 1, Turn: 1, Narrative: "unapproved narrative", NarrativeStatus: NarrativeDraft, Entries: []ReportEntry{
		{Title: "Visible", Text: "public result", Seen: SeenByAll, Order: "private order", Secret: []string{"hidden roll"}},
		{Title: "Secret", Text: "private result", Seen: SeenByHolder},
	}}}
	v := BuildView(r, testScenario(t), mod, ref)
	prompt := v.Reports[0].Art.SuggestedPrompt
	if !strings.Contains(prompt, "public result") {
		t.Fatal("missing shared report text")
	}
	for _, s := range []string{"private", "secret", "hidden", "unapproved"} {
		if strings.Contains(prompt, s) {
			t.Fatalf("prompt leaks %s", s)
		}
	}
	if err := r.QueueVignette(ref, 1, 1, "A phalanx waiting at dawn", "Soldiers at dawn", "images", "art-model", time.Now()); err != nil {
		t.Fatal(err)
	}
	d := r.Reports[0].Vignette
	d.Status, d.File = RunDone, "art.png"
	if v := BuildView(r, testScenario(t), mod, player); v.Reports[0].Art.Image != nil || v.Reports[0].Art.SuggestedPrompt != "" {
		t.Fatal("player sees private image draft")
	}
	if err := r.DecideVignette(ref, 1, 1, d.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	shown := BuildView(r, testScenario(t), mod, player).Reports[0].Art.Image
	if shown == nil || shown.Caption != "Soldiers at dawn" || shown.Prompt != "" || shown.Model != "" || shown.Provider != "" {
		t.Fatalf("bad filtered vignette: %+v", shown)
	}
	r.snapshot(time.Now())
	r.Reports[0].Vignette = nil
	// Report snapshots retain image metadata for turn correction.
	if !strings.Contains(string(r.Undo.State), "art.png") {
		t.Fatal("snapshot lost image")
	}
}
