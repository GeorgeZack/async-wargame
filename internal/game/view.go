package game

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// View is everything one member may see of a room. Building it is where
// hidden-state filtering happens: nothing outside a View reaches a browser.
type View struct {
	ImageAI      ImageAIView // referee-only image service details
	BriefingArt  VignetteView
	Code         string
	CodeDisplay  string
	Name         string
	ScenarioName string
	Summary      string
	Objectives   []string
	Briefing     []string
	RefereeNotes string
	ModuleName   string
	ModuleVer    string
	ScaleText    string // "1 hex = 50 m · 1 turn = 10 minutes"
	Board        Board
	MyCards      []*UnitCard // a player's own characters, with their briefs
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
	Channel     Channel       // the one open under Talk
	Thread      []MessageView // its messages
	Screen      string

	EventTable     []EventRow // referee
	ThreatLevels   []int
	CommittedCount int // the viewer's committed orders this turn
	AwaitingReview int // a commander's players' orders waiting for review

	Desk       *DeskView       // referee, while a turn is being adjudicated
	Intentions []IntentionView // referee: what each enemy unit means to do this turn
	Reports    []ReportView
	AARs       []OperationRecord
	AdminMode  bool // referee sees everything
	Blind      bool // referee can't see player orders right now
	Frozen     bool // players: the board as it stood at the lock
	Watched    bool // players: the referee can see this turn's orders (admin view)

	Facts       []FactView // the viewer can read, newest first
	OpenFacts   int        // referee: proposals waiting
	FactUnits   []UnitView // units the viewer may propose facts about
	SquadOrders []SquadOrderSlot
	MyOrders    []OrderSlot // units the viewer can give orders to
	CheckIns    []CheckIn   // referee, while orders are open: each player unit's status
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
	ScheduleText string
	NextTurn     int
	Unwritten    int // locked orders without an outcome yet

	// Order readiness: units held by players, and how many have committed orders.
	OrdersIn    int
	OrdersTotal int
	AllIn       bool     // every player-held unit has a committed order
	Waiting     []string // players still to commit, with how many units each

	// Schedule form (referee).
	TZ           string
	Times        []string // always 4 slots; blanks are unused
	Days         []bool   // indexed by weekday
	DeferDefault string   // datetime-local value one day after the deadline
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
	Formation    string   `json:"formation,omitempty"`
	Detached     bool     `json:"detached,omitempty"`
	Members      []string `json:"members,omitempty"`
	Card         string   `json:"card,omitempty"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ClassName    string   `json:"class"`
	Kind         string   `json:"kind"` // group, soldier, special, monster
	Abbr         string   `json:"abbr"`
	Symbol       string   `json:"-"` // counter symbol; empty for an unidentified contact (drawn face down)
	Factors      string   `json:"-"` // printed on the counter: a headcount
	Side         string   `json:"side"`
	Role         string   `json:"-"` // the side's role: player, enemy, or neutral
	X            float64  `json:"x"`
	Y            float64  `json:"y"`
	Where        string   `json:"where,omitempty"` // grid reference and nearest place
	Strength     string   `json:"strength"`
	Status       string   `json:"status,omitempty"`
	Bonus        string   `json:"bonus,omitempty"` // what its rolls start from
	Kit          string   `json:"kit,omitempty"`
	Description  string   `json:"description,omitempty"`
	Hidden       bool     `json:"hidden,omitempty"`  // referee view: players don't know about it
	Contact      bool     `json:"contact,omitempty"` // player view: seen but not identified
	Stale        bool     `json:"stale,omitempty"`   // out of contact: shown where it last called in
	SeenTurn     int      `json:"seenTurn,omitempty"`
	AssignedTo   string   `json:"-"`
	AssignedName string   `json:"commander,omitempty"`
	Mine         bool     `json:"mine,omitempty"`
	OrderStatus  string   `json:"-"`               // this turn's order, if the viewer may see it
	Assignable   bool     `json:"-"`               // the viewer may reassign it
	Radio        string   `json:"radio,omitempty"` // friendly units: "up" or "down"
	RadioText    string   `json:"radioText,omitempty"`
}

type FeatureView struct {
	ID     string `json:"id,omitempty"` // referee only, for editing
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Where  string `json:"where,omitempty"`
	Note   string `json:"note,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// BuildView filters a room for the given member.
func BuildView(r *Room, sc *Scenario, mod *Module, me *Member) *View {
	ref := me.Role == RoleReferee
	v := &View{
		Code: r.Code, CodeDisplay: DisplayCode(r.Code), Name: r.Name,
		ScenarioName: sc.Name, Summary: sc.Summary, Briefing: r.Briefing, Objectives: r.Objectives,
		ModuleName: mod.Name, ModuleVer: r.ModuleVersion, ScaleText: r.Board.ScaleText(), Board: r.Board,
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

	// Units: friendly units are visible to every member; others once the
	// referee reveals them, and as contacts until identified. While the
	// referee adjudicates, players see the board as it stood at the lock.
	groups := map[string]*ForceGroup{}
	var visible []UnitView
	units := r.Units
	if !ref && r.Phase == PhaseResolving && r.LockedUnits != nil {
		units, v.Frozen = r.LockedUnits, true
	}
	for _, u := range units {
		side := mod.Side(u.Side)
		if side == nil || mod.Class(u.Class) == nil || !r.KnownBy(mod, u, me) {
			continue
		}
		uv := unitView(r, u, mod, me)
		uv.AssignedName = names[u.AssignedTo]
		uv.Mine = u.AssignedTo != "" && u.AssignedTo == me.ID
		if at, stale := r.reportedAt(u, me); stale {
			uv.X, uv.Y, uv.Stale, uv.SeenTurn = at.X, at.Y, true, u.LastContact
			uv.Where = r.Where(at, ref)
		}
		if side.Playable {
			uv.Radio, uv.RadioText = "up", r.radioText(u)
			if u.OutOfContact {
				uv.Radio = "down"
			}
		}
		visible = append(visible, uv)
		if uv.Mine {
			v.MyUnits = append(v.MyUnits, uv)
		}
		if side.Playable && (ref || me.Role == RoleCommander && r.commands(me, r.memberOrNil(u.AssignedTo))) {
			uv.Assignable = true
			v.Assignable = append(v.Assignable, uv)
		}
		if side.Playable && (ref || uv.Mine || me.Role != RoleReferee) {
			v.FactUnits = append(v.FactUnits, uv)
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
	v.Map, v.Legend, v.Data = buildMap(r, mod, me, visible)
	v.Data.Plans = r.PlansFor(me)
	addOrders(v, r, mod, me)
	markOrderStatus(v)
	v.BriefingArt = r.vignetteView(max(r.Operation, 1), 0, v.IsReferee)
	addReports(v, r, me)
	v.Channels = r.Channels(me)
	v.Facts = r.FactsFor(me, "")
	if ref {
		addRefereeEvents(v, r, mod)
		v.AI = assistantView(r)

		v.Desk = buildDesk(r, mod, me, v.AI)
		if r.intentionsOpen() == nil {
			v.Intentions = r.intentionViews(mod, me)
		}
		for _, f := range r.Facts {
			if f.Status == FactProposed {
				v.OpenFacts++
			}
		}
		if r.CanCorrect() {
			v.Correctable, v.SetAside = r.Undo.Turn, r.NewTurnOrders()
		}
	}
	v.AdminMode, v.Blind = me.AdminMode, r.Blind(me)
	if ref := r.Referee(); !v.IsReferee && ref != nil && ref.AdminMode && r.Phase == PhasePlanning {
		v.Watched = true
	}
	v.NextAction = nextAction(v)
	if !ref {
		for _, uv := range v.MyUnits {
			if c, err := BuildUnitCard(r, mod, me, uv.ID); err == nil {
				v.MyCards = append(v.MyCards, c)
			}
		}
	}
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

func unitView(r *Room, u *Unit, mod *Module, me *Member) UnitView {
	ref := me.Role == RoleReferee
	cl := mod.Class(u.Class)
	side := mod.Side(u.Side)
	kind := cl.Kind
	if kind == "" {
		kind = "group"
	}
	uv := UnitView{
		ID: u.ID, Name: u.Name, ClassName: cl.Name, Abbr: cl.Abbr, Side: u.Side, Role: sideRole(side), Kind: kind, Symbol: cl.Symbol,
		X: u.X, Y: u.Y, Where: r.Where(u.Pos(), ref), AssignedTo: u.AssignedTo, Hidden: ref && !u.Known && !side.Playable, Status: u.Status,
		Description: cl.Description,
	}
	headcount := func(noun string) string {
		if !cl.IsGroup() && u.Strength <= 1 {
			if u.Strength <= 0 {
				return "down"
			}
			return ""
		}
		return plural(u.Strength, noun, noun+"s")
	}
	switch {
	case side.Playable || ref || u.Identified:
		if ref || side.Playable {
			uv.Formation, uv.Detached = u.Formation, u.Detached
		}
		uv.Bonus, uv.Kit = bonusText(mod, u), KitOf(mod, u)
		switch {
		case side.Playable && kind == "soldier":
			uv.Strength = headcount("marine")
		case side.Enemy:
			uv.Strength = headcount("infected")
		default:
			uv.Strength = headcount("person")
		}
		if cl.IsGroup() {
			uv.Factors = fmt.Sprint(u.Strength)
		}
		if !ref && !side.Playable {
			uv.ID = contactID(r, u)
		}
	default:
		// Players see a contact: its signature, not its identity.
		uv.ID, uv.Contact, uv.Symbol = contactID(r, u), true, ""
		uv.Name = "Contact"
		if uv.Where != "" {
			uv.Name += " " + strings.TrimPrefix(strings.TrimPrefix(uv.Where, "at "), "near ")
		}
		uv.ClassName, uv.Abbr, uv.Description, uv.Status = "Unidentified contact", "?", "", ""
		if cl.Contact != nil {
			uv.ClassName, uv.Abbr = cl.Contact.Label, cl.Contact.Abbr
		}
		uv.Strength = "unknown"
		if cl.IsGroup() {
			uv.Strength = estimate(u.Strength)
		}
	}
	return uv
}

// estimate turns an exact count into the kind of range a scout would report.
func estimate(n int) string {
	lo := max(1, int(float64(n)*0.6))
	hi := max(lo+1, int(math.Ceil(float64(n)*1.5)))
	return fmt.Sprintf("about %d–%d", lo, hi)
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
			return fmt.Sprintf("Turn %d is locked. The referee is working out what happens; the report will appear under Situation.", v.Turn)
		}
		if v.Desk != nil && v.Desk.Unwritten > 0 {
			return fmt.Sprintf("Turn %d is locked. Work through %s on the Desk: roll where it's in doubt, write what happened, then publish.",
				v.Turn, plural(v.Desk.Unwritten, "order", "orders"))
		}
		return fmt.Sprintf("Turn %d is ready. Check the outcomes on the Desk, then publish it and open turn %d.", v.Turn, v.Turn+1)
	}
	if v.Phase == PhasePlanning && len(v.MyOrders) > 0 && !v.IsReferee {
		done, reachable := 0, 0
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
			return fmt.Sprintf("Say what your marine does: %d of %d orders committed. Orders lock %s.", done, reachable, v.Clock.DeadlineText)
		}
		return fmt.Sprintf("Your orders are in. You can change them until %s.", v.Clock.DeadlineText)
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
			return fmt.Sprintf("%s waiting on the People screen.", plural(len(v.Pending), "access request", "access requests"))
		case v.OpenFacts > 0:
			return fmt.Sprintf("%s on the Desk.", capitalize(plural(v.OpenFacts, "proposed fact is waiting", "proposed facts are waiting")))
		case v.Phase == PhasePlanning && v.Clock.AllIn:
			return fmt.Sprintf("Every order is in (%d of %d). Lock turn %d now, or wait for %s.",
				v.Clock.OrdersIn, v.Clock.OrdersTotal, v.Turn, v.Clock.DeadlineText)
		case len(v.Proposals) > 0:
			return fmt.Sprintf("Review %s on the Desk: apply or discard.", plural(len(v.Proposals), "proposed event", "proposed events"))
		case len(v.Players) == 0 && v.Phase == PhaseSetup:
			return "Share room code " + v.CodeDisplay + " or an invite link with your players. Their requests will appear on the People screen."
		case v.Phase == PhasePlanning:
			return fmt.Sprintf("Turn %d: %d of %d orders committed. Orders lock %s.",
				v.Turn, v.Clock.OrdersIn, v.Clock.OrdersTotal, v.Clock.DeadlineText)
		case unassigned > 0:
			return fmt.Sprintf("Hand out the marines: %s still held by the referee.", plural(unassigned, "unit is", "units are"))
		default:
			return "Everyone has a marine. Check the map, scale, and briefs on Setup, then start the operation."
		}
	}
	if len(v.MyUnits) == 0 && !v.IsCommander {
		return "Waiting for the referee to give you a marine. Read the briefing and the map while you wait."
	}
	if v.Phase == PhaseSetup {
		return "Orders open when the operation starts. Read your character and private brief, and propose anything you'd like to be true."
	}
	return fmt.Sprintf("Orders lock %s.", v.Clock.DeadlineText)
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
			x, y := ef.X, ef.Y
			if ef.Op == "drift" {
				x, y = ef.ToX, ef.ToY
			}
			v.Map.Proposed = append(v.Map.Proposed, ProposedMark{X: x, Y: y, Label: e.Title + ": " + ef.Text})
		}
	}
}

var WeekdayShort = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

func buildClock(r *Room) ClockView {
	sc := r.Schedule
	c := ClockView{Deadline: r.Deadline, Deferred: r.Deferred, NextTurn: r.Turn + 1,
		ScheduleText: fmt.Sprintf("Orders lock at %s, %s.", joinTimes(sc), daysText(sc.Days)),
		TZ:           sc.TimeZone, Days: make([]bool, 7)}
	if r.Phase == PhasePlanning {
		c.OrdersIn, c.OrdersTotal, c.Waiting = r.OrderReadiness()
		c.AllIn = c.OrdersTotal > 0 && c.OrdersIn == c.OrdersTotal
	}
	c.Unwritten = r.Unwritten()
	if !r.Deadline.IsZero() {
		c.DeadlineText = r.FormatInZone(r.Deadline)
		c.DeferDefault = r.Deadline.Add(24 * time.Hour).In(sc.Location()).Format("2006-01-02T15:04")
	}
	if !r.LockedAt.IsZero() {
		c.LockedText = r.FormatInZone(r.LockedAt)
	}
	c.Times = append(slices.Clone(sc.Times), make([]string, max(0, 4-len(sc.Times)))...)
	for d := range 7 {
		c.Days[d] = len(sc.Days) == 0 || slices.Contains(sc.Days, d)
	}
	return c
}

// contactID gives players a stable handle for a contact that doesn't leak
// the unit's real id (which can reveal what it is).
func contactID(r *Room, u *Unit) string {
	sum := sha256.Sum256([]byte(r.Code + "/" + u.ID))
	return "c" + hex.EncodeToString(sum[:5])
}

// Tab is one screen of the room.
type Tab struct {
	ID, Label, Href, Badge string
	Calm                   bool // the badge is good news, not a task
	Active                 bool
}

// screenOrder lists the screens each kind of member gets, in tab order.
// Players have one page; the referee has four.
var screenOrder = map[bool][]string{
	true:  {"map", "desk", "people", "setup"},
	false: {"map"},
}

var screenLabels = map[string]string{"map": "Table", "desk": "Desk", "people": "People", "setup": "Setup"}

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
		case "desk":
			switch {
			case v.Phase == PhaseResolving && v.Clock.Unwritten > 0:
				t.Badge = fmt.Sprint(v.Clock.Unwritten)
			case v.OpenFacts+len(v.Proposals) > 0:
				t.Badge = n(v.OpenFacts + len(v.Proposals))
			case v.Phase == PhaseResolving:
				t.Badge, t.Calm = "✓", true
			}
		case "people":
			t.Badge = n(len(v.Pending))
		case "map":
			unread := 0
			for _, c := range v.Channels {
				if !(screen == "map" && c.ID == v.Channel.ID) {
					unread += c.Unread
				}
			}
			t.Badge = n(unread)
		}
		v.Tabs = append(v.Tabs, t)
	}
}
