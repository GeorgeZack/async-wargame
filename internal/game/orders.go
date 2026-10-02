package game

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Orders. A player writes what their marine does, in their own words, and
// may mark a spot on the map. An order stays a draft until committed; the
// squad leader can review it and send it back; at the lock it freezes. A
// marine without a committed order carries on as before, and the referee
// decides what that means.

type OrderStatus string

const (
	OrderDraft     OrderStatus = "draft"
	OrderCommitted OrderStatus = "committed"
	OrderLocked    OrderStatus = "locked"
	OrderLapsed    OrderStatus = "lapsed" // a draft never committed before lock
	OrderWithdrawn OrderStatus = "withdrawn"
)

const maxOrderLen = 1500

// Order is one unit's order for one turn.
type Order struct {
	Group       string          `json:"group,omitempty"` // shared squad instruction; empty for individual orders
	ID          string          `json:"id"`
	Turn        int             `json:"turn"`
	Unit        string          `json:"unit"`
	Author      string          `json:"author"` // member id
	Text        string          `json:"text"`
	Mark        *Pos            `json:"mark,omitempty"` // a spot on the map, if the player marked one
	Status      OrderStatus     `json:"status"`
	Version     int             `json:"version"`
	CommittedAt *time.Time      `json:"committedAt,omitempty"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	History     []OrderRevision `json:"history,omitempty"`
	Review      string          `json:"review,omitempty"` // approved or returned
	ReviewNote  string          `json:"reviewNote,omitempty"`
	ReviewedBy  string          `json:"reviewedBy,omitempty"`
}

type OrderRevision struct {
	Version int         `json:"version"`
	At      time.Time   `json:"at"`
	By      string      `json:"by"`
	Status  OrderStatus `json:"status"`
	Text    string      `json:"text"`
}

// CanOrder reports whether actor may give unit u orders: its holder, the
// holder's commander, or the referee.
func (r *Room) CanOrder(mod *Module, actor *Member, u *Unit) bool {
	if s := mod.Side(u.Side); s == nil || !s.Playable {
		return false
	}
	switch actor.Role {
	case RoleReferee:
		return true
	case RoleCommander:
		return u.AssignedTo == actor.ID || r.commands(actor, r.memberOrNil(u.AssignedTo))
	case RolePlayer:
		return actor.InPlay() && u.AssignedTo == actor.ID
	}
	return false
}

// CurrentOrder is a unit's order for this turn, if any.
func (r *Room) CurrentOrder(unitID string) *Order {
	for i := len(r.Orders) - 1; i >= 0; i-- {
		o := r.Orders[i]
		if o.Unit == unitID && o.Turn == r.Turn && o.Status != OrderWithdrawn {
			return o
		}
	}
	return nil
}

// SaveOrder writes or revises a unit's order for this turn. mark is a spot
// on the board, or nil.
func (r *Room) SaveOrder(actor *Member, mod *Module, unitID, text string, mark *Pos, commit bool, now time.Time) (*Order, error) {
	if strings.HasPrefix(unitID, "f-") {
		return r.SaveSquadOrder(actor, mod, strings.TrimPrefix(unitID, "f-"), text, mark, commit, now)
	}
	if r.Phase != PhasePlanning {
		return nil, bad("orders can only be written while a turn is in planning")
	}
	u := r.Unit(unitID)
	if u == nil {
		return nil, ErrNotFound
	}
	if !r.CanOrder(mod, actor, u) || !r.seesOrder(actor, u) {
		return nil, ErrForbidden
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return nil, bad("say what %s does", u.Name)
	}
	if len([]rune(text)) > maxOrderLen {
		return nil, bad("keep an order under %d characters", maxOrderLen)
	}
	var at *Pos
	if mark != nil {
		if math.IsNaN(mark.X) || math.IsNaN(mark.Y) || math.IsInf(mark.X, 0) || math.IsInf(mark.Y, 0) {
			return nil, bad("choose a point on the map")
		}
		p := r.clampPos(*mark)
		at = &p
	}
	o := r.CurrentOrder(u.ID)
	if o == nil {
		o = &Order{ID: newID(), Turn: r.Turn, Unit: u.ID}
		r.Orders = append(r.Orders, o)
	}
	// Keep this seat's planning ghost in step with an explicitly edited pin.
	for _, p := range r.PlansFor(actor) {
		if p.Unit == u.ID && at != nil {
			p.To = *at
		}
	}
	if at == nil {
		r.Plans = slices.DeleteFunc(r.Plans, func(p *Plan) bool { return p.Member == actor.ID && p.Turn == r.Turn && p.Unit == u.ID })
	}
	o.Group = ""
	o.Author, o.Text, o.Mark = actor.ID, text, at
	o.Status, o.Review, o.ReviewNote, o.ReviewedBy, o.CommittedAt = OrderDraft, "", "", "", nil
	if commit {
		o.Status, o.CommittedAt = OrderCommitted, &now
	}
	o.Version++
	o.UpdatedAt = now.UTC()
	o.History = append(o.History, OrderRevision{Version: o.Version, At: now.UTC(), By: actor.Name, Status: o.Status, Text: text})
	if commit {
		r.logKind(VisPlayers, "", "%s committed an order for %s.", actor.Name, u.Name)
		r.checkAllIn()
	}
	return o, nil
}

// WithdrawOrder cancels an order before lock.
func (r *Room) WithdrawOrder(actor *Member, mod *Module, id string, now time.Time) error {
	if r.Phase != PhasePlanning {
		return bad("orders can only be withdrawn while a turn is in planning")
	}
	for _, o := range r.Orders {
		if o.ID != id || o.Turn != r.Turn || o.Status == OrderWithdrawn {
			continue
		}
		u := r.Unit(o.Unit)
		if u == nil || !r.CanOrder(mod, actor, u) || !r.seesOrder(actor, u) {
			return ErrForbidden
		}
		peers := r.sharedOrderPeers(o)
		for _, p := range peers {
			unit := r.Unit(p.Unit)
			if unit == nil || !r.CanOrder(mod, actor, unit) || !r.seesOrder(actor, unit) {
				return ErrForbidden
			}
		}
		for _, p := range peers {
			p.Status, p.UpdatedAt = OrderWithdrawn, now.UTC()
			p.History = append(p.History, OrderRevision{Version: p.Version, At: now.UTC(), By: actor.Name, Status: OrderWithdrawn})
		}
		if o.Group == "" {
			return r.restoreSquadOrder(actor, mod, u, now)
		}
		return nil
	}
	return ErrNotFound
}

// Review results.
const (
	ReviewApproved = "approved"
	ReviewReturned = "returned"
)

// CanReview reports whether actor, a commander, may review u's order: one
// of their players holds it and they can see it.
func (r *Room) CanReview(actor *Member, u *Unit) bool {
	if actor.Role != RoleCommander || u == nil {
		return false
	}
	holder := r.memberOrNil(u.AssignedTo)
	return holder != nil && holder.ID != actor.ID && holder.ReportsTo == actor.ID && r.seesOrder(actor, u)
}

// ReviewOrder is a commander approving a player's committed order, or
// returning it to draft with a note.
func (r *Room) ReviewOrder(actor *Member, id, action, note string, now time.Time) error {
	if r.Phase != PhasePlanning {
		return bad("orders can only be reviewed while a turn is in planning")
	}
	var o *Order
	for _, x := range r.Orders {
		if x.ID == id && x.Turn == r.Turn {
			o = x
		}
	}
	if o == nil {
		return ErrNotFound
	}
	u := r.Unit(o.Unit)
	for _, peer := range r.sharedOrderPeers(o) {
		if !r.CanReview(actor, r.Unit(peer.Unit)) {
			return ErrForbidden
		}
	}
	if !r.CanReview(actor, u) {
		return ErrForbidden
	}
	if o.Status != OrderCommitted {
		return bad("only a committed order can be reviewed")
	}
	holder := r.memberOrNil(u.AssignedTo)
	note = strings.TrimSpace(note)
	switch action {
	case "approve":
		o.Review, o.ReviewNote = ReviewApproved, clip(note, 300)
		r.logKind(VisPlayers, "", "%s approved %s's order for %s.", actor.Name, holder.Name, u.Name)
	case "return":
		if note == "" {
			return bad("say what should change")
		}
		o.Review, o.ReviewNote = ReviewReturned, clip(note, 300)
		o.Status, o.CommittedAt = OrderDraft, nil
		r.logKind(VisPlayers, "", "%s sent back %s's order for %s: “%s”", actor.Name, holder.Name, u.Name, o.ReviewNote)
		r.notify(holder.ID, fmt.Sprintf("%s sent back an order", actor.Name), fmt.Sprintf("%s: “%s” Revise it and commit before the lock.", u.Name, o.ReviewNote), "orders")
	default:
		return bad("approve or return")
	}
	o.ReviewedBy, o.Version = actor.Name, o.Version+1
	o.History = append(o.History, OrderRevision{Version: o.Version, At: now.UTC(), By: actor.Name, Status: o.Status, Text: o.Text})
	for _, peer := range r.sharedOrderPeers(o) {
		if peer == o {
			continue
		}
		peer.Review, peer.ReviewNote, peer.ReviewedBy, peer.Status, peer.CommittedAt = o.Review, o.ReviewNote, o.ReviewedBy, o.Status, o.CommittedAt
		peer.Version++
		peer.History = append(peer.History, OrderRevision{Version: peer.Version, At: now.UTC(), By: actor.Name, Status: peer.Status, Text: peer.Text})
	}
	return nil
}

// lockOrders freezes committed orders and lets drafts lapse.
func (r *Room) lockOrders() {
	for _, o := range r.Orders {
		if o.Turn != r.Turn {
			continue
		}
		switch o.Status {
		case OrderCommitted:
			o.Status = OrderLocked
		case OrderDraft:
			o.Status = OrderLapsed
		}
	}
}

// OrderReadiness counts units held by players and how many have committed
// orders, and lists who's still writing.
func (r *Room) OrderReadiness() (in, total int, waiting []string) {
	missing := map[string]int{}
	var order []string
	for _, u := range r.Units {
		m := r.memberOrNil(u.AssignedTo)
		if m == nil || !m.InPlay() {
			continue
		}
		total++
		if o := r.CurrentOrder(u.ID); o != nil && o.Status == OrderCommitted {
			in++
			continue
		}
		if missing[m.Name] == 0 {
			order = append(order, m.Name)
		}
		missing[m.Name]++
	}
	for _, name := range order {
		waiting = append(waiting, fmt.Sprintf("%s (%s)", name, plural(missing[name], "unit", "units")))
	}
	return in, total, waiting
}

// LockedOrders are this turn's orders in the order units appear.
func (r *Room) LockedOrders() []*Order {
	var out []*Order
	groups := map[string]bool{}
	for _, u := range r.Units {
		if o := r.CurrentOrder(u.ID); o != nil && o.Status == OrderLocked {
			if o.Group != "" {
				if groups[o.Group] {
					continue
				}
				groups[o.Group] = true
			}
			out = append(out, o)
		}
	}
	return out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
