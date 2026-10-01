package game

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
)

// View is everything one member may see of a room. Building it is where
// hidden-state filtering happens: nothing outside a View reaches a browser.
type View struct {
	Code         string
	CodeDisplay  string
	Name         string
	ScenarioName string
	Summary      string
	Briefing     []string
	RefereeNotes string
	ModuleName   string
	ModuleVer    string
	Scale        Scale
	Turn         int
	Operation    int
	Phase        Phase
	Version      int64

	Base       string // URL prefix for this seat: /r/CODE or /r/CODE/k/KEY
	AccessLink string // absolute link that opens this seat on another device or tab
	TestSeats  []TestSeat

	Me         MemberView
	IsReferee  bool
	NextAction string

	Pending     []MemberView
	Roster      []MemberView
	Players     []MemberView // who units can be assigned to: everyone in play (referee) or self and own players (commander)
	CanAssign   bool
	IsCommander bool
	Commanders  []MemberView
	MyPlayers   []MemberView // a commander's players
	Invites     []InviteView
	Forces      []ForceGroup
	MyUnits     []UnitView
	Assignable  []UnitView
	Log         []LogEntry

	Map    MapView
	Legend []Terrain
	Data   InspectorData

	Clock ClockView

	Tabs        []Tab
	Channels    []Channel     // discussion channels the viewer can read
	Correctable int           // referee: the published turn they can still reopen (0: none)
	PushKey     string        // the server's web push key; empty when push is off
	AI          AIView        // referee: the assistant
	SetAside    int           // orders reopening it would set aside
	Channel     Channel       // the one open on the Messages screen
	Thread      []MessageView // its messages
	CommsActive bool          // comms rules apply in this room
	CommsRules  bool          // the module has comms rules (the referee may switch them off)
	Screen      string

	EventTable     []EventRow // referee
	ThreatLevels   []int
	CommittedCount int // the viewer's committed orders this turn
	AwaitingReview int // a commander's players' orders waiting for review
	StandingDrafts int // carried-over standing orders waiting to be confirmed

	Resolution *ResolutionView // referee, while a turn is being resolved
	Reports    []ReportView
	Replays    map[string]ReplayData // replays linked from the reports shown
	AARs       []OperationRecord
	Stats      OpStats
	AdminMode  bool // referee sees everything
	Blind      bool // referee can't see player orders right now

	OrderSpec   OrderSpec
	MyOrders    []OrderSlot // units the viewer can give orders to
	OtherOrders []OrderView // other friendly orders the viewer may see

	// Referee only.
	Threat       int
	MaxThreat    int
	EventOptions []EventOption
	Proposals    []ProposalView
}

type EventOption struct {
	ID, Title, Category string
	Eligible            bool
	Why                 string // why it can't come up in a random roll right now
}

type ProposalView struct {
	*GameEvent
	RollText string
}

type MemberView struct {
	ID          string    `json:"-"`
	Name        string    `json:"name"`
	Role        Role      `json:"role"`
	Status      Status    `json:"status"`
	RequestedAt time.Time `json:"-"`
	Units       []string  `json:"-"`
	IsMe        bool      `json:"-"`
	ReportsTo   string    `json:"-"` // commander id
	ReportsName string    `json:"-"`
	Test        bool      `json:"-"`
}

// ClockView is the turn clock as the page shows it. Times are rendered in the
// room's zone; the client adds the viewer's local time.
type ClockView struct {
	Deadline     time.Time
	DeadlineText string
	Deferred     bool
	LockedText   string
	Takeover     time.Time
	TakeoverText string
	ScheduleText string
	NextTurn     int
	HasPlan      bool // fights still to settle before the turn can be published
	Unsettled    int

	// Order readiness: units held by players, and how many have committed orders.
	OrdersIn    int
	OrdersTotal int
	AllIn       bool     // every player-held unit has a committed order
	Waiting     []string // players still to commit, with how many units each

	// Schedule form (referee).
	TZ            string
	Times         []string // always 4 slots; blanks are unused
	Days          []bool   // indexed by weekday
	TakeoverHours int
	DeferDefault  string // datetime-local value one day after the deadline
}

type InviteView struct {
	ID, Link, Summary string
	Role              Role
	Uses, MaxUses     int
	Expires           time.Time
	Open              bool
	Joined            []string
}

type TestSeat struct {
	Name   string
	Key    string
	Status Status
}

type ForceGroup struct {
	Side  string
	Name  string
	Units []UnitView
}

type UnitView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ClassName    string `json:"class"`
	Kind         string `json:"kind"`             // group, special, monster, nest
	Traits       string `json:"traits,omitempty"` // shown once the unit is identified
	Abbr         string `json:"abbr"`
	Symbol       string `json:"-"` // counter symbol; empty for an unidentified contact (drawn face down)
	Factors      string `json:"-"` // printed on the counter: attack-armor-move, or a headcount
	Side         string `json:"side"`
	Role         string `json:"-"` // the side's role: player, enemy, or neutral
	Hex          string `json:"hex"`
	Strength     string `json:"strength"`
	Hidden       bool   `json:"hidden,omitempty"`     // referee view: players don't know about it
	Contact      bool   `json:"contact,omitempty"`    // player view: detected but unidentified
	Ghost        bool   `json:"ghost,omitempty"`      // player view: out of sight, shown where last seen
	Stale        bool   `json:"stale,omitempty"`      // off the net: shown where it last reported from
	LocalSight   bool   `json:"localSight,omitempty"` // seen only by the viewer's units off the net
	SeenTurn     int    `json:"seenTurn,omitempty"`
	AssignedTo   string `json:"-"`
	AssignedName string `json:"commander,omitempty"`
	Mine         bool   `json:"mine,omitempty"`
	Stats        string `json:"stats,omitempty"`
	Level        string `json:"level,omitempty"` // experience, friendly units
	LevelRank    int    `json:"-"`               // 0 green .. 3 elite
	Chevrons     string `json:"-"`               // path data for experience chevrons on the counter
	XPDice       int    `json:"xpDice,omitempty"`
	Supply       string `json:"supply,omitempty"`
	Sector       string `json:"-"`
	AmmoText     string `json:"-"`               // ammunition left, friendly units
	OrderStatus  string `json:"-"`               // this turn's order, if the viewer may see it
	Assignable   bool   `json:"-"`               // the viewer may reassign it
	Radio        string `json:"radio,omitempty"` // friendly units under comms rules: "up" or "down"
	RadioText    string `json:"radioText,omitempty"`
	RadioSetting string `json:"-"`                // referee's override
	Aboard       string `json:"aboard,omitempty"` // name of the transport it's riding
}

type FeatureView struct {
	ID     string `json:"id,omitempty"` // referee only, for editing
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Hex    string `json:"hex"`
	Note   string `json:"note,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// BuildView filters a room for the given member.
func BuildView(r *Room, sc *Scenario, mod *Module, me *Member) *View {
	ref := me.Role == RoleReferee
	v := &View{
		Code: r.Code, CodeDisplay: DisplayCode(r.Code), Name: r.Name,
		ScenarioName: sc.Name, Summary: sc.Summary, Briefing: r.Briefing,
		ModuleName: mod.Name, ModuleVer: r.ModuleVersion, Scale: mod.Scale,
		Turn: r.Turn, Operation: max(r.Operation, 1), Phase: r.Phase, Version: r.Version, IsReferee: ref,
		Me: memberView(r, me, me),
	}
	if ref {
		v.RefereeNotes = r.RefereeNotes
		for _, m := range r.Members {
			if m.TestKey != "" && (m.Status == StatusApproved || m.Status == StatusPending) {
				v.TestSeats = append(v.TestSeats, TestSeat{Name: m.Name, Key: m.TestKey, Status: m.Status})
			}
		}
	}

	names := map[string]string{}
	for _, m := range r.Members {
		names[m.ID] = m.Name
		switch {
		case m.Status == StatusPending && ref:
			v.Pending = append(v.Pending, memberView(r, m, me))
		case m.Status == StatusApproved:
			v.Roster = append(v.Roster, memberView(r, m, me))
			if m.Role == RoleCommander {
				v.Commanders = append(v.Commanders, memberView(r, m, me))
			}
			if m.InPlay() && (ref || r.commands(me, m) && me.Role == RoleCommander) {
				v.Players = append(v.Players, memberView(r, m, me))
			}
			if me.Role == RoleCommander && m.ReportsTo == me.ID && m.InPlay() {
				v.MyPlayers = append(v.MyPlayers, memberView(r, m, me))
			}
		}
	}
	v.IsCommander = me.Role == RoleCommander
	v.CanAssign = ref || v.IsCommander
	if ref {
		now := time.Now()
		for i := len(r.Invites) - 1; i >= 0; i-- {
			inv := r.Invites[i]
			if !inv.Open(now) && now.Sub(inv.CreatedAt) > 2*inviteLifetime {
				continue // old history
			}
			iv := InviteView{ID: inv.ID, Link: "/i/" + r.Code + "/" + inv.Token, Summary: r.InviteSummary(inv),
				Role: inv.Role, Uses: inv.Uses, MaxUses: inv.MaxUses, Expires: inv.ExpiresAt, Open: inv.Open(now)}
			for _, id := range inv.Joined {
				if m := r.Member(id); m != nil {
					iv.Joined = append(iv.Joined, m.Name)
				}
			}
			v.Invites = append(v.Invites, iv)
		}
	}

	// Units: friendly units are visible to all approved members; others only
	// when known, and enemies only as unidentified contacts.
	groups := map[string]*ForceGroup{}
	var visible []UnitView
	local := r.localObservers(mod, me)
	for _, u := range r.Units {
		side := mod.Side(u.Side)
		var uv UnitView
		switch {
		case ref || r.VisibleToPlayers(mod, u):
			uv = unitView(u, mod, ref)
			if !ref && side.Enemy {
				uv.ID = contactID(r, u)
			}
		case !side.Playable && len(local) > 0 && r.inSight(mod, u, local):
			// Seen only by the viewer's own units off the net.
			uv = unitView(u, mod, false)
			uv.LocalSight = true
			if side.Enemy {
				uv.ID = contactID(r, u)
			}
		case side.Enemy && r.Sightings[u.ID] != (Sighting{}):
			uv = ghostView(r, u, mod) // out of sight: where it was last seen
		default:
			continue
		}
		uv.AssignedName = names[u.AssignedTo]
		if t := r.Unit(u.Aboard); t != nil {
			uv.Aboard = t.Name
		}
		uv.Mine = u.AssignedTo != "" && u.AssignedTo == me.ID
		at, stale := r.reportedAt(u, me)
		if stale {
			uv.Hex, uv.Stale, uv.SeenTurn = HexID(at[0], at[1]), true, u.LastContact
		}
		visible = append(visible, uv)
		if uv.Mine {
			v.MyUnits = append(v.MyUnits, uv)
		}
		uv.Sector = r.Map.SectorAt(at[0], at[1])
		if uv.Ghost {
			sg := r.Sightings[u.ID]
			uv.Sector = r.Map.SectorAt(sg.Col, sg.Row)
		}
		if side.Playable {
			uv.AmmoText = ammoText(mod.Class(u.Class), u)
			if r.CommsActive(mod) || u.OutOfContact {
				uv.Radio, uv.RadioText = "up", r.radioText(u)
				if u.OutOfContact {
					uv.Radio = "down"
				}
				if ref {
					uv.RadioSetting = u.Radio
				}
			}
		}
		if side.Playable && (ref || me.Role == RoleCommander && r.commands(me, r.memberOrNil(u.AssignedTo))) {
			uv.Assignable = true
			v.Assignable = append(v.Assignable, uv)
		}
		g := groups[u.Side]
		if g == nil {
			g = &ForceGroup{Side: u.Side, Name: side.Name}
			groups[u.Side] = g
		}
		g.Units = append(g.Units, uv)
	}
	for _, s := range mod.Sides {
		if g := groups[s.ID]; g != nil {
			v.Forces = append(v.Forces, *g)
		}
	}

	for i := len(r.Log) - 1; i >= 0 && len(v.Log) < 40; i-- {
		if e := r.Log[i]; r.seesLog(me, e) {
			v.Log = append(v.Log, e)
		}
	}

	v.Clock = buildClock(r)
	v.Map, v.Legend, v.Data = buildMap(r, mod, me, visible, local)
	addOrders(v, r, mod, me)
	addResolution(v, r, mod, me)
	v.Channels = r.Channels(me)
	if ref {
		addRefereeEvents(v, r, mod)
	}
	v.AdminMode, v.Blind = me.AdminMode, r.Blind(me)
	v.CommsActive, v.CommsRules = r.CommsActive(mod), mod.Comms != nil
	v.NextAction = nextAction(v)
	if ref {
		v.EventTable = eventTable(r, mod)
		for t := MinThreat; t <= MaxThreat; t++ {
			v.ThreatLevels = append(v.ThreatLevels, t)
		}
	}
	return v
}

func memberView(r *Room, m, me *Member) MemberView {
	mv := MemberView{ID: m.ID, Name: m.Name, Role: m.Role, Status: m.Status, RequestedAt: m.RequestedAt, IsMe: m.ID == me.ID,
		ReportsTo: m.ReportsTo, Test: m.TestKey != ""}
	if c := r.Member(m.ReportsTo); c != nil {
		mv.ReportsName = c.Name
	}
	for _, u := range r.Units {
		if u.AssignedTo == m.ID {
			mv.Units = append(mv.Units, u.Name)
		}
	}
	return mv
}

func sideRole(s *Side) string {
	switch {
	case s.Playable:
		return "player"
	case s.Enemy:
		return "enemy"
	}
	return "neutral"
}

func unitView(u *Unit, mod *Module, ref bool) UnitView {
	cl := mod.Class(u.Class)
	side := mod.Side(u.Side)
	kind := cl.Kind
	if kind == "" {
		kind = "group"
	}
	uv := UnitView{
		ID: u.ID, Name: u.Name, ClassName: cl.Name, Abbr: cl.Abbr, Side: u.Side, Role: sideRole(side), Kind: kind, Symbol: cl.Symbol,
		Factors: fmt.Sprintf("%d-%d-%d", cl.Attack, cl.Armor, cl.Movement),
		Hex:     HexID(u.Col, u.Row), AssignedTo: u.AssignedTo, Hidden: ref && !u.Known,
	}
	switch {
	case side.Playable:
		lvl := ExperienceOf(mod, u)
		uv.Level, uv.XPDice = lvl.Name, lvl.Dice
		uv.LevelRank = slices.IndexFunc(mod.Experience.Levels, func(l XPLevel) bool { return l.Name == lvl.Name })
		for i := range max(0, uv.LevelRank) {
			uv.Chevrons += fmt.Sprintf("M-8.1 %.1fl1.2-1.1 1.2 1.1", -0.6+float64(i)*1.5)
		}
		uv.Strength = fmt.Sprintf("%d personnel", u.Strength)
		uv.Stats = fmt.Sprintf("Move %d · Attack %d · Range %d · Armor %d", cl.Movement, cl.Attack, cl.Range, cl.Armor)
		uv.Supply = supplyText(cl)
	case ref:
		switch {
		case !side.Enemy: // neutral: civilians
			uv.Strength = fmt.Sprintf("%d people", u.Strength)
			uv.Factors = fmt.Sprint(u.Strength)
		case cl.IsGroup():
			uv.Strength = fmt.Sprintf("%d infected", u.Strength)
		default:
			uv.Strength = kind
		}
		if side.Enemy {
			uv.Stats = fmt.Sprintf("Move %d · Attack %d · Armor %d", cl.Movement, cl.Attack, cl.Armor)
			uv.Traits = traitsText(cl, mod)
		}
	case side.Enemy:
		// Players see a contact: its signature, not its identity.
		uv.Contact, uv.Symbol, uv.Factors = true, "", ""
		uv.Name = "Contact at " + uv.Hex
		uv.ClassName, uv.Abbr = "Unidentified contact", "?"
		if cl.Contact != nil {
			uv.ClassName, uv.Abbr = cl.Contact.Label, cl.Contact.Abbr
		}
		if cl.IsGroup() {
			uv.Strength = estimate(u.Strength)
		} else {
			uv.Strength = "size unknown"
		}
	default:
		uv.Strength = estimate(u.Strength) + " people"
		uv.Factors = "~" + fmt.Sprint((u.Strength+5)/10*10)
	}
	return uv
}

func traitsText(cl *Class, mod *Module) string {
	var parts []string
	for _, id := range cl.Traits {
		if t := mod.Trait(id); t != nil {
			parts = append(parts, t.Name+": "+t.Text)
		}
	}
	return strings.Join(parts, " ")
}

func supplyText(cl *Class) string {
	var parts []string
	for k, n := range cl.Ammo {
		parts = append(parts, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(parts)
	if cl.Capacity > 0 {
		parts = append(parts, fmt.Sprintf("carries %d", cl.Capacity))
	}
	return strings.Join(parts, " · ")
}

// estimate turns an exact count into the kind of range a scout would report.
func estimate(n int) string {
	lo := max(10, int(float64(n)*0.5)/10*10)
	hi := int(math.Ceil(float64(n)*1.5/10)) * 10
	return fmt.Sprintf("est. %d–%d", lo, hi)
}

func stackRank(u UnitView) int {
	switch {
	case u.Mine:
		return 0
	case u.Role == "player":
		return 1
	case u.Role == "enemy":
		return 2
	default:
		return 3
	}
}

func nextAction(v *View) string {
	if v.Phase == PhaseResolving {
		if !v.IsReferee {
			return fmt.Sprintf("Turn %d is locked. The referee is resolving it; results will appear on the Reports screen.", v.Turn)
		}
		msg := fmt.Sprintf("Turn %d is locked. Settle %s on the Turn screen, then publish the turn.", v.Turn, plural(v.Clock.Unsettled, "fight", "fights"))
		if v.Clock.Unsettled == 0 {
			msg = fmt.Sprintf("Turn %d is resolved. Check the moves on the Turn screen, then publish it and open turn %d.", v.Turn, v.Turn+1)
		}
		if v.Clock.TakeoverText != "" {
			msg += " If you don't, the AI takes over " + v.Clock.TakeoverText + "."
		}
		return msg
	}
	if v.Phase == PhasePlanning && len(v.MyOrders) > 0 && !v.IsReferee {
		done := 0
		reachable := 0
		for _, o := range v.MyOrders {
			if o.NoContact {
				continue
			}
			reachable++
			if o.Order != nil && o.Order.Status == OrderCommitted {
				done++
			}
		}
		if done < reachable {
			return fmt.Sprintf("Give your units orders: %d of %d committed. Orders lock %s.", done, reachable, v.Clock.DeadlineText)
		}
		return fmt.Sprintf("All %d of your units have committed orders. You can revise them until %s.", done, v.Clock.DeadlineText)
	}
	if v.IsReferee {
		unassigned := 0
		for _, u := range v.Assignable {
			if u.AssignedTo == "" {
				unassigned++
			}
		}
		switch {
		case len(v.Pending) > 0:
			return fmt.Sprintf("%s waiting for your decision.", plural(len(v.Pending), "access request", "access requests"))
		case v.Phase == PhasePlanning && v.Clock.AllIn:
			return fmt.Sprintf("Every player has committed orders (%d of %d units). Lock turn %d now, or wait for %s.",
				v.Clock.OrdersIn, v.Clock.OrdersTotal, v.Turn, v.Clock.DeadlineText)
		case len(v.Proposals) > 0:
			return fmt.Sprintf("Review %s: apply or discard.", plural(len(v.Proposals), "proposed event", "proposed events"))
		case len(v.Players) == 0 && v.Phase == PhaseSetup:
			return "Share room code " + v.CodeDisplay + " with your players. Their requests will appear on the People screen."
		case v.Phase == PhasePlanning:
			return fmt.Sprintf("Turn %d planning is open: %d of %d player units have committed orders. Orders lock %s.",
				v.Turn, v.Clock.OrdersIn, v.Clock.OrdersTotal, v.Clock.DeadlineText)
		case unassigned > 0:
			return fmt.Sprintf("Assign forces: %s still held by the referee.", plural(unassigned, "unit is", "units are"))
		default:
			return "All forces are assigned. Start the operation when you're ready."
		}
	}
	if v.IsCommander {
		pool := len(v.Assignable)
		switch {
		case pool == 0:
			return "You're a commander. Waiting for the referee to place units under your command."
		case len(v.MyPlayers) == 0:
			return fmt.Sprintf("You command %s. Ask the referee to put players under you, then hand units out in Assign forces.", plural(pool, "unit", "units"))
		default:
			return fmt.Sprintf("You command %s and lead %s. Hand units out in Assign forces.", plural(pool, "unit", "units"), plural(len(v.MyPlayers), "player", "players"))
		}
	}
	if len(v.MyUnits) == 0 {
		return "Waiting for your force to be assigned. Study the map while you wait."
	}
	if v.Phase == PhaseSetup {
		return fmt.Sprintf("You command %s. Orders open when the operation starts.", plural(len(v.MyUnits), "unit", "units"))
	}
	return fmt.Sprintf("You command %s. Orders lock %s. (Order entry isn't built yet.)", plural(len(v.MyUnits), "unit", "units"), v.Clock.DeadlineText)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

var eventCategoryOrder = []string{"horde", "wave", "special", "brood", "monster", "respite"}

func addRefereeEvents(v *View, r *Room, mod *Module) {
	v.Threat, v.MaxThreat = r.Threat, MaxThreat
	for _, cat := range eventCategoryOrder {
		for i := range mod.Events {
			d := &mod.Events[i]
			if d.Category != cat {
				continue
			}
			o := EventOption{ID: d.ID, Title: d.Title, Category: d.Category, Eligible: r.Eligible(d)}
			switch {
			case d.MinThreat > 0 && r.Threat < d.MinThreat:
				o.Why = fmt.Sprintf("threat %d+", d.MinThreat)
			case d.MaxThreat > 0 && r.Threat > d.MaxThreat:
				o.Why = fmt.Sprintf("threat %d or lower", d.MaxThreat)
			case d.Requires != "" && !r.hasClass(d.Requires):
				o.Why = "needs a " + strings.ToLower(mod.Class(d.Requires).Name)
			case d.Unique != "" && r.hasClass(d.Unique):
				o.Why = "already on the map"
			}
			v.EventOptions = append(v.EventOptions, o)
		}
	}
	for _, e := range r.Events {
		if e.Status != EventProposed {
			continue
		}
		pv := ProposalView{GameEvent: e, RollText: "Chosen by the referee"}
		switch {
		case !e.Chosen:
			pv.RollText = fmt.Sprintf("Rolled %d on 1–%d", e.Roll, e.Of)
		}
		pv.RollText += fmt.Sprintf(" at threat %d", e.Threat)
		v.Proposals = append(v.Proposals, pv)
		for _, ef := range e.Effects {
			if !ef.located() {
				continue
			}
			col, row := ef.Col, ef.Row
			if ef.Op == "drift" {
				col, row = ef.ToCol, ef.ToRow
			}
			x, y := HexCenter(col, row)
			v.Map.Proposed = append(v.Map.Proposed, ProposedMark{X: x, Y: y, Hex: HexID(col, row),
				Label: e.Title + ": " + ef.Text})
		}
	}
}

var WeekdayShort = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

func buildClock(r *Room) ClockView {
	sc := r.Schedule
	c := ClockView{Deadline: r.Deadline, Deferred: r.Deferred, NextTurn: r.Turn + 1, HasPlan: r.Unsettled() > 0, Unsettled: r.Unsettled(),
		ScheduleText: fmt.Sprintf("Orders lock at %s, %s.", joinTimes(sc), daysText(sc.Days)),
		TZ:           sc.TimeZone, TakeoverHours: sc.Takeover, Days: make([]bool, 7)}
	if r.Phase == PhasePlanning {
		c.OrdersIn, c.OrdersTotal, c.Waiting = r.OrderReadiness()
		c.AllIn = c.OrdersTotal > 0 && c.OrdersIn == c.OrdersTotal
	}
	if !r.Deadline.IsZero() {
		c.DeadlineText = r.FormatInZone(r.Deadline)
		c.DeferDefault = r.Deadline.Add(24 * time.Hour).In(sc.Location()).Format("2006-01-02T15:04")
	}
	if !r.LockedAt.IsZero() {
		c.LockedText = r.FormatInZone(r.LockedAt)
	}
	if t := r.TakeoverAt(); !t.IsZero() {
		c.Takeover, c.TakeoverText = t, r.FormatInZone(t)
	}
	c.Times = append(slices.Clone(sc.Times), make([]string, max(0, 4-len(sc.Times)))...)
	for d := range 7 {
		c.Days[d] = len(sc.Days) == 0 || slices.Contains(sc.Days, d)
	}
	return c
}

// TakeoverLabel describes an AI takeover setting for the schedule form.
func TakeoverLabel(h int) string {
	switch h {
	case -1:
		return "Never: the turn waits for me"
	case 0:
		return "Immediately at lock"
	}
	return fmt.Sprintf("%s after lock", plural(h, "hour", "hours"))
}

// contactID gives players a stable handle for a contact that doesn't leak
// the unit's real id (which can reveal what it is).
func contactID(r *Room, u *Unit) string {
	sum := sha256.Sum256([]byte(r.Code + "/" + u.ID))
	return "c" + hex.EncodeToString(sum[:5])
}

// ghostView is a contact players lost sight of, at its last known hex.
func ghostView(r *Room, u *Unit, mod *Module) UnitView {
	sg := r.Sightings[u.ID]
	cl := mod.Class(u.Class)
	kind := cl.Kind
	if kind == "" {
		kind = "group"
	}
	return UnitView{ID: contactID(r, u), Name: "Lost contact", ClassName: "Last seen here", Abbr: "?", Side: u.Side, Role: "enemy",
		Kind: kind, Hex: HexID(sg.Col, sg.Row), Contact: true, Ghost: true, SeenTurn: sg.Turn,
		Strength: fmt.Sprintf("last seen turn %d, %s", sg.Turn, sg.At.Format("Jan 2 15:04 UTC"))}
}

// Tab is one screen of the room.
type Tab struct {
	ID, Label, Href, Badge string
	Calm                   bool // the badge is good news, not a task
	Active                 bool
}

// screenOrder lists the screens each kind of member gets, in tab order.
var screenOrder = map[bool][]string{
	true:  {"map", "turn", "events", "forces", "people", "messages", "reports", "activity", "room"},
	false: {"map", "orders", "forces", "messages", "reports", "activity", "people"},
}

var screenLabels = map[string]string{"map": "Map", "turn": "Turn", "events": "Random events", "forces": "Forces",
	"people": "People", "reports": "Reports", "activity": "Activity", "messages": "Messages", "room": "Room", "orders": "Orders"}

// HasScreen reports whether the viewer has a screen by this name.
func (v *View) HasScreen(screen string) bool {
	return slices.Contains(screenOrder[v.IsReferee], screen)
}

// SetScreen marks the current screen and builds the tab strip. Call it after
// Base is set.
func (v *View) SetScreen(screen string) {
	v.Screen = screen
	n := func(k int) string {
		if k <= 0 {
			return ""
		}
		return fmt.Sprint(k)
	}
	v.Tabs = nil
	for _, id := range screenOrder[v.IsReferee] {
		t := Tab{ID: id, Label: screenLabels[id], Href: v.Base + "/" + id, Active: id == screen}
		if id == "map" {
			t.Href = v.Base
		}
		switch id {
		case "turn":
			switch {
			case v.Phase == PhaseResolving && v.Clock.Unsettled > 0:
				t.Badge = fmt.Sprint(v.Clock.Unsettled)
			case v.Phase == PhaseResolving, v.Clock.AllIn:
				t.Badge, t.Calm = "✓", true
			}
		case "events":
			t.Badge = n(len(v.Proposals))
		case "people":
			t.Badge = n(len(v.Pending))
		case "messages":
			unread := 0
			for _, c := range v.Channels {
				if !(screen == "messages" && c.ID == v.Channel.ID) {
					unread += c.Unread
				}
			}
			t.Badge = n(unread)
		case "orders":
			if v.Phase == PhasePlanning {
				reachable := 0
				for _, o := range v.MyOrders {
					if !o.NoContact {
						reachable++
					}
				}
				t.Badge = n(reachable - v.CommittedCount + v.AwaitingReview)
			}
		}
		v.Tabs = append(v.Tabs, t)
	}
}

func ammoText(cl *Class, u *Unit) string {
	if len(cl.Ammo) == 0 {
		return ""
	}
	keys := make([]string, 0, len(cl.Ammo))
	for k := range cl.Ammo {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d/%d", k, u.ammo(cl, k), cl.Ammo[k]))
	}
	return strings.Join(parts, " · ")
}
