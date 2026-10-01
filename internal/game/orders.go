package game

import (
	"container/heap"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Orders. Each turn a unit can have one order: a task, a destination when
// the task needs one, the route there, the player's intent, acceptable risk,
// and a fallback. Orders start as drafts; committing one puts it before the
// referee, and the lock freezes committed orders for the turn. A unit
// without a committed order at lock holds its ground and takes no offensive
// action, as the design doc's absence rule says.
//
// Destinations are limited by the unit's Movement stat in movement points,
// spent on terrain, edges, and crossings, so the server (not the browser)
// decides what's in range. Players plan only around enemies they can see.

type OrderStatus string

const (
	OrderDraft     OrderStatus = "draft"
	OrderCommitted OrderStatus = "committed"
	OrderLocked    OrderStatus = "locked"
	OrderLapsed    OrderStatus = "lapsed" // a draft never committed before lock
	OrderWithdrawn OrderStatus = "withdrawn"
)

const (
	maxIntentLen = 400
)

type Order struct {
	ID          string          `json:"id"`
	Turn        int             `json:"turn"`
	Unit        string          `json:"unit"`
	Author      string          `json:"author"` // member id of the latest editor
	Task        string          `json:"task"`
	HasDest     bool            `json:"hasDest,omitempty"`
	DestCol     int             `json:"destCol,omitempty"`
	DestRow     int             `json:"destRow,omitempty"`
	FromCol     int             `json:"fromCol"`
	FromRow     int             `json:"fromRow"`
	Route       [][2]int        `json:"route,omitempty"`     // hexes after the start, ending at the destination
	Transport   string          `json:"transport,omitempty"` // embark: the transport to board
	Review      string          `json:"review,omitempty"`    // approved or returned, by the holder's commander
	ReviewNote  string          `json:"reviewNote,omitempty"`
	ReviewedBy  string          `json:"reviewedBy,omitempty"`
	Standing    bool            `json:"standing,omitempty"` // carry over next turn as a draft to confirm
	Then        string          `json:"then,omitempty"`     // task to take up on arrival
	Carried     bool            `json:"carried,omitempty"`  // brought forward from a standing order
	Goal        *[2]int         `json:"goal,omitempty"`     // where a carried-over move is ultimately headed
	Cost        int             `json:"cost,omitempty"`
	Intent      string          `json:"intent,omitempty"`
	Risk        string          `json:"risk"`
	Fallback    string          `json:"fallback"`
	Status      OrderStatus     `json:"status"`
	Version     int             `json:"version"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	CommittedAt *time.Time      `json:"committedAt,omitempty"`
	History     []OrderRevision `json:"history,omitempty"`
}

type OrderRevision struct {
	Version int         `json:"version"`
	At      time.Time   `json:"at"`
	By      string      `json:"by"`
	Status  OrderStatus `json:"status"`
	Summary string      `json:"summary"`
}

// OrderInput is what a player submits.
type OrderInput struct {
	Unit, Task, Dest, Intent, Risk, Fallback, Transport string
	Commit                                              bool
	WithStack                                           bool   // give the same order to the viewer's other units in the hex
	Standing                                            bool   // carry over next turn as a draft to confirm
	Then                                                string // task to take up on arrival
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
		return r.commands(actor, r.memberOrNil(u.AssignedTo))
	case RolePlayer:
		return actor.InPlay() && u.AssignedTo == actor.ID
	}
	return false
}

// CurrentOrder is the unit's live order for this turn, if any.
func (r *Room) CurrentOrder(unitID string) *Order {
	for i := len(r.Orders) - 1; i >= 0; i-- {
		o := r.Orders[i]
		if o.Unit == unitID && o.Turn == r.Turn && o.Status != OrderWithdrawn {
			return o
		}
	}
	return nil
}

type Reach struct {
	Cost int
	Prev [2]int
}

// stepCost is the movement points for unit class cl to cross from a hex over
// side dir, or -1 if it can't.
func (r *Room) stepCost(mod *Module, cl *Class, col, row, dir int) int {
	nc, nr, ok := r.Map.Neighbor(col, row, dir)
	if !ok {
		return -1
	}
	water := mod.TerrainAt(r.Map.TerrainCode(nc, nr)).Water()
	if cl.Domain == "water" {
		if water {
			return 1
		}
		return -1
	}
	return r.moveCost(mod, col, row, dir)
}

// Reach maps every hex unit u can get to this turn to its cheapest cost and
// the previous hex on that path. Hexes holding enemies the players can see
// are off limits; unseen ones are discovered the hard way at resolution.
func (r *Room) Reach(mod *Module, u *Unit) map[[2]int]Reach {
	if u.Aboard != "" {
		return r.disembarkReach(mod, u)
	}
	cl := mod.Class(u.Class)
	blocked := map[[2]int]bool{}
	for _, e := range r.Units {
		if mod.Side(e.Side).Enemy && r.VisibleToPlayers(mod, e) {
			blocked[[2]int{e.Col, e.Row}] = true
		}
	}
	start := [2]int{u.Col, u.Row}
	out := map[[2]int]Reach{start: {Cost: 0, Prev: start}}
	pq := &hexQueue{{start, 0}}
	for pq.Len() > 0 {
		cur := popHex(pq)
		if cur.cost > out[cur.hex].Cost {
			continue
		}
		for dir := range 6 {
			c := r.stepCost(mod, cl, cur.hex[0], cur.hex[1], dir)
			if c < 0 {
				continue
			}
			nc, nr, _ := r.Map.Neighbor(cur.hex[0], cur.hex[1], dir)
			n := [2]int{nc, nr}
			total := cur.cost + c
			if blocked[n] || total > cl.Movement {
				continue
			}
			if prev, ok := out[n]; !ok || total < prev.Cost {
				out[n] = Reach{Cost: total, Prev: cur.hex}
				pushHex(pq, hexItem{n, total})
			}
		}
	}
	return out
}

func routeTo(reach map[[2]int]Reach, start, dest [2]int) [][2]int {
	var path [][2]int
	for h := dest; h != start; h = reach[h].Prev {
		path = append(path, h)
	}
	slices.Reverse(path)
	return path
}

// SaveOrders saves an order and, if asked, the same order for the actor's
// other units in that hex. Each is checked on its own (reach, stacking); if
// any fails, the caller's store update discards them all.
func (r *Room) SaveOrders(actor *Member, mod *Module, in OrderInput, now time.Time) ([]*Order, error) {
	o, err := r.SaveOrder(actor, mod, in, now)
	if err != nil || !in.WithStack {
		return []*Order{o}, err
	}
	out := []*Order{o}
	lead := r.Unit(in.Unit)
	for _, u := range r.Units {
		if u.ID == lead.ID || u.Col != lead.Col || u.Row != lead.Row || !r.CanOrder(mod, actor, u) {
			continue
		}
		next := in
		next.Unit, next.WithStack = u.ID, false
		o, err := r.SaveOrder(actor, mod, next, now)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", u.Name, err)
		}
		out = append(out, o)
	}
	return out, nil
}

// SaveOrder creates or revises the unit's order for this turn.
func (r *Room) SaveOrder(actor *Member, mod *Module, in OrderInput, now time.Time) (*Order, error) {
	if r.Phase != PhasePlanning {
		return nil, bad("orders can only be written while a turn is in planning")
	}
	u := r.Unit(in.Unit)
	if u == nil {
		return nil, bad("pick one of your units")
	}
	if !r.CanOrder(mod, actor, u) {
		return nil, ErrForbidden
	}
	if !r.seesOrder(actor, u) {
		if actor.Role == RoleReferee {
			return nil, bad("%s is held by a player; switch to admin view to see or change its orders", u.Name)
		}
		return nil, bad("%s is out of contact: only the player with it can give it orders", u.Name)
	}
	task := mod.Orders.Task(in.Task)
	if task == nil {
		return nil, bad("pick a task")
	}
	if optionName(mod.Orders.Risks, in.Risk) == "" || optionName(mod.Orders.Fallbacks, in.Fallback) == "" {
		return nil, bad("pick an acceptable risk and a fallback")
	}
	intent := strings.TrimSpace(in.Intent)
	if utf8.RuneCountInString(intent) > maxIntentLen {
		return nil, bad("keep the intent under %d characters", maxIntentLen)
	}
	then := ""
	if in.Then != "" {
		t := mod.Orders.Task(in.Then)
		if t == nil || t.Destination || t.Embark || t.Disembark {
			return nil, bad("the follow-on task must be one that stays put, like holding or digging in")
		}
		then = t.ID
	}
	// Validate everything before touching the stored order.
	var (
		hasDest  bool
		dest     [2]int
		route    [][2]int
		cost     int
		destText = strings.TrimSpace(in.Dest)
	)
	if task.Destination && destText != "" {
		col, row, ok := ParseHexID(destText)
		if !ok || !r.Map.InBounds(col, row) {
			return nil, bad("%q isn't a hex on this map", destText)
		}
		reach := r.Reach(mod, u)
		step, ok := reach[[2]int{col, row}]
		if !ok {
			return nil, bad("%s can't reach %s this turn (movement %d)", u.Name, HexID(col, row), mod.Class(u.Class).Movement)
		}
		if step.Cost == 0 && !task.Disembark {
			return nil, bad("%s is already at %s; pick another hex or order it to hold", u.Name, HexID(col, row))
		}
		if load := r.ProjectedLoad(mod, u.ID, actor); !r.fits(mod, u, [2]int{col, row}, load) {
			return nil, r.stackFullError(mod, u, [2]int{col, row}, load)
		}
		hasDest, dest, cost = true, [2]int{col, row}, step.Cost
		route = routeTo(reach, [2]int{u.Col, u.Row}, dest)
	}
	if task.Destination && !hasDest && in.Commit {
		return nil, bad("%s needs a destination: pick a highlighted hex on the map", task.Name)
	}
	// Passengers can only get off (or sit tight); getting off needs a ride.
	if u.Aboard != "" && !task.Disembark && task.Destination {
		return nil, bad("%s is aboard %s: give it a Disembark order first", u.Name, r.Unit(u.Aboard).Name)
	}
	if task.Disembark && u.Aboard == "" {
		return nil, bad("%s isn't aboard a transport", u.Name)
	}
	transport := ""
	if task.Embark {
		if !mod.Class(u.Class).Embarks {
			return nil, bad("%s can't ride a transport", u.Name)
		}
		reach := r.Reach(mod, u)
		var opt *TransportOption
		for _, t := range r.TransportsFor(mod, u, reach, actor) {
			if t.ID == in.Transport {
				opt = &t
			}
		}
		switch {
		case opt == nil:
			return nil, bad("pick a transport %s can reach this turn", u.Name)
		case opt.Room < u.Strength:
			return nil, bad("%s has room for %d, and %s needs %d", opt.Name, max(0, opt.Room), u.Name, u.Strength)
		}
		col, row, _ := ParseHexID(opt.Board)
		transport = opt.ID
		if col != u.Col || row != u.Row {
			hasDest, dest, cost = true, [2]int{col, row}, opt.Cost
			route = routeTo(reach, [2]int{u.Col, u.Row}, dest)
		}
	}

	o := r.CurrentOrder(u.ID)
	if o == nil {
		o = &Order{ID: newID(), Turn: r.Turn, Unit: u.ID}
		r.Orders = append(r.Orders, o)
	}
	o.Task, o.Intent, o.Risk, o.Fallback, o.Author = task.ID, intent, in.Risk, in.Fallback, actor.ID
	o.Standing, o.Then, o.Carried, o.Goal = in.Standing, then, false, nil
	o.FromCol, o.FromRow, o.HasDest, o.Route, o.Cost, o.Transport = u.Col, u.Row, hasDest, route, cost, transport
	o.DestCol, o.DestRow = dest[0], dest[1]
	o.Status, o.Review, o.ReviewNote, o.ReviewedBy = OrderDraft, "", "", "" // a revision needs a fresh review
	if in.Commit {
		o.Status, o.CommittedAt = OrderCommitted, &now
	}
	o.Version++
	o.UpdatedAt = now.UTC()
	o.History = append(o.History, OrderRevision{Version: o.Version, At: now.UTC(), By: actor.Name, Status: o.Status,
		Summary: r.orderSummary(mod, o)})
	if in.Commit {
		r.logKind(VisPlayers, "", "%s committed an order for %s: %s.", actor.Name, u.Name, r.orderSummary(mod, o))
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
		wasCommitted := o.Status == OrderCommitted
		o.Status, o.UpdatedAt = OrderWithdrawn, now.UTC()
		o.Version++
		o.History = append(o.History, OrderRevision{Version: o.Version, At: now.UTC(), By: actor.Name, Status: o.Status, Summary: "withdrawn"})
		if wasCommitted {
			r.logKind(VisPlayers, "", "%s withdrew the order for %s.", actor.Name, u.Name)
		}
		return nil
	}
	return ErrNotFound
}

func (r *Room) orderSummary(mod *Module, o *Order) string {
	t := mod.Orders.Task(o.Task)
	name := o.Task
	if t != nil {
		name = t.Name
	}
	if t := r.Unit(o.Transport); t != nil {
		return fmt.Sprintf("board %s", t.Name)
	}
	s := strings.ToLower(name)
	if o.HasDest {
		s = fmt.Sprintf("%s to %s (%s)", s, HexID(o.DestCol, o.DestRow), r.Map.SectorAt(o.DestCol, o.DestRow))
	}
	if o.Goal != nil {
		s += fmt.Sprintf(", heading for %s", HexID(o.Goal[0], o.Goal[1]))
	}
	if t := mod.Orders.Task(o.Then); t != nil {
		s += ", then " + strings.ToLower(t.Name)
	}
	if o.Standing {
		s += " (standing)"
	}
	return s
}

// lockOrders freezes committed orders and lets drafts lapse when the turn locks.
func (r *Room) lockOrders(mod *Module, now time.Time) {
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

func popHex(q *hexQueue) hexItem      { return heap.Pop(q).(hexItem) }
func pushHex(q *hexQueue, it hexItem) { heap.Push(q, it) }

// OrderReadiness counts units held by players (not the referee) and how many
// have a committed order this turn, and names the players still to commit.
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
