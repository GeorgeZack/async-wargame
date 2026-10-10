package game

// The chain of command: the referee promotes players to commander and sets
// who each player reports to. A commander's pool is the units held by the
// commander and by the players who report to them; a commander can hand
// those units out among themselves and their players.

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

// CreateInvite issues an invite link for a commander or a player.
func (r *Room) CreateInvite(actor *Member, role Role, reportsTo string, maxUses int) (*Invite, error) {
	if actor.Role != RoleReferee {
		return nil, ErrForbidden
	}
	inv, err := r.Membership.createInvite(role, reportsTo, maxUses)
	if err != nil {
		return nil, err
	}
	r.logKind(VisReferee, "", "Created a %s invite (%s).", role, plural(maxUses, "use", "uses"))
	return inv, nil
}

func (r *Room) RevokeInvite(actor *Member, id string) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	inv, err := r.Membership.revokeInvite(id)
	if err != nil {
		return err
	}
	r.logKind(VisReferee, "", "Revoked a %s invite.", inv.Role)
	return nil
}

// AcceptInvite creates an approved member from an open invite.
func (r *Room) AcceptInvite(token, name, tokenHash string) (*Member, error) {
	if r.Closed {
		return nil, bad("this room is closed")
	}
	m, err := r.Membership.acceptInvite(token, name, tokenHash)
	if err != nil {
		return nil, err
	}
	if c := r.Member(m.ReportsTo); c != nil {
		r.logf(VisAll, "%s joined by invite and reports to %s.", m.Name, c.Name)
	} else {
		r.logf(VisAll, "%s joined by invite as %s.", m.Name, m.Role)
	}
	return m, nil
}
