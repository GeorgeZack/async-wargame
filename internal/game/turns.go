package game

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// The turn clock. Each turn's orders lock at a scheduled time in the room's
// time zone (say 6 PM and midnight Central), so people with day jobs know
// when to show up. At lock the enemy AI plans the infected's moves as a
// proposal for the referee. If the referee hasn't opened the next turn by the
// takeover time, the AI applies its plan and opens the next turn itself.

const PhaseResolving Phase = "resolving"

// minPlanning is the least time players get once a turn opens; a slot closer
// than this is skipped in favor of the following one.
const minPlanning = time.Hour

type Schedule struct {
	TimeZone string   `json:"tz"`
	Times    []string `json:"times"` // "18:00", "00:00"
	Days     []int    `json:"days"`  // time.Weekday values; empty means every day
	// Takeover is how many hours after lock the AI acts if the referee
	// hasn't resolved the turn: -1 never, 0 immediately.
	Takeover int `json:"takeover"`
}

func DefaultSchedule() Schedule {
	return Schedule{TimeZone: "America/Chicago", Times: []string{"18:00"}, Takeover: 6}
}

var TakeoverChoices = []int{-1, 0, 1, 2, 3, 6, 12, 24, 48}

type ZoneOption struct{ ID, Label string }

var ZoneOptions = []ZoneOption{
	{"America/New_York", "Eastern (New York)"}, {"America/Chicago", "Central (Chicago)"},
	{"America/Denver", "Mountain (Denver)"}, {"America/Phoenix", "Arizona (Phoenix)"},
	{"America/Los_Angeles", "Pacific (Los Angeles)"}, {"America/Anchorage", "Alaska (Anchorage)"},
	{"Pacific/Honolulu", "Hawaii (Honolulu)"}, {"Europe/London", "UK (London)"},
	{"Europe/Berlin", "Central Europe (Berlin)"}, {"Asia/Tokyo", "Japan (Tokyo)"},
	{"Australia/Sydney", "Australia East (Sydney)"}, {"UTC", "UTC"},
}

func (s Schedule) Location() *time.Location {
	if loc, err := time.LoadLocation(s.TimeZone); err == nil {
		return loc
	}
	return time.UTC
}

func (s Schedule) validate() error {
	if !slices.ContainsFunc(ZoneOptions, func(z ZoneOption) bool { return z.ID == s.TimeZone }) {
		return bad("choose a time zone from the list")
	}
	if _, err := time.LoadLocation(s.TimeZone); err != nil {
		return bad("the server doesn't know the time zone %s", s.TimeZone)
	}
	if len(s.Times) == 0 || len(s.Times) > 6 {
		return bad("set between 1 and 6 lock times")
	}
	seen := map[string]bool{}
	for _, t := range s.Times {
		if _, err := time.Parse("15:04", t); err != nil {
			return bad("lock time %q isn't a time of day", t)
		}
		if seen[t] {
			return bad("lock time %s is listed twice", t)
		}
		seen[t] = true
	}
	for _, d := range s.Days {
		if d < 0 || d > 6 {
			return bad("unknown weekday")
		}
	}
	if !slices.Contains(TakeoverChoices, s.Takeover) {
		return bad("unknown AI takeover setting")
	}
	return nil
}

// Next returns the first scheduled lock strictly after t, or the zero time
// if the schedule has none (which validation prevents).
func (s Schedule) Next(t time.Time) time.Time {
	loc := s.Location()
	local := t.In(loc)
	times := slices.Clone(s.Times)
	sort.Strings(times)
	for day := 0; day < 15; day++ {
		d := time.Date(local.Year(), local.Month(), local.Day()+day, 0, 0, 0, 0, loc)
		if len(s.Days) > 0 && !slices.Contains(s.Days, int(d.Weekday())) {
			continue
		}
		for _, hm := range times {
			p, _ := time.Parse("15:04", hm)
			c := time.Date(d.Year(), d.Month(), d.Day(), p.Hour(), p.Minute(), 0, 0, loc)
			if c.After(t) {
				return c.UTC()
			}
		}
	}
	return time.Time{}
}

// nextDeadline is the next slot that leaves players at least minPlanning.
func (r *Room) nextDeadline(now time.Time) time.Time {
	return r.Schedule.Next(now.Add(minPlanning))
}

// TakeoverAt is when the AI resolves a locked turn on its own; zero for never.
func (r *Room) TakeoverAt() time.Time {
	if r.Phase != PhaseResolving || r.Schedule.Takeover < 0 {
		return time.Time{}
	}
	return r.LockedAt.Add(time.Duration(r.Schedule.Takeover) * time.Hour)
}

// FormatInZone renders a time the way the room's schedule reads it.
func (r *Room) FormatInZone(t time.Time) string {
	return t.In(r.Schedule.Location()).Format("Mon Jan 2, 3:04 PM MST")
}

func (r *Room) SetSchedule(actor *Member, s Schedule, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if err := s.validate(); err != nil {
		return err
	}
	r.Schedule = s
	msg := fmt.Sprintf("Turn schedule set: orders lock at %s, %s.", joinTimes(s), daysText(s.Days))
	if r.Phase == PhasePlanning && !r.Deferred {
		r.Deadline = r.nextDeadline(now)
		msg += " This turn now locks " + r.FormatInZone(r.Deadline) + "."
	}
	r.logf(VisAll, "%s", msg)
	return nil
}

func joinTimes(s Schedule) string {
	loc := s.Location()
	var out []string
	times := slices.Clone(s.Times)
	sort.Strings(times)
	for _, t := range times {
		p, _ := time.Parse("15:04", t)
		out = append(out, time.Date(2026, 1, 15, p.Hour(), p.Minute(), 0, 0, loc).Format("3:04 PM"))
	}
	zone := time.Now().In(loc).Format("MST")
	return fmt.Sprintf("%s %s", joinList(out), zone)
}

func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return fmt.Sprintf("%s and %s", strings.Join(items[:len(items)-1], ", "), items[len(items)-1])
}

var weekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

func daysText(days []int) string {
	switch {
	case len(days) == 0 || len(days) == 7:
		return "every day"
	case slices.Equal(sortedInts(days), []int{1, 2, 3, 4, 5}):
		return "on weekdays"
	case slices.Equal(sortedInts(days), []int{0, 6}):
		return "on weekends"
	}
	var names []string
	for _, d := range sortedInts(days) {
		names = append(names, weekdayNames[d])
	}
	return "on " + joinList(names)
}

func sortedInts(in []int) []int {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}

// openTurn starts planning for turn n with the next scheduled deadline.
func (r *Room) openTurn(n int, now time.Time) {
	r.Turn, r.Phase, r.Deferred, r.LockedAt = n, PhasePlanning, false, time.Time{}
	r.Deadline = r.nextDeadline(now)
	r.setReminder(now)
	r.logf(VisAll, "Turn %d planning is open. Orders lock %s.", n, r.FormatInZone(r.Deadline))
	r.announceTurn(n)
}

// Defer moves the lock to a later time chosen by the referee.
func (r *Room) Defer(actor *Member, until, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhasePlanning {
		return bad("only a turn in planning can have its lock deferred")
	}
	if !until.After(now.Add(5 * time.Minute)) {
		return bad("pick a time at least a few minutes from now")
	}
	if until.After(now.Add(30 * 24 * time.Hour)) {
		return bad("a lock can be deferred by at most 30 days")
	}
	r.Deadline, r.Deferred = until.UTC(), true
	r.setReminder(now)
	r.logf(VisAll, "The referee deferred the turn %d lock to %s.", r.Turn, r.FormatInZone(r.Deadline))
	return nil
}

// LockNow locks the current turn immediately.
func (r *Room) LockNow(actor *Member, mod *Module, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Phase != PhasePlanning {
		return bad("there is no turn in planning to lock")
	}
	r.lock(mod, now)
	return nil
}

// lock closes planning: orders freeze, and the server resolves the turn
// (see resolution.go) for the referee to review.
func (r *Room) lock(mod *Module, now time.Time) {
	r.Phase, r.LockedAt = PhaseResolving, now.UTC()
	r.Undo = nil // the previous turn can't be reopened once this one locks
	r.RemindAt = time.Time{}
	r.logf(VisAll, "Turn %d is locked. The referee is resolving it.", r.Turn)
	r.lockOrders(mod, now)
	rng, seed := newResolutionRNG()
	r.Resolution = r.resolve(mod, now, rng, seed)
	if t := r.TakeoverAt(); !t.IsZero() && !now.Before(t) {
		r.takeover(mod, now)
		return
	}
	if ref := r.Referee(); ref != nil {
		body := "Every move is worked out and ready to publish."
		if n := len(r.Resolution.Encounters); n > 0 {
			body = fmt.Sprintf("%s to settle, then publish.", capitalize(plural(n, "fight", "fights")))
		}
		r.notify(ref.ID, fmt.Sprintf("Turn %d locked", r.Turn), body, "turn")
	}
}

// NextTurn is the referee publishing the resolved turn and opening the next.
func (r *Room) NextTurn(actor *Member, mod *Module, now time.Time) error {
	return r.PublishTurn(actor, mod, now)
}

// takeover is the AI acting for an absent referee: the rules settle every
// open fight, and the turn is published.
func (r *Room) takeover(mod *Module, now time.Time) {
	r.settleRemaining(mod)
	r.logf(VisAll, "The referee was away, so the rules settled turn %d's fights and published it.", r.Turn)
	if ref := r.Referee(); ref != nil {
		r.notify(ref.ID, fmt.Sprintf("Turn %d published without you", r.Turn), "You were away, so the rules settled its fights and published it.", "reports")
	}
	r.publish(mod, now)
}

// Due reports whether the clock needs to act on this room at time now.
func (r *Room) Due(now time.Time) bool {
	if r.Closed {
		return false
	}
	switch r.Phase {
	case PhasePlanning:
		return !r.Deadline.IsZero() && !now.Before(r.Deadline) || r.reminderDue(now)
	case PhaseResolving:
		t := r.TakeoverAt()
		return !t.IsZero() && !now.Before(t)
	}
	return false
}

// Tick advances the clock: locks a turn whose deadline passed, and lets the
// AI take over a locked turn the referee left unresolved.
func (r *Room) Tick(mod *Module, now time.Time) {
	switch {
	case !r.Due(now):
	case r.Phase == PhasePlanning && r.reminderDue(now) && now.Before(r.Deadline):
		r.remind()
	case r.Phase == PhasePlanning:
		r.lock(mod, now)
	case r.Phase == PhaseResolving:
		r.takeover(mod, now)
	}
}
