package game

import "time"

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
