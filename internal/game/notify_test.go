package game

import (
	"slices"
	"strings"
	"testing"
)

func TestNotificationTriggers(t *testing.T) {
	r, mod, ref, cmd, hicks := orderRoom(t)
	vasq := r.Member("v")
	for _, m := range r.Members {
		m.Push = []PushSub{{Endpoint: "https://push.example/" + m.ID}}
	}
	got := func(who *Member, title string) bool {
		return slices.ContainsFunc(r.Outbox, func(n Notice) bool { return n.Member == who.ID && strings.Contains(n.Title, title) })
	}
	now := central(t, "2026-09-30 13:00")

	// Commander returns Hicks's order.
	o, _ := r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "cautious", Fallback: "hold", Commit: true}, now)
	r.Outbox = nil
	r.ReviewOrder(cmd, o.ID, "return", "Dig in further east.", now)
	if !got(hicks, "Apone returned an order") {
		t.Errorf("no return notice: %+v", r.Outbox)
	}

	// The reminder goes only to players with orders missing.
	r.Outbox = nil
	r.SaveOrder(hicks, mod, OrderInput{Unit: "m1", Task: "dig_in", Risk: "cautious", Fallback: "hold", Commit: true}, now)
	r.remind()
	if got(hicks, "Orders due") || !got(vasq, "Orders due") {
		t.Errorf("reminders: %+v", r.Outbox)
	}

	// The last commit tells the referee, once.
	r.Outbox = nil
	r.SaveOrder(vasq, mod, OrderInput{Unit: "m2", Task: "hold", Risk: "balanced", Fallback: "hold", Commit: true}, now)
	in, total, _ := r.OrderReadiness()
	if in == total && !got(ref, "All orders are in") {
		t.Errorf("all-in: %d/%d %+v", in, total, r.Outbox)
	}
	r.Outbox = nil
	r.SaveOrder(vasq, mod, OrderInput{Unit: "m2", Task: "hold", Risk: "cautious", Fallback: "hold", Commit: true}, now)
	if got(ref, "All orders are in") {
		t.Error("told the referee twice")
	}

	// Lock tells the referee; publishing tells the players.
	r.Outbox = nil
	r.Tick(mod, r.Deadline)
	if !got(ref, "Turn 1 locked") {
		t.Errorf("lock: %+v", r.Outbox)
	}
	r.Outbox = nil
	publishTurn(t, r, mod, ref, now)
	if !got(hicks, "Turn 1 results are in") || !got(vasq, "Turn 1 results are in") || got(ref, "results") {
		t.Errorf("publish: %+v", r.Outbox)
	}

	// No subscription, no notice.
	vasq.Push = nil
	r.Outbox = nil
	r.Post(hicks, "players", "Hold the line.", now)
	if got(vasq, "Hicks") || !got(cmd, "Hicks · Players only") {
		t.Errorf("message notices: %+v", r.Outbox)
	}
}
