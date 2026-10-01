package game

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"slices"
	"time"
)

// The chain of command: the referee promotes players to commander and sets
// who each player reports to. A commander's pool is the units held by the
// commander and by the players who report to them; a commander can hand
// those units out among themselves and their players.

const RoleCommander Role = "commander"

// InPlay reports whether the member commands units in the game (not the referee).
func (m *Member) InPlay() bool {
	return m.Status == StatusApproved && (m.Role == RolePlayer || m.Role == RoleCommander)
}

// Subordinates returns the approved players who report to commander c.
func (r *Room) Subordinates(c *Member) []*Member {
	var out []*Member
	for _, m := range r.Members {
		if m.InPlay() && m.ReportsTo == c.ID && m.ID != c.ID {
			out = append(out, m)
		}
	}
	return out
}

// CanAssign reports whether actor may give unit u to target (nil = the referee).
func (r *Room) CanAssign(actor *Member, u *Unit, target *Member) bool {
	switch actor.Role {
	case RoleReferee:
		return target == nil || target.InPlay()
	case RoleCommander:
		if !actor.InPlay() || target == nil || !r.commands(actor, target) {
			return false
		}
		holder := r.Member(u.AssignedTo)
		return holder != nil && r.commands(actor, holder)
	}
	return false
}

// commands reports whether c may direct m: m is c, or reports to c.
func (r *Room) commands(c, m *Member) bool {
	return m != nil && m.InPlay() && (m.ID == c.ID || m.ReportsTo == c.ID)
}

// memberOrNil is Member, but safe to pass straight to commands.
func (r *Room) memberOrNil(id string) *Member {
	if id == "" {
		return nil
	}
	return r.Member(id)
}

// Promote makes an approved player a commander; Demote reverses it, and the
// commander's players then report to the referee again.
func (r *Room) setRole(m *Member, role Role) error {
	if !m.InPlay() {
		return bad("only approved players can be promoted or demoted")
	}
	if m.Role == role {
		return bad("%s is already a %s", m.Name, role)
	}
	m.Role = role
	if role == RoleCommander {
		m.ReportsTo = ""
		r.logf(VisAll, "%s was promoted to commander.", m.Name)
		return nil
	}
	for _, s := range r.Subordinates(m) {
		s.ReportsTo = ""
	}
	r.logf(VisAll, "%s is no longer a commander. Their players report to the referee.", m.Name)
	return nil
}

// SetReportsTo places a player under a commander, or under the referee with "".
func (r *Room) SetReportsTo(actor *Member, memberID, commanderID string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	m := r.Member(memberID)
	if m == nil || !m.InPlay() || m.Role != RolePlayer {
		return bad("only approved players can be placed under a commander")
	}
	if commanderID == "" {
		if m.ReportsTo != "" {
			m.ReportsTo = ""
			r.logf(VisAll, "%s now reports to the referee.", m.Name)
		}
		return nil
	}
	c := r.Member(commanderID)
	if c == nil || !c.InPlay() || c.Role != RoleCommander {
		return bad("that member is not a commander")
	}
	if m.ReportsTo != c.ID {
		m.ReportsTo = c.ID
		r.logf(VisAll, "%s now reports to %s.", m.Name, c.Name)
	}
	return nil
}

// --- invites ---

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

func (r *Room) openInvites(now time.Time) int {
	n := 0
	for _, i := range r.Invites {
		if i.Open(now) {
			n++
		}
	}
	return n
}

// CreateInvite issues an invite link for a commander or a player.
func (r *Room) CreateInvite(actor *Member, role Role, reportsTo string, maxUses int) (*Invite, error) {
	if actor.Role != RoleReferee {
		return nil, ErrForbidden
	}
	if role != RoleCommander && role != RolePlayer {
		return nil, bad("invites are for commanders or players")
	}
	if !slices.Contains([]int{1, 5, 20}, maxUses) {
		return nil, bad("an invite can be used 1, 5, or 20 times")
	}
	now := time.Now().UTC()
	if r.openInvites(now) >= maxOpenInvites {
		return nil, bad("at most %d open invites; revoke some first", maxOpenInvites)
	}
	if reportsTo != "" {
		c := r.Member(reportsTo)
		if role != RolePlayer || c == nil || !c.InPlay() || c.Role != RoleCommander {
			return nil, bad("only player invites can name a commander")
		}
	}
	b := make([]byte, 16)
	rand.Read(b)
	inv := &Invite{ID: newID(), Token: hex.EncodeToString(b), Role: role, ReportsTo: reportsTo,
		MaxUses: maxUses, CreatedAt: now, ExpiresAt: now.Add(inviteLifetime)}
	r.Invites = append(r.Invites, inv)
	r.logKind(VisReferee, "", "Created a %s invite (%s).", role, plural(maxUses, "use", "uses"))
	return inv, nil
}

func (r *Room) RevokeInvite(actor *Member, id string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	for _, i := range r.Invites {
		if i.ID == id {
			if i.Revoked {
				return bad("that invite is already revoked")
			}
			i.Revoked = true
			r.logKind(VisReferee, "", "Revoked a %s invite.", i.Role)
			return nil
		}
	}
	return ErrNotFound
}

// InviteByToken finds an invite by its secret; it may be closed.
func (r *Room) InviteByToken(token string) *Invite {
	for _, i := range r.Invites {
		if len(token) == len(i.Token) && subtle.ConstantTimeCompare([]byte(i.Token), []byte(token)) == 1 {
			return i
		}
	}
	return nil
}

// AcceptInvite creates an approved member from an open invite.
func (r *Room) AcceptInvite(token, name, tokenHash string) (*Member, error) {
	if r.Closed {
		return nil, bad("this room is closed")
	}
	inv := r.InviteByToken(token)
	now := time.Now().UTC()
	if inv == nil || !inv.Open(now) {
		return nil, bad("this invite has expired, been used up, or been revoked; ask the referee for a new one")
	}
	if r.nameTaken(name) {
		return nil, bad("someone in this room already uses that name")
	}
	m := &Member{ID: newID(), Name: name, Role: inv.Role, Status: StatusApproved,
		TokenHash: tokenHash, RequestedAt: now, DecidedAt: &now}
	if c := r.Member(inv.ReportsTo); c != nil && c.InPlay() && c.Role == RoleCommander {
		m.ReportsTo = c.ID
	}
	r.Members = append(r.Members, m)
	inv.Uses++
	inv.Joined = append(inv.Joined, m.ID)
	if c := r.Member(m.ReportsTo); c != nil {
		r.logf(VisAll, "%s joined by invite and reports to %s.", m.Name, c.Name)
	} else {
		r.logf(VisAll, "%s joined by invite as %s.", m.Name, m.Role)
	}
	return m, nil
}

// InviteSummary describes an invite to the person holding the link.
func (r *Room) InviteSummary(inv *Invite) string {
	if inv.Role == RoleCommander {
		return "as a commander. You'll be able to hand out the units placed under your command"
	}
	if c := r.Member(inv.ReportsTo); c != nil && c.InPlay() && c.Role == RoleCommander {
		return fmt.Sprintf("as a player reporting to %s", c.Name)
	}
	return "as a player"
}
