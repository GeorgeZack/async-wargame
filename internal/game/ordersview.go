package game

import (
	"fmt"
	"strings"
	"time"
)

// OrderView is an order as shown to someone allowed to see it.
type OrderView struct {
	ID         string      `json:"id"`
	UnitID     string      `json:"unit"`
	UnitName   string      `json:"unitName"`
	Task       string      `json:"task"`
	TaskName   string      `json:"taskName"`
	Dest       string      `json:"dest,omitempty"`
	DestLabel  string      `json:"destLabel,omitempty"`
	Cost       int         `json:"cost,omitempty"`
	Intent     string      `json:"intent,omitempty"`
	Risk       string      `json:"risk"`
	Fallback   string      `json:"fallback"`
	Status     OrderStatus `json:"status"`
	Version    int         `json:"version"`
	Author     string      `json:"author"`
	UpdatedAt  time.Time   `json:"-"`
	Editable   bool        `json:"editable"`
	Summary    string      `json:"summary"`
	Transport  string      `json:"transport,omitempty"`
	Review     string      `json:"review,omitempty"`
	ReviewNote string      `json:"reviewNote,omitempty"`
	ReviewedBy string      `json:"reviewedBy,omitempty"`
	Standing   bool        `json:"standing,omitempty"`
	Then       string      `json:"then,omitempty"`
	Carried    bool        `json:"carried,omitempty"`
	Goal       string      `json:"goal,omitempty"`
	CanReview  bool        `json:"-"`
	Holder     string      `json:"-"`
}

// OrderSlot is a unit the viewer may give orders to.
type OrderSlot struct {
	Unit      UnitView
	Movement  int
	Order     *OrderView
	NoContact bool   // off the command net: its orders stay with the player holding it
	Holder    string // that player
}

type OrderLine struct {
	Points string
	Status OrderStatus
	Mine   bool
	Label  string
}

// OrderData is the order-entry part of the page's JSON.
type OrderData struct {
	Open          bool                         `json:"open"`
	Reach         map[string]map[string]any    `json:"reach"` // unit id -> hex -> [cost, previous hex]
	Movement      map[string]int               `json:"movement"`
	Need          map[string]int               `json:"need"` // unit id -> stacking points it uses
	Orders        map[string]OrderView         `json:"orders"`
	NeedDest      map[string]bool              `json:"needDest"`      // task id -> needs a destination
	Units         []OrderUnit                  `json:"units"`         // units the viewer can order, for the form's unit list
	Transports    map[string][]TransportOption `json:"transports"`    // unit id -> transports it can board
	NeedTransport map[string]bool              `json:"needTransport"` // task id -> picks a transport
	Aboard        map[string]string            `json:"aboard"`        // unit id -> transport name
}

type OrderUnit struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (r *Room) orderView(mod *Module, o *Order, editable bool) OrderView {
	u := r.Unit(o.Unit)
	name := o.Unit
	if u != nil {
		name = u.Name
	}
	ov := OrderView{ID: o.ID, UnitID: o.Unit, UnitName: name, Task: o.Task, Cost: o.Cost, Intent: o.Intent, Transport: o.Transport,
		Review: o.Review, ReviewNote: o.ReviewNote, ReviewedBy: o.ReviewedBy, Standing: o.Standing, Then: o.Then, Carried: o.Carried,
		Risk: o.Risk, Fallback: o.Fallback, Status: o.Status, Version: o.Version, UpdatedAt: o.UpdatedAt,
		Editable: editable && r.Phase == PhasePlanning, Summary: r.orderSummary(mod, o)}
	if t := mod.Orders.Task(o.Task); t != nil {
		ov.TaskName = t.Name
	}
	if m := r.Member(o.Author); m != nil {
		ov.Author = m.Name
	}
	if o.Goal != nil {
		ov.Goal = HexID(o.Goal[0], o.Goal[1])
	}
	if o.HasDest {
		ov.Dest, ov.DestLabel = HexID(o.DestCol, o.DestRow), r.Map.SectorAt(o.DestCol, o.DestRow)
	}
	return ov
}

// addOrders fills the order parts of a view for member me.
func addOrders(v *View, r *Room, mod *Module, me *Member) {
	defer markOrderStatus(v, r, mod, me)
	v.OrderSpec = mod.Orders
	v.Data.Orders = OrderData{Open: r.Phase == PhasePlanning, Reach: map[string]map[string]any{},
		Movement: map[string]int{}, Need: map[string]int{}, Orders: map[string]OrderView{}, NeedDest: map[string]bool{},
		Transports: map[string][]TransportOption{}, NeedTransport: map[string]bool{}, Aboard: map[string]string{}}
	for _, t := range mod.Orders.Tasks {
		v.Data.Orders.NeedDest[t.ID] = t.Destination
		v.Data.Orders.NeedTransport[t.ID] = t.Embark
	}
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
		uv := unitView(u, mod, me.Role == RoleReferee)
		if r.CommsActive(mod) || u.OutOfContact {
			uv.Radio, uv.RadioText = "up", r.radioText(u)
			if u.OutOfContact {
				uv.Radio = "down"
			}
		}
		if !r.seesOrder(me, u) {
			// A commander's unit that's off the net: shown where it last
			// reported from, but out of reach.
			if at, stale := r.reportedAt(u, me); stale {
				uv.Hex, uv.Stale, uv.SeenTurn = HexID(at[0], at[1]), true, u.LastContact
			}
			slot := OrderSlot{Unit: uv, Movement: mod.Class(u.Class).Movement, NoContact: true}
			if m := r.memberOrNil(u.AssignedTo); m != nil {
				slot.Holder = m.Name
			}
			v.MyOrders = append(v.MyOrders, slot)
			continue
		}
		mine[u.ID] = true
		slot := OrderSlot{Unit: uv, Movement: mod.Class(u.Class).Movement}
		if o := r.CurrentOrder(u.ID); o != nil {
			ov := r.orderView(mod, o, true)
			ov.CanReview = r.Phase == PhasePlanning && o.Status == OrderCommitted && o.Review == "" && r.CanReview(me, u)
			if h := r.memberOrNil(u.AssignedTo); h != nil {
				ov.Holder = h.Name
			}
			if ov.CanReview {
				v.AwaitingReview++
			}
			if o.Carried && o.Status == OrderDraft && r.Phase == PhasePlanning {
				v.StandingDrafts++
			}
			slot.Order = &ov
			v.Data.Orders.Orders[u.ID] = ov
			if o.Status == OrderCommitted {
				v.CommittedCount++
			}
		}
		v.MyOrders = append(v.MyOrders, slot)
		label := fmt.Sprintf("%s · %s · movement %d", u.Name, HexID(u.Col, u.Row), slot.Movement)
		if t := r.Unit(u.Aboard); t != nil {
			label = fmt.Sprintf("%s · aboard %s", u.Name, t.Name)
		}
		v.Data.Orders.Units = append(v.Data.Orders.Units, OrderUnit{ID: u.ID, Label: label})
		if r.Phase == PhasePlanning {
			reach := map[string]any{}
			load := r.ProjectedLoad(mod, u.ID, me)
			fullReach := r.Reach(mod, u)
			for h, st := range fullReach {
				free := r.StackLimit(mod, h[0], h[1]) - load[h]
				reach[HexID(h[0], h[1])] = []any{st.Cost, HexID(st.Prev[0], st.Prev[1]), free}
			}
			v.Data.Orders.Transports[u.ID] = r.TransportsFor(mod, u, fullReach, me)
			if t := r.Unit(u.Aboard); t != nil {
				v.Data.Orders.Aboard[u.ID] = t.Name
			}
			v.Data.Orders.Need[u.ID] = stackPoints(mod, u)
			v.Data.Orders.Reach[u.ID] = reach
			v.Data.Orders.Movement[u.ID] = slot.Movement
		}
	}
	for _, o := range r.Orders {
		if o.Turn != r.Turn || o.Status == OrderWithdrawn || o.Status == OrderLapsed {
			continue
		}
		shared := o.Status == OrderCommitted || o.Status == OrderLocked
		if !r.seesOrder(me, r.Unit(o.Unit)) {
			continue
		}
		if !mine[o.Unit] && !shared && me.Role != RoleReferee {
			continue // drafts stay with their author, commander, and referee
		}
		if !mine[o.Unit] {
			v.OtherOrders = append(v.OtherOrders, r.orderView(mod, o, false))
		}
		if o.HasDest {
			pts := []string{point(o.FromCol, o.FromRow)}
			for _, h := range o.Route {
				pts = append(pts, point(h[0], h[1]))
			}
			name := o.Unit
			if u := r.Unit(o.Unit); u != nil {
				name = u.Name
			}
			v.Map.OrderLines = append(v.Map.OrderLines, OrderLine{Points: strings.Join(pts, " "), Status: o.Status,
				Mine: mine[o.Unit], Label: fmt.Sprintf("%s: %s (%s)", name, r.orderSummary(mod, o), o.Status)})
		}
	}
}

// markOrderStatus shows each unit's order status in the forces ledger, for
// orders the viewer may see.
func markOrderStatus(v *View, r *Room, mod *Module, me *Member) {
	for gi := range v.Forces {
		for ui := range v.Forces[gi].Units {
			uv := &v.Forces[gi].Units[ui]
			u := r.Unit(uv.ID)
			if u == nil || !mod.Side(u.Side).Playable || !r.seesOrder(me, u) || r.Phase == PhaseSetup {
				continue
			}
			o := r.CurrentOrder(u.ID)
			switch {
			case o != nil && (o.Status == OrderCommitted || o.Status == OrderLocked || me.Role == RoleReferee || r.CanOrder(mod, me, u)):
				uv.OrderStatus = string(o.Status)
			case o == nil && r.memberOrNil(u.AssignedTo) != nil && r.Phase == PhasePlanning:
				uv.OrderStatus = "none"
			}
		}
	}
}

func point(col, row int) string {
	x, y := HexCenter(col, row)
	return fmt.Sprintf("%.1f,%.1f", x, y)
}
