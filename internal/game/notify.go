package game

import (
	"fmt"
	"strings"
	"time"
)

// Notifications. Game code calls notify while it changes a room; the store
// collects them once the change is saved and passes them to whatever
// delivers them (web push). Nothing here knows how they're sent.

// Notice is one notification for one member.
type Notice struct {
	Room   string `json:"room"`
	Member string `json:"-"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	// Screen is where tapping it leads: "map", "orders", "messages?c=all", ...
	Screen string `json:"screen"`
	// Tag groups notifications so a newer one replaces an older one.
	Tag string `json:"tag,omitempty"`
}

// PushSub is one browser's web push subscription.
type PushSub struct {
	Endpoint string    `json:"endpoint"`
	P256dh   string    `json:"p256dh"`
	Auth     string    `json:"auth"`
	Added    time.Time `json:"added"`
}

// notify queues a notification for a member, if they can receive one.
func (r *Room) notify(memberID, title, body, screen string) {
	m := r.memberOrNil(memberID)
	if m == nil || m.Status != StatusApproved || len(m.Push) == 0 {
		return
	}
	r.Outbox = append(r.Outbox, Notice{Room: r.Code, Member: memberID, Title: title, Body: body, Screen: screen, Tag: r.Code + "/" + screen})
}

// notifyAll queues a notification for every approved member except one.
func (r *Room) notifyAll(except, title, body, screen string) {
	for _, m := range r.Members {
		if m.ID != except {
			r.notify(m.ID, title, body, screen)
		}
	}
}

// remindBefore is how long before the lock players hear their orders are due.
const remindBefore = 2 * time.Hour

// setReminder schedules the orders-due reminder, if the lock is far enough
// off for one to be useful.
func (r *Room) setReminder(now time.Time) {
	r.RemindAt = time.Time{}
	if r.Deadline.Sub(now) > remindBefore+time.Hour {
		r.RemindAt = r.Deadline.Add(-remindBefore)
	}
}

func (r *Room) reminderDue(now time.Time) bool {
	return !r.RemindAt.IsZero() && !now.Before(r.RemindAt)
}

// remind tells each member with units still lacking a committed order.
func (r *Room) remind() {
	r.RemindAt = time.Time{}
	missing := map[string]int{}
	for _, u := range r.Units {
		m := r.memberOrNil(u.AssignedTo)
		if m == nil || !m.InPlay() {
			continue
		}
		if o := r.CurrentOrder(u.ID); o == nil || o.Status != OrderCommitted {
			missing[m.ID]++
		}
	}
	for id, n := range missing {
		r.notify(id, fmt.Sprintf("Orders due %s", r.FormatInZone(r.Deadline)),
			fmt.Sprintf("%s still %s no committed order for turn %d.", capitalize(plural(n, "unit", "units")), map[bool]string{true: "has", false: "have"}[n == 1], r.Turn), "orders")
	}
}

// announceTurn tells players a new turn is open, and the results of the last.
func (r *Room) announceTurn(n int) {
	title, screen := "The operation has begun", "orders"
	if n > 1 {
		title, screen = fmt.Sprintf("Turn %d results are in", n-1), "reports"
	}
	body := fmt.Sprintf("Turn %d is open. Orders lock %s.", n, r.FormatInZone(r.Deadline))
	for _, m := range r.Members {
		if m.Role != RoleReferee {
			r.notify(m.ID, title, body, screen)
		}
	}
}

// checkAllIn tells the referee, once a turn, when every order is committed.
func (r *Room) checkAllIn() {
	if r.Phase != PhasePlanning || r.AllInTurn == r.Turn {
		return
	}
	in, total, _ := r.OrderReadiness()
	if total == 0 || in < total {
		return
	}
	r.AllInTurn = r.Turn
	if ref := r.Referee(); ref != nil {
		r.notify(ref.ID, fmt.Sprintf("All orders are in for turn %d", r.Turn), "You can lock the turn now rather than wait for the deadline.", "turn")
	}
}

// announceMessage tells everyone else who can read a channel about a post.
func (r *Room) announceMessage(author *Member, channel, text string) {
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > 140 {
		text = string([]rune(text)[:139]) + "…"
	}
	for _, m := range r.Members {
		if m.ID == author.ID || m.Status != StatusApproved || len(m.Push) == 0 {
			continue
		}
		c, ok := r.channel(m, channel)
		if !ok {
			continue
		}
		r.notify(m.ID, fmt.Sprintf("%s · %s", author.Name, c.Name), text, "messages?c="+channel)
	}
}
