package game

import "time"

// OrderView is an order as one member may see it.
type OrderView struct {
	ID         string      `json:"id"`
	UnitID     string      `json:"unit"`
	UnitName   string      `json:"unitName"`
	Text       string      `json:"text"`
	Mark       string      `json:"mark,omitempty"`
	MarkLabel  string      `json:"markLabel,omitempty"`
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
	NoContact bool   // out of contact: the viewer (a commander) can't reach it
	Holder    string // who holds it, when that's not the viewer
}

// OrderData is what the order form needs on the client.
type OrderData struct {
	Open   bool                 `json:"open"`
	Orders map[string]OrderView `json:"orders"` // by unit id
	Units  []OrderUnit          `json:"units"`
}

type OrderUnit struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hex   string `json:"hex"`
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
		ov.Mark, ov.MarkLabel = HexID(o.Mark[0], o.Mark[1]), r.Map.SectorAt(o.Mark[0], o.Mark[1])
	}
	return ov
}

// addOrders fills the order panels and the map's order marks.
func addOrders(v *View, r *Room, mod *Module, me *Member) {
	v.Data.Orders = OrderData{Open: r.Phase == PhasePlanning, Orders: map[string]OrderView{}}
	if r.Phase == PhaseSetup {
		return
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
				uv.Hex, uv.Stale, uv.SeenTurn = HexID(at[0], at[1]), true, u.LastContact
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
		v.Data.Orders.Units = append(v.Data.Orders.Units, OrderUnit{ID: u.ID, Label: u.Name, Hex: HexID(u.Col, u.Row)})
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
		if o.Mark == "" {
			return
		}
		col, row, _ := ParseHexID(o.Mark)
		x, y := HexCenter(col, row)
		v.Map.OrderMarks = append(v.Map.OrderMarks, OrderMark{X: x, Y: y, Hex: o.Mark, Mine: own, Status: string(o.Status),
			Label: o.UnitName + ": " + clip(o.Text, 120)})
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
