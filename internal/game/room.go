package game

import (
	"asyncwargame/internal/rules"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Role string

const (
	RoleReferee Role = "referee"
	RolePlayer  Role = "player"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDeclined Status = "declined"
	StatusRemoved  Status = "removed"
)

// Phase is the campaign's position in the play cycle.
type Phase string

const (
	PhaseSetup    Phase = "setup"
	PhasePlanning Phase = "planning"
	// PhaseResolving: orders are locked and the referee is adjudicating.
	PhaseResolving Phase = "resolving"
)

// Room is one hosted campaign: its membership, canonical state, and history.
type Room struct {
	Code          string            `json:"code"`
	Name          string            `json:"name"`
	ScenarioID    string            `json:"scenario"`
	ModuleID      string            `json:"module"`
	ModuleVersion string            `json:"moduleVersion"`
	CreatedAt     time.Time         `json:"createdAt"`
	Turn          int               `json:"turn"`
	Phase         Phase             `json:"phase"`
	Closed        bool              `json:"closed,omitempty"`
	ClosedAt      time.Time         `json:"closedAt,omitzero"`
	Schedule      Schedule          `json:"schedule"`
	Deadline      time.Time         `json:"deadline,omitzero"`  // when planning locks
	Deferred      bool              `json:"deferred,omitempty"` // the referee moved the deadline off the schedule
	LockedAt      time.Time         `json:"lockedAt,omitzero"`
	Threat        int               `json:"threat"`
	Members       []*Member         `json:"members"`
	Units         []*Unit           `json:"units"`
	Features      []Feature         `json:"features"`
	Map           *HexMap           `json:"map"`   // the drawn hex map (the background when Board.Background is hex)
	Board         Board             `json:"board"` // background, grid, scale
	Briefing      []string          `json:"briefing"`
	RefereeNotes  string            `json:"refereeNotes"`
	Events        []*GameEvent      `json:"events,omitempty"`
	Invites       []*Invite         `json:"invites,omitempty"`
	Orders        []*Order          `json:"orders,omitempty"`
	Operation     int               `json:"operation,omitempty"` // current operation number, from 1
	Operations    []OperationRecord `json:"operations,omitempty"`
	// Desk is the referee's work on the turn being adjudicated.
	Desk *Desk `json:"desk,omitempty"`
	// LockedUnits is the board as it stood at the lock: what players see
	// while the referee adjudicates, so nothing leaks before publishing.
	LockedUnits []*Unit       `json:"lockedUnits,omitempty"`
	Reports     []*TurnReport `json:"reports,omitempty"`
	Facts       []*Fact       `json:"facts,omitempty"`
	Intentions  []*Intention  `json:"intentions,omitempty"` // what enemy units mean to do, by turn
	Log         []LogEntry    `json:"log"`
	Messages    []*Message    `json:"messages,omitempty"`
	Assistant   Assistant     `json:"assistant,omitzero"`    // the referee's AI assistant
	Undo        *Undo         `json:"undo,omitempty"`        // the game just before the last publish
	RemindAt    time.Time     `json:"remindAt,omitzero"`     // when to remind players their orders are due
	AllInTurn   int           `json:"allInTurn,omitempty"`   // last turn the referee was told every order was in
	Corrections []Correction  `json:"corrections,omitempty"` // turns reopened, and why
	// Outbox collects notifications raised during an update; the store
	// hands them to the sender once the update is saved.
	Outbox []Notice `json:"-"`
	// ReadMarks is, per member and channel, how many of the channel's
	// messages they have seen.
	ReadMarks map[string]map[string]int `json:"readMarks,omitempty"`
	Seq       int                       `json:"seq"` // numbering for things created during play
	Version   int64                     `json:"version"`
}

type Member struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Role        Role       `json:"role"`
	Status      Status     `json:"status"`
	TokenHash   string     `json:"tokenHash"`
	RequestedAt time.Time  `json:"requestedAt"`
	DecidedAt   *time.Time `json:"decidedAt,omitempty"`
	Briefed     bool       `json:"briefed"`
	// Test seats are puppet players the referee creates to try the game from
	// several browser tabs. Their key is kept so the referee can reopen them.
	TestKey string `json:"testKey,omitempty"`
	// ReportsTo is the commander this player answers to; empty means the referee.
	ReportsTo string `json:"reportsTo,omitempty"`
	// AdminMode lets a referee see player orders during planning.
	AdminMode bool `json:"adminMode,omitempty"`
	// Push is where to send this member's notifications: one entry per
	// browser that subscribed.
	Push []PushSub `json:"push,omitempty"`
}

// Unit is a marine, a group of survivors, or a pack of infected on the map.
// Its class is the template it started from; everything on it can be
// changed by the referee as the story goes.
type Unit struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Class      string   `json:"class"`
	Side       string   `json:"side"`
	X          float64  `json:"x"` // position on the board
	Y          float64  `json:"y"`
	Strength   int      `json:"strength"`             // headcount
	Quality    *int     `json:"quality,omitempty"`    // overrides the class's quality
	Kit        *string  `json:"kit,omitempty"`        // overrides the class's kit
	Notes      string   `json:"notes,omitempty"`      // the referee's notes, shown on the card
	Status     string   `json:"status,omitempty"`     // "pinned", "wounded: left arm"
	Brief      string   `json:"brief,omitempty"`      // private: only its holder and the referee read it
	Known      bool     `json:"known"`                // every player knows it's there
	KnownTo    []string `json:"knownTo,omitempty"`    // members who know it's there, when not everyone does
	Identified bool     `json:"identified,omitempty"` // those who know it's there know what it is
	AssignedTo string   `json:"assignedTo,omitempty"`
	Operations int      `json:"operations,omitempty"` // operations survived in this campaign
	Record     []string `json:"record,omitempty"`     // service record
	// Radio contact, set by the referee and by events.
	RadioDown    bool `json:"radioDown,omitempty"`
	OutOfContact bool `json:"outOfContact,omitempty"` // worked out by RefreshComms
	LastContact  int  `json:"lastContact,omitempty"`  // last turn it was in contact
	JammedUntil  int  `json:"jammedUntil,omitempty"`  // radio jammed through this turn
	Reported     *Pos `json:"reported,omitempty"`     // where it was when last in contact
}

type Feature struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Kind   string  `json:"kind"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Hidden bool    `json:"hidden,omitempty"`
	Note   string  `json:"note,omitempty"`
}

type Visibility string

const (
	VisAll     Visibility = "all"
	VisReferee Visibility = "referee"
)

type LogEntry struct {
	At         time.Time  `json:"at"`
	Text       string     `json:"text"`
	Visibility Visibility `json:"visibility"`
	Kind       string     `json:"kind,omitempty"` // "", "event" (referee record), or "sitrep" (player report)
	Turn       int        `json:"turn,omitempty"`
}

var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("not allowed")
	ErrBadRequest = rules.ErrInvalid
	ErrRoomFull   = errors.New("too many pending requests for this room")
)

const (
	MaxNameLen        = 32
	MaxPendingPerRoom = 20
	codeAlphabet      = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I, O, 0, 1
	codeLen           = 6
)

// NormalizeCode accepts user input like "abc-def " and returns "ABCDEF".
func NormalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if strings.ContainsRune(codeAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DisplayCode formats a room code for reading aloud: "ABC-DEF".
func DisplayCode(code string) string {
	if len(code) != codeLen {
		return code
	}
	return code[:3] + "-" + code[3:]
}

func newCode() string {
	b := make([]byte, codeLen)
	rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b)
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewToken returns a secret for the client and the hash to store.
func NewToken() (token, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	token = hex.EncodeToString(b)
	return token, HashToken(token)
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CleanName trims and validates a display or campaign name.
func CleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", fmt.Errorf("%w: name is required", ErrBadRequest)
	}
	if utf8.RuneCountInString(s) > MaxNameLen {
		return "", fmt.Errorf("%w: name must be %d characters or fewer", ErrBadRequest, MaxNameLen)
	}
	return s, nil
}

func (r *Room) MemberByToken(token string) *Member {
	if token == "" {
		return nil
	}
	h := HashToken(token)
	for _, m := range r.Members {
		if subtle.ConstantTimeCompare([]byte(m.TokenHash), []byte(h)) == 1 {
			return m
		}
	}
	return nil
}

func (r *Room) Member(id string) *Member {
	for _, m := range r.Members {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (r *Room) Unit(id string) *Unit {
	for _, u := range r.Units {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (r *Room) Referee() *Member {
	for _, m := range r.Members {
		if m.Role == RoleReferee {
			return m
		}
	}
	return nil
}

func (r *Room) logf(vis Visibility, format string, args ...any) {
	r.logKind(vis, "", format, args...)
}

func (r *Room) logKind(vis Visibility, kind, format string, args ...any) {
	r.Log = append(r.Log, LogEntry{At: time.Now().UTC(), Text: fmt.Sprintf(format, args...), Visibility: vis, Kind: kind, Turn: r.Turn})
}

func (r *Room) nameTaken(name string) bool {
	for _, m := range r.Members {
		if (m.Status == StatusApproved || m.Status == StatusPending) && strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}

// newRoom builds a campaign from a scenario with the creator as referee.
func newRoom(code, name string, sc *Scenario, mod *Module, refName, tokenHash string) *Room {
	now := time.Now().UTC()
	ref := &Member{ID: newID(), Name: refName, Role: RoleReferee, Status: StatusApproved,
		TokenHash: tokenHash, RequestedAt: now, DecidedAt: &now, Briefed: true}
	r := &Room{
		Code: code, Name: name, ScenarioID: sc.ID, ModuleID: mod.ID, ModuleVersion: mod.Version,
		CreatedAt: now, Phase: PhaseSetup, Threat: MinThreat, Operation: 1, Schedule: DefaultSchedule(), Members: []*Member{ref},
		Map: sc.Map.Clone(), Board: sc.Board.withDefaults(), Briefing: slices.Clone(sc.Briefing), RefereeNotes: sc.RefereeNotes,
	}
	for _, f := range sc.Features {
		p := f.Pos()
		r.Features = append(r.Features, Feature{ID: f.ID, Name: f.Name, Kind: f.Kind, X: p.X, Y: p.Y, Hidden: f.Hidden, Note: f.Note})
	}
	for _, su := range sc.Units {
		cl := mod.Class(su.Class)
		side := mod.Side(cl.Side)
		str := su.Strength
		if str == 0 {
			str = max(1, cl.Strength)
		}
		p := su.Pos()
		r.Units = append(r.Units, &Unit{
			ID: su.ID, Name: su.Name, Class: su.Class, Side: cl.Side,
			X: p.X, Y: p.Y, Strength: str, Brief: su.Brief,
			Known: su.Known || side.Playable, Identified: su.Known || side.Playable,
		})
	}
	r.logf(VisAll, "%s opened the campaign as referee.", refName)
	return r
}

// RequestJoin adds a pending membership request.
func (r *Room) RequestJoin(name, tokenHash string) (*Member, error) {
	if r.Closed {
		return nil, bad("this room is closed")
	}
	pending := 0
	for _, m := range r.Members {
		if m.Status == StatusPending {
			pending++
		}
	}
	if pending >= MaxPendingPerRoom {
		return nil, ErrRoomFull
	}
	if r.nameTaken(name) {
		return nil, fmt.Errorf("%w: someone in this room already uses that name", ErrBadRequest)
	}
	m := &Member{ID: newID(), Name: name, Role: RolePlayer, Status: StatusPending,
		TokenHash: tokenHash, RequestedAt: time.Now().UTC()}
	r.Members = append(r.Members, m)
	r.logf(VisReferee, "%s requested access.", name)
	return m, nil
}

// Decide applies a referee membership decision: approve, decline, or remove.
func (r *Room) Decide(actor *Member, memberID, action string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	m := r.Member(memberID)
	if m == nil || m.Role == RoleReferee {
		return ErrNotFound
	}
	now := time.Now().UTC()
	switch {
	case action == "approve" && m.Status == StatusPending:
		m.Status = StatusApproved
		r.logf(VisAll, "%s was approved and joined the operation.", m.Name)
	case action == "decline" && m.Status == StatusPending:
		m.Status = StatusDeclined
		r.logf(VisReferee, "Declined access request from %s.", m.Name)
	case action == "promote":
		return r.setRole(m, RoleCommander)
	case action == "demote":
		return r.setRole(m, RolePlayer)
	case action == "remove" && m.Status == StatusApproved:
		return r.Remove(actor, m.ID, "")
	default:
		return fmt.Errorf("%w: cannot %s a member who is %s", ErrBadRequest, action, m.Status)
	}
	m.DecidedAt = &now
	return nil
}

// Withdraw lets a pending player cancel their own request.
func (r *Room) Withdraw(m *Member) error {
	if m.Status != StatusPending {
		return ErrBadRequest
	}
	m.Status = StatusRemoved
	r.logf(VisReferee, "%s withdrew their access request.", m.Name)
	return nil
}

// Assign gives a playable unit to an approved player, or unassigns it when memberID is empty.
func (r *Room) Assign(actor *Member, mod *Module, unitID, memberID string) error {
	if actor.Role != RoleReferee && actor.Role != RoleCommander {
		return ErrForbidden
	}
	u := r.Unit(unitID)
	if u == nil {
		return ErrNotFound
	}
	if s := mod.Side(u.Side); s == nil || !s.Playable {
		return fmt.Errorf("%w: %s cannot be assigned to a player", ErrBadRequest, u.Name)
	}
	var target *Member
	if memberID != "" {
		if target = r.Member(memberID); target == nil || !target.InPlay() {
			return fmt.Errorf("%w: that player is not an approved member", ErrBadRequest)
		}
	}
	if !r.CanAssign(actor, u, target) {
		if actor.Role == RoleCommander {
			return fmt.Errorf("%w: commanders can only reassign units in their own command, to themselves or their players", ErrBadRequest)
		}
		return ErrForbidden
	}
	switch {
	case target == nil && u.AssignedTo != "":
		u.AssignedTo = ""
		r.logf(VisAll, "%s is now held by the referee.", u.Name)
	case target != nil && u.AssignedTo != target.ID:
		u.AssignedTo = target.ID
		if actor.Role == RoleCommander {
			r.logf(VisAll, "%s assigned %s to %s.", actor.Name, u.Name, target.Name)
		} else {
			r.logf(VisAll, "%s assigned to %s.", u.Name, target.Name)
		}
	}
	return nil
}

// BadRequest makes a user-facing validation error.
func BadRequest(format string, args ...any) error { return bad(format, args...) }
