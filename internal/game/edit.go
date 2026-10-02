package game

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// EditOp is one referee change from the scenario editor. A batch of ops is
// applied atomically: if any op fails, the store discards the whole batch.
type EditOp struct {
	Op         string   `json:"op"`
	Hex        string   `json:"hex,omitempty"`
	Dir        int      `json:"dir"`
	Terrain    string   `json:"terrain,omitempty"`  // terrain id
	Type       string   `json:"type,omitempty"`     // edge type; "" removes the edge
	Crossing   string   `json:"crossing,omitempty"` // "" removes the crossing
	On         bool     `json:"on,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	Class      string   `json:"class,omitempty"`
	Name       string   `json:"name,omitempty"`
	Strength   int      `json:"strength,omitempty"`
	Known      *bool    `json:"known,omitempty"`
	Feature    string   `json:"feature,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Note       *string  `json:"note,omitempty"`
	Hidden     *bool    `json:"hidden,omitempty"`
	Operations *int     `json:"operations,omitempty"` // experience: operations survived
	X          *float64 `json:"x,omitempty"`          // a point on the board, for placing and moving
	Y          *float64 `json:"y,omitempty"`
}

const (
	maxOpsPerBatch = 1000
	maxTestSeats   = 8
	maxUnits       = 400
	maxFeatures    = 100
	maxTextLen     = 4000
)

func bad(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrBadRequest, fmt.Sprintf(format, args...))
}

func (r *Room) hex(id string) (int, int, error) {
	c, rw, ok := ParseHexID(id)
	if !ok || !r.Map.InBounds(c, rw) {
		return 0, 0, bad("no hex %q on this map", id)
	}
	return c, rw, nil
}

// point reads where an editor op puts something: an exact board point (x,
// y), or the middle of a drawn hex.
func (r *Room) point(op EditOp) (Pos, error) {
	if op.X != nil && op.Y != nil {
		w, h := r.Size()
		if *op.X < 0 || *op.Y < 0 || *op.X > w || *op.Y > h {
			return Pos{}, bad("that point is off the board")
		}
		return Pos{*op.X, *op.Y}, nil
	}
	col, row, err := r.hex(op.Hex)
	if err != nil {
		return Pos{}, err
	}
	return center(col, row), nil
}

func (r *Room) feature(id string) *Feature {
	for i := range r.Features {
		if r.Features[i].ID == id {
			return &r.Features[i]
		}
	}
	return nil
}

func cleanText(s string, max int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		return "", bad("text must be %d characters or fewer", max)
	}
	return s, nil
}

// ApplyEdits applies a batch of editor ops for the referee.
func (r *Room) ApplyEdits(actor *Member, mod *Module, ops []EditOp) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if len(ops) == 0 || len(ops) > maxOpsPerBatch {
		return bad("send between 1 and %d changes at a time", maxOpsPerBatch)
	}
	for i, op := range ops {
		if err := r.applyEdit(mod, op); err != nil {
			if len(ops) > 1 {
				return fmt.Errorf("change %d of %d: %w", i+1, len(ops), err)
			}
			return err
		}
	}
	if r.Phase != PhaseSetup {
		r.logKind(VisReferee, "event", "Editor: %s applied during play.", plural(len(ops), "change", "changes"))
	}
	return nil
}

func (r *Room) applyEdit(mod *Module, op EditOp) error {
	switch op.Op {
	case "terrain":
		col, row, err := r.hex(op.Hex)
		if err != nil {
			return err
		}
		var code byte
		for _, t := range mod.Terrain {
			if t.ID == op.Terrain {
				code = t.Code[0]
			}
		}
		if code == 0 {
			return bad("unknown terrain %q", op.Terrain)
		}
		r.Map.SetTerrain(col, row, code)
		return nil

	case "edge", "crossing", "road":
		col, row, err := r.hex(op.Hex)
		if err != nil {
			return err
		}
		s := HexSide{col, row, op.Dir}
		switch op.Op {
		case "road":
			if nc, nr, ok := r.Map.Neighbor(col, row, op.Dir); op.On && ok &&
				(mod.TerrainAt(r.Map.TerrainCode(col, row)).Water() || mod.TerrainAt(r.Map.TerrainCode(nc, nr)).Water()) {
				return bad("roads need land on both sides of %s", HexID(col, row))
			}
			return r.Map.SetRoad(s, op.On)
		case "edge":
			if op.Type == "" {
				return r.Map.SetEdge(s, "", "")
			}
			if err := mod.CheckEdge(op.Type, ""); err != nil {
				return err
			}
			crossing := ""
			if e := r.Map.EdgeAt(s); e != nil && mod.CheckEdge(op.Type, e.Crossing) == nil {
				crossing = e.Crossing // keep a bridge when a stream becomes a river
			}
			return r.Map.SetEdge(s, op.Type, crossing)
		default:
			e := r.Map.EdgeAt(s)
			if e == nil {
				return bad("there is no river, ridge, or wall on that side to cross")
			}
			if err := mod.CheckEdge(e.Type, op.Crossing); err != nil {
				return err
			}
			return r.Map.SetEdge(s, e.Type, op.Crossing)
		}

	case "unit-add":
		if len(r.Units) >= maxUnits {
			return bad("a map can hold at most %d units", maxUnits)
		}
		cl := mod.Class(op.Class)
		if cl == nil {
			return bad("unknown unit class %q", op.Class)
		}
		p, err := r.point(op)
		if err != nil {
			return err
		}
		r.Seq++
		name := op.Name
		if name == "" {
			name = fmt.Sprintf("%s %d", cl.Name, r.Seq)
		}
		if name, err = CleanName(name); err != nil {
			return err
		}
		known := mod.Side(cl.Side).Playable || (op.Known != nil && *op.Known)
		str := op.Strength
		if str <= 0 {
			str = max(1, cl.Strength)
		}
		r.Units = append(r.Units, &Unit{ID: fmt.Sprintf("s%d", r.Seq), Name: name, Class: cl.ID, Side: cl.Side,
			X: p.X, Y: p.Y, Strength: min(100000, str), Known: known, Identified: known})
		return nil

	case "unit-move", "unit-update", "unit-delete":
		u := r.Unit(op.Unit)
		if u == nil {
			return bad("that unit no longer exists; reload the editor")
		}
		switch op.Op {
		case "unit-delete":
			r.Units = slices.DeleteFunc(r.Units, func(x *Unit) bool { return x == u })
			return nil
		case "unit-move":
			p, err := r.point(op)
			if err != nil {
				return err
			}
			u.X, u.Y = p.X, p.Y
			return nil
		}
		if op.Name != "" {
			name, err := CleanName(op.Name)
			if err != nil {
				return err
			}
			u.Name = name
		}
		if op.Strength != 0 {
			if op.Strength < 1 || op.Strength > 100000 {
				return bad("strength must be between 1 and 100000")
			}
			u.Strength = op.Strength
		}
		if op.Known != nil && !mod.Side(u.Side).Playable {
			u.Known, u.Identified = *op.Known, *op.Known
		}
		if op.Operations != nil {
			if !mod.Side(u.Side).Playable || *op.Operations < 0 || *op.Operations > 50 {
				return bad("operations survived is 0 to 50, for friendly units")
			}
			u.Operations = *op.Operations
		}
		return nil

	case "feature-add":
		if len(r.Features) >= maxFeatures {
			return bad("a map can hold at most %d places", maxFeatures)
		}
		if !slices.Contains(FeatureKinds, op.Kind) {
			return bad("unknown place kind %q", op.Kind)
		}
		p, err := r.point(op)
		if err != nil {
			return err
		}
		name, err := CleanName(op.Name)
		if err != nil {
			return err
		}
		r.Seq++
		f := Feature{ID: fmt.Sprintf("f%d", r.Seq), Name: name, Kind: op.Kind, X: p.X, Y: p.Y}
		if op.Hidden != nil {
			f.Hidden = *op.Hidden
		}
		if op.Note != nil {
			if f.Note, err = cleanText(*op.Note, 300); err != nil {
				return err
			}
		}
		r.Features = append(r.Features, f)
		return nil

	case "feature-move", "feature-update", "feature-delete":
		f := r.feature(op.Feature)
		if f == nil {
			return bad("that place no longer exists; reload the editor")
		}
		switch op.Op {
		case "feature-delete":
			r.Features = slices.DeleteFunc(r.Features, func(x Feature) bool { return x.ID == op.Feature })
			return nil
		case "feature-move":
			p, err := r.point(op)
			if err != nil {
				return err
			}
			f.X, f.Y = p.X, p.Y
			return nil
		}
		if op.Name != "" {
			name, err := CleanName(op.Name)
			if err != nil {
				return err
			}
			f.Name = name
		}
		if op.Kind != "" {
			if !slices.Contains(FeatureKinds, op.Kind) {
				return bad("unknown place kind %q", op.Kind)
			}
			f.Kind = op.Kind
		}
		if op.Note != nil {
			note, err := cleanText(*op.Note, 300)
			if err != nil {
				return err
			}
			f.Note = note
		}
		if op.Hidden != nil {
			f.Hidden = *op.Hidden
		}
		return nil
	}
	return bad("unknown editor change %q", op.Op)
}

// SetTexts updates the campaign name, player briefing, and referee notes.
// Briefing paragraphs are separated by blank lines.
func (r *Room) SetTexts(actor *Member, name, briefing, notes string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	name, err := CleanName(name)
	if err != nil {
		return err
	}
	if briefing, err = cleanText(briefing, maxTextLen); err != nil {
		return err
	}
	if notes, err = cleanText(notes, maxTextLen); err != nil {
		return err
	}
	var paras []string
	for _, p := range strings.Split(strings.ReplaceAll(briefing, "\r\n", "\n"), "\n\n") {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			paras = append(paras, p)
		}
	}
	r.Name, r.Briefing, r.RefereeNotes = name, paras, notes
	return nil
}

// StartOperation releases the scenario to players and opens turn 1.
func (r *Room) StartOperation(actor *Member, mod *Module, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhaseSetup {
		return bad("the operation has already started")
	}
	r.logf(VisAll, "The operation has begun.")
	r.openTurn(1, now)
	return nil
}

// ResetToScenario restores the template map, places, units, and texts.
// Unit assignments survive for units that still exist.
func (r *Room) ResetToScenario(actor *Member, sc *Scenario, mod *Module) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhaseSetup {
		return bad("the scenario can only be reset before the operation starts")
	}
	assigned := map[string]string{}
	for _, u := range r.Units {
		assigned[u.ID] = u.AssignedTo
	}
	fresh := newRoom(r.Code, r.Name, sc, mod, "", "")
	r.Map, r.Features, r.Units = fresh.Map, fresh.Features, fresh.Units
	r.Briefing, r.RefereeNotes, r.Threat = fresh.Briefing, fresh.RefereeNotes, MinThreat
	for _, u := range r.Units {
		if m := r.Member(assigned[u.ID]); m != nil && m.Status == StatusApproved {
			u.AssignedTo = m.ID
		}
	}
	now := time.Now().UTC()
	for _, e := range r.Events {
		if e.Status == EventProposed {
			e.Status, e.DecidedAt = EventDiscarded, &now
		}
	}
	r.logKind(VisReferee, "event", "Scenario reset to the %s template.", sc.Name)
	return nil
}

// AddTestSeat creates an approved puppet player for testing from another
// browser tab. It returns the seat's key.
func (r *Room) AddTestSeat(actor *Member) (string, error) {
	if actor.Role != RoleReferee {
		return "", ErrForbidden
	}
	n := 0
	for _, m := range r.Members {
		if m.TestKey != "" && m.Status == StatusApproved {
			n++
		}
	}
	if n >= maxTestSeats {
		return "", bad("at most %d test seats at a time; remove one first", maxTestSeats)
	}
	name := ""
	for i := 1; name == "" || r.nameTaken(name); i++ {
		name = fmt.Sprintf("Tester %d", i)
	}
	token, hash := NewToken()
	now := time.Now().UTC()
	r.Members = append(r.Members, &Member{ID: newID(), Name: name, Role: RolePlayer, Status: StatusApproved,
		TokenHash: hash, RequestedAt: now, DecidedAt: &now, TestKey: token})
	r.logf(VisAll, "%s (test seat) joined the operation.", name)
	return token, nil
}

// --- editor view ---

type Editor struct {
	*View
	Live           bool // the operation has started; edits reach players immediately
	TerrainOptions []Terrain
	EdgeTypes      []EdgeType
	Crossings      []Crossing
	Classes        []ClassOption
	FeatureKinds   []string
	Units          []EditUnit
	Places         []EditPlace
	BriefingText   string
	EventTable     []EventRow
	ThreatLevels   []int
}

type ClassOption struct {
	ID, Name, SideName string
	Group              bool
}

type EditUnit struct {
	ID, Name, ClassName, Side, SideName, Where string
	Strength, Operations                       int
	Known, Playable                            bool
}

type EditPlace struct {
	ID, Name, Kind, Where, Note string
	Hidden                      bool
}

type EventRow struct {
	ID, Title, Category, Referee, Report string
	Weight                               int
	Limits                               string   // threat window, requirements
	Effects                              []string // plain-language effect list
	Odds                                 []string // chance per threat level, "—" when ineligible
	Now                                  bool     // can come up at the current threat
}

// BuildEditor prepares the referee's scenario editor.
func BuildEditor(r *Room, sc *Scenario, mod *Module, me *Member) *Editor {
	e := &Editor{
		View: BuildView(r, sc, mod, me), Live: r.Phase != PhaseSetup,
		TerrainOptions: mod.Terrain, EdgeTypes: mod.EdgeTypes, Crossings: mod.Crossings,
		FeatureKinds: FeatureKinds, BriefingText: strings.Join(r.Briefing, "\n\n"),
	}
	for _, cl := range mod.Classes {
		e.Classes = append(e.Classes, ClassOption{ID: cl.ID, Name: cl.Name, SideName: mod.Side(cl.Side).Name, Group: cl.IsGroup()})
	}
	for _, s := range mod.Sides {
		for _, u := range r.Units {
			if u.Side == s.ID {
				e.Units = append(e.Units, EditUnit{ID: u.ID, Name: u.Name, ClassName: mod.Class(u.Class).Name,
					Side: u.Side, SideName: s.Name, Where: r.Where(u.Pos(), true), Strength: u.Strength,
					Operations: u.Operations, Known: u.Known, Playable: s.Playable})
			}
		}
	}
	for _, f := range r.Features {
		e.Places = append(e.Places, EditPlace{ID: f.ID, Name: f.Name, Kind: f.Kind, Where: r.GridRef(f.Pos()), Note: f.Note, Hidden: f.Hidden})
	}
	for t := MinThreat; t <= MaxThreat; t++ {
		e.ThreatLevels = append(e.ThreatLevels, t)
	}
	e.EventTable = eventTable(r, mod)
	return e
}

func eventTable(r *Room, mod *Module) []EventRow {
	total := map[int]int{}
	for t := MinThreat; t <= MaxThreat; t++ {
		for i := range mod.Events {
			if r.eligibleAt(&mod.Events[i], t) {
				total[t] += mod.Events[i].Weight
			}
		}
	}
	var rows []EventRow
	for _, cat := range eventCategoryOrder {
		for i := range mod.Events {
			d := &mod.Events[i]
			if d.Category != cat {
				continue
			}
			row := EventRow{ID: d.ID, Title: d.Title, Category: d.Category, Referee: d.Referee, Report: d.Report,
				Weight: d.Weight, Now: r.Eligible(d)}
			var limits []string
			switch {
			case d.MinThreat > 0 && d.MaxThreat > 0:
				limits = append(limits, fmt.Sprintf("threat %d–%d", d.MinThreat, d.MaxThreat))
			case d.MinThreat > 0:
				limits = append(limits, fmt.Sprintf("threat %d+", d.MinThreat))
			case d.MaxThreat > 0:
				limits = append(limits, fmt.Sprintf("threat ≤%d", d.MaxThreat))
			}
			if d.Requires != "" {
				limits = append(limits, "needs a "+strings.ToLower(mod.Class(d.Requires).Name))
			}
			if d.Unique != "" {
				limits = append(limits, "only one "+mod.Class(d.Unique).Name)
			}
			row.Limits = strings.Join(limits, "; ")
			for _, f := range d.Effects {
				row.Effects = append(row.Effects, describeEffect(f, mod))
			}
			for t := MinThreat; t <= MaxThreat; t++ {
				if r.eligibleAt(d, t) && total[t] > 0 {
					row.Odds = append(row.Odds, fmt.Sprintf("%.0f%%", 100*float64(d.Weight)/float64(total[t])))
				} else {
					row.Odds = append(row.Odds, "—")
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func rangeText(r Range) string {
	if r[1] <= r[0] {
		return fmt.Sprint(r[0])
	}
	return fmt.Sprintf("%d–%d", r[0], r[1])
}

func classNames(ids []string, mod *Module) string {
	var names []string
	for _, id := range ids {
		names = append(names, strings.ToLower(mod.Class(id).Name)+"s")
	}
	return strings.Join(names, ", ")
}

// describeEffect explains an effect definition in plain language.
func describeEffect(f EffectDef, mod *Module) string {
	scaled := ""
	if f.Scale {
		scaled = ", scaled by threat"
	}
	count := rangeText(f.Count)
	if f.Count == (Range{}) {
		count = "all"
	}
	switch f.Op {
	case "spawn":
		cl := mod.Class(f.Class)
		size := ""
		if cl.IsGroup() {
			size = fmt.Sprintf(" of %s%s", rangeText(f.Strength), scaled)
		}
		var where []string
		w := f.Where
		if w.Edge != "" {
			where = append(where, "on the "+w.Edge+" map edge")
		}
		if len(w.Terrain) > 0 {
			where = append(where, "in "+strings.Join(w.Terrain, " or "))
		}
		if w.NearFeature != "" {
			where = append(where, "next to a "+w.NearFeature)
		}
		if w.NearClass != "" {
			where = append(where, "next to a "+strings.ToLower(mod.Class(w.NearClass).Name))
		}
		if w.AwayFromMarines > 0 {
			where = append(where, fmt.Sprintf("at least %d hexes from marines", w.AwayFromMarines))
		}
		vis := "hidden from players"
		if f.Known {
			vis = "seen by players"
		}
		return fmt.Sprintf("Spawn %s %s%s %s, %s", count, cl.Name, size, strings.Join(where, ", "), vis)
	case "grow":
		return fmt.Sprintf("Grow %s %s by %s%%%s", count, classNames(f.Classes, mod), rangeText(f.Percent), scaled)
	case "drift":
		return fmt.Sprintf("Move %s %s %s hex toward %s, routing around rivers and ridges",
			count, classNames(f.Classes, mod), rangeText(f.Steps), f.Toward)
	case "reveal":
		within := ""
		if f.Within > 0 {
			within = fmt.Sprintf(" within %d hexes of marines", f.Within)
		}
		return fmt.Sprintf("Reveal %s hidden %s%s", count, mod.Side(f.Side).Name, within)
	case "threat":
		return fmt.Sprintf("Threat %+d", f.Amount)
	case "jam":
		return fmt.Sprintf("Jam %s radios for %s turns", count, rangeText(f.Turns))
	}
	return f.Op
}
