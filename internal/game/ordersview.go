package game

import "time"

// OrderView is an order as one member may see it.
type OrderView struct {
	ID         string      `json:"id"`
	UnitID     string      `json:"unit"`
	UnitName   string      `json:"unitName"`
	Text       string      `json:"text"`
	Mark       *Pos        `json:"mark,omitempty"`
	MarkLabel  string      `json:"markLabel,omitempty"` // where the mark is, in words
	Status     OrderStatus `json:"status"`
	Version    int         `json:"version"`
	Author     string      `json:"author"`
	UpdatedAt  time.Time   `json:"-"`
	Editable   bool        `json:"editable"`
	Review     string      `json:"review,omitempty"`
	ReviewNote string      `json:"reviewNote,omitempty"`
	ReviewedBy string      `json:"reviewedBy,omitempty"`
	CanReview  bool        `json:"-"`
	Holder     string      `json:"-"`
}

// OrderSlot is a unit the viewer may give orders to.
type OrderSlot struct {
	Unit      UnitView
	Order     *OrderView
	NoContact bool      // out of contact: the viewer (a commander) can't reach it
	Holder    string    // who holds it, when that's not the viewer
	Moved     *MoveView // referee: the counter was moved by hand this turn
}

// CheckIn is one player unit's order status, for the referee: who's in,
// even while they referee blind (the status, never the text).
type CheckIn struct {
	Unit   UnitView
	Holder string
	Status string    // "in", "draft", "returned", "none"
	At     time.Time // when it was committed
	Moved  *MoveView
}

// OrderData is what the order form needs on the client.
type OrderData struct {
	Open   bool                 `json:"open"`
	Orders map[string]OrderView `json:"orders"` // by unit id
	Units  []OrderUnit          `json:"units"`
}

type OrderUnit struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

func (r *Room) orderView(o *Order, editable bool) OrderView {
	ov := OrderView{ID: o.ID, UnitID: o.Unit, Text: o.Text, Status: o.Status, Version: o.Version, UpdatedAt: o.UpdatedAt,
		Editable: editable && r.Phase == PhasePlanning, Review: o.Review, ReviewNote: o.ReviewNote, ReviewedBy: o.ReviewedBy}
	if u := r.Unit(o.Unit); u != nil {
		ov.UnitName = u.Name
	}
	if m := r.memberOrNil(o.Author); m != nil {
		ov.Author = m.Name
	}
	if o.Mark != nil {
		p := *o.Mark
		ov.Mark, ov.MarkLabel = &p, r.Where(p, false)
	}
	return ov
}

// addOrders fills the order panels and the map's order marks.
func addOrders(v *View, r *Room, mod *Module, me *Member) {
	v.Data.Orders = OrderData{Open: r.Phase == PhasePlanning, Orders: map[string]OrderView{}}
	if r.Phase == PhaseSetup {
		return
	}
	if me.Role == RoleReferee && r.Phase == PhasePlanning {
		for _, u := range r.Units {
			h := r.memberOrNil(u.AssignedTo)
			if !mod.Side(u.Side).Playable || h == nil || h.Role == RoleReferee || h.Status != StatusApproved {
				continue
			}
			c := CheckIn{Unit: unitView(r, u, mod, me), Holder: h.Name, Status: "none", Moved: r.MoveOf(u.ID)}
			if o := r.CurrentOrder(u.ID); o != nil {
				switch {
				case o.Status == OrderCommitted:
					c.Status, c.At = "in", o.UpdatedAt
				case o.Review == "returned":
					c.Status = "returned"
				default:
					c.Status = "draft"
				}
			}
			v.CheckIns = append(v.CheckIns, c)
		}
	}
	mine := map[string]bool{}
	for _, u := range r.Units {
		if !r.CanOrder(mod, me, u) {
			continue
		}
		if r.Blind(me) && !r.seesOrder(me, u) {
			continue // a blind referee doesn't see player units' orders
		}
		uv := unitView(r, u, mod, me)
		if !r.seesOrder(me, u) {
			// A commander's marine who's out of contact: shown, but out of reach.
			if at, stale := r.reportedAt(u, me); stale {
				uv.X, uv.Y, uv.Where, uv.Stale, uv.SeenTurn = at.X, at.Y, r.Where(at, false), true, u.LastContact
			}
			slot := OrderSlot{Unit: uv, NoContact: true}
			if m := r.memberOrNil(u.AssignedTo); m != nil {
				slot.Holder = m.Name
			}
			v.MyOrders = append(v.MyOrders, slot)
			continue
		}
		mine[u.ID] = true
		slot := OrderSlot{Unit: uv}
		if me.Role == RoleReferee {
			slot.Moved = r.MoveOf(u.ID)
		}
		if h := r.memberOrNil(u.AssignedTo); h != nil && h.ID != me.ID {
			slot.Holder = h.Name
		}
		if o := r.CurrentOrder(u.ID); o != nil {
			ov := r.orderView(o, true)
			ov.CanReview = r.Phase == PhasePlanning && o.Status == OrderCommitted && o.Review == "" && r.CanReview(me, u)
			ov.Holder = slot.Holder
			if ov.CanReview {
				v.AwaitingReview++
			}
			slot.Order = &ov
			v.Data.Orders.Orders[u.ID] = ov
			if o.Status == OrderCommitted {
				v.CommittedCount++
			}
		}
		v.MyOrders = append(v.MyOrders, slot)
		v.Data.Orders.Units = append(v.Data.Orders.Units, OrderUnit{ID: u.ID, Label: u.Name, X: u.X, Y: u.Y})
	}
	// Other friendly orders the viewer may read: committed and in contact.
	for _, o := range r.Orders {
		if o.Turn != r.Turn || mine[o.Unit] {
			continue
		}
		if o.Status != OrderCommitted && o.Status != OrderLocked {
			continue
		}
		u := r.Unit(o.Unit)
		if u == nil || !r.seesOrder(me, u) || r.Blind(me) && u.AssignedTo != "" {
			continue
		}
		v.OtherOrders = append(v.OtherOrders, r.orderView(o, false))
	}
	// Marks on the map: the viewer's own orders and the others they can read.
	mark := func(o OrderView, own bool) {
		if o.Mark == nil {
			return
		}
		v.Map.OrderMarks = append(v.Map.OrderMarks, OrderMark{X: o.Mark.X, Y: o.Mark.Y, Mine: own, Status: string(o.Status),
			Label: o.UnitName + " (" + o.MarkLabel + "): " + clip(o.Text, 120)})
	}
	for _, o := range v.Data.Orders.Orders {
		mark(o, true)
	}
	for _, o := range v.OtherOrders {
		mark(o, false)
	}
}

// markOrderStatus notes each visible unit's order status for the forces list.
func markOrderStatus(v *View) {
	for gi := range v.Forces {
		for ui := range v.Forces[gi].Units {
			u := &v.Forces[gi].Units[ui]
			if o, ok := v.Data.Orders.Orders[u.ID]; ok {
				u.OrderStatus = string(o.Status)
			}
		}
	}
}
