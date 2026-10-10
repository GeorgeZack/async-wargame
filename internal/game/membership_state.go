package game

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Role string

const (
	RoleReferee   Role = "referee"
	RolePlayer    Role = "player"
	RoleCommander Role = "commander"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDeclined Status = "declined"
	StatusRemoved  Status = "removed"
)

// Membership owns members, their command hierarchy and admission links.
// Embedding it in Room keeps the existing flat members/invites JSON format.
type Membership struct {
	Members []*Member `json:"members"`
	Invites []*Invite `json:"invites,omitempty"`
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

// InPlay reports whether the member commands units in the game (not the referee).
func (m *Member) InPlay() bool {
	return m.Status == StatusApproved && (m.Role == RolePlayer || m.Role == RoleCommander)
}

func (ms *Membership) MemberByToken(token string) *Member {
	if token == "" {
		return nil
	}
	h := HashToken(token)
	for _, m := range ms.Members {
		if subtle.ConstantTimeCompare([]byte(m.TokenHash), []byte(h)) == 1 {
			return m
		}
	}
	return nil
}

func (ms *Membership) Member(id string) *Member {
	for _, m := range ms.Members {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (ms *Membership) Referee() *Member {
	for _, m := range ms.Members {
		if m.Role == RoleReferee {
			return m
		}
	}
	return nil
}

func (ms *Membership) nameTaken(name string) bool {
	for _, m := range ms.Members {
		if (m.Status == StatusApproved || m.Status == StatusPending) && strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}

// Subordinates returns the approved players who report to commander c.
func (ms *Membership) Subordinates(c *Member) []*Member {
	var out []*Member
	for _, m := range ms.Members {
		if m.InPlay() && m.ReportsTo == c.ID && m.ID != c.ID {
			out = append(out, m)
		}
	}
	return out
}

// CanAssign reports whether actor may give unit u to target (nil = the referee).
func (ms *Membership) CanAssign(actor *Member, u *Unit, target *Member) bool {
	switch actor.Role {
	case RoleReferee:
		return target == nil || target.InPlay()
	case RoleCommander:
		if !actor.InPlay() || target == nil || !ms.commands(actor, target) {
			return false
		}
		holder := ms.Member(u.AssignedTo)
		return holder != nil && ms.commands(actor, holder)
	}
	return false
}

// commands reports whether c may direct m: m is c, or reports to c.
func (ms *Membership) commands(c, m *Member) bool {
	return m != nil && m.InPlay() && (m.ID == c.ID || m.ReportsTo == c.ID)
}

// memberOrNil is Member, but safe to pass straight to commands.
func (ms *Membership) memberOrNil(id string) *Member {
	if id == "" {
		return nil
	}
	return ms.Member(id)
}

func (ms *Membership) requestJoin(name, tokenHash string) (*Member, error) {
	pending := 0
	for _, m := range ms.Members {
		if m.Status == StatusPending {
			pending++
		}
	}
	if pending >= MaxPendingPerRoom {
		return nil, ErrRoomFull
	}
	if ms.nameTaken(name) {
		return nil, fmt.Errorf("%w: someone in this room already uses that name", ErrBadRequest)
	}
	m := &Member{ID: newID(), Name: name, Role: RolePlayer, Status: StatusPending,
		TokenHash: tokenHash, RequestedAt: time.Now().UTC()}
	ms.Members = append(ms.Members, m)
	return m, nil
}

// Invite is a link the referee sends to a specific kind of member. Using it
// counts as the referee's approval, so the newcomer skips the pending queue.
type Invite struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"` // kept so the referee can copy the link again
	Role      Role      `json:"role"`
	ReportsTo string    `json:"reportsTo,omitempty"`
	MaxUses   int       `json:"maxUses"`
	Uses      int       `json:"uses"`
	Joined    []string  `json:"joined,omitempty"` // member ids
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revoked   bool      `json:"revoked,omitempty"`
}

const (
	inviteLifetime = 7 * 24 * time.Hour
	maxOpenInvites = 20
)

func (i *Invite) Open(now time.Time) bool {
	return !i.Revoked && i.Uses < i.MaxUses && now.Before(i.ExpiresAt)
}

func (ms *Membership) openInvites(now time.Time) int {
	n := 0
	for _, i := range ms.Invites {
		if i.Open(now) {
			n++
		}
	}
	return n
}

// InviteByToken finds an invite by its secret; it may be closed.
func (ms *Membership) InviteByToken(token string) *Invite {
	for _, i := range ms.Invites {
		if len(token) == len(i.Token) && subtle.ConstantTimeCompare([]byte(i.Token), []byte(token)) == 1 {
			return i
		}
	}
	return nil
}

// InviteSummary describes an invite to the person holding the link.
func (ms *Membership) InviteSummary(inv *Invite) string {
	if inv.Role == RoleCommander {
		return "as a commander. You'll be able to hand out the units placed under your command"
	}
	if c := ms.Member(inv.ReportsTo); c != nil && c.InPlay() && c.Role == RoleCommander {
		return fmt.Sprintf("as a player reporting to %s", c.Name)
	}
	return "as a player"
}

func (ms *Membership) createInvite(role Role, reportsTo string, maxUses int) (*Invite, error) {
	if role != RoleCommander && role != RolePlayer {
		return nil, bad("invites are for commanders or players")
	}
	if !slices.Contains([]int{1, 5, 20}, maxUses) {
		return nil, bad("an invite can be used 1, 5, or 20 times")
	}
	now := time.Now().UTC()
	if ms.openInvites(now) >= maxOpenInvites {
		return nil, bad("at most %d open invites; revoke some first", maxOpenInvites)
	}
	if reportsTo != "" {
		c := ms.Member(reportsTo)
		if role != RolePlayer || c == nil || !c.InPlay() || c.Role != RoleCommander {
			return nil, bad("only player invites can name a commander")
		}
	}
	b := make([]byte, 16)
	rand.Read(b)
	inv := &Invite{ID: newID(), Token: hex.EncodeToString(b), Role: role, ReportsTo: reportsTo,
		MaxUses: maxUses, CreatedAt: now, ExpiresAt: now.Add(inviteLifetime)}
	ms.Invites = append(ms.Invites, inv)
	return inv, nil
}

func (ms *Membership) revokeInvite(id string) (*Invite, error) {
	for _, i := range ms.Invites {
		if i.ID == id {
			if i.Revoked {
				return nil, bad("that invite is already revoked")
			}
			i.Revoked = true
			return i, nil
		}
	}
	return nil, ErrNotFound
}

func (ms *Membership) acceptInvite(token, name, tokenHash string) (*Member, error) {
	inv := ms.InviteByToken(token)
	now := time.Now().UTC()
	if inv == nil || !inv.Open(now) {
		return nil, bad("this invite has expired, been used up, or been revoked; ask the referee for a new one")
	}
	if ms.nameTaken(name) {
		return nil, bad("someone in this room already uses that name")
	}
	m := &Member{ID: newID(), Name: name, Role: inv.Role, Status: StatusApproved,
		TokenHash: tokenHash, RequestedAt: now, DecidedAt: &now}
	if c := ms.Member(inv.ReportsTo); c != nil && c.InPlay() && c.Role == RoleCommander {
		m.ReportsTo = c.ID
	}
	ms.Members = append(ms.Members, m)
	inv.Uses++
	inv.Joined = append(inv.Joined, m.ID)
	return m, nil
}
