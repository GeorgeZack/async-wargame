package game

import (
	"fmt"
	"strings"
	"time"
)

// Commander review. A commander can approve a committed order from one of
// their players, or return it for revision with a note, before the lock. A
// returned order goes back to draft: unless the player fixes and recommits
// it, it lapses at lock like any draft. Review is a chance to catch mistakes,
// not a guarantee: an order nobody returns is carried out as written.

const (
	ReviewApproved = "approved"
	ReviewReturned = "returned"
)

// CanReview reports whether actor may review the order on unit u: actor
// commands its holder (but isn't the holder) and can reach it on the net.
func (r *Room) CanReview(actor *Member, u *Unit) bool {
	if actor.Role != RoleCommander || u == nil {
		return false
	}
	holder := r.memberOrNil(u.AssignedTo)
	return holder != nil && holder.ID != actor.ID && holder.ReportsTo == actor.ID && r.seesOrder(actor, u)
}

// ReviewOrder approves an order or returns it for revision.
func (r *Room) ReviewOrder(actor *Member, id, action, note string, now time.Time) error {
	if r.Phase != PhasePlanning {
		return bad("orders can only be reviewed while a turn is in planning")
	}
	var o *Order
	for _, x := range r.Orders {
		if x.ID == id && x.Turn == r.Turn {
			o = x
		}
	}
	if o == nil {
		return ErrNotFound
	}
	u := r.Unit(o.Unit)
	if !r.CanReview(actor, u) {
		return ErrForbidden
	}
	if o.Status != OrderCommitted {
		return bad("only committed orders can be reviewed")
	}
	note = strings.TrimSpace(note)
	holder := r.memberOrNil(u.AssignedTo)
	switch action {
	case "approve":
		o.Review, o.ReviewNote = ReviewApproved, note
		r.logKind(VisPlayers, "", "%s approved %s's order for %s.", actor.Name, holder.Name, u.Name)
	case "return":
		if note == "" {
			return bad("say what should change")
		}
		if len([]rune(note)) > 300 {
			return bad("keep the note under 300 characters")
		}
		o.Review, o.ReviewNote = ReviewReturned, note
		o.Status, o.CommittedAt = OrderDraft, nil
		r.logKind(VisPlayers, "", "%s returned %s's order for %s: “%s”", actor.Name, holder.Name, u.Name, note)
		r.notify(holder.ID, fmt.Sprintf("%s returned an order", actor.Name), fmt.Sprintf("%s: “%s” Revise and commit before the lock.", u.Name, note), "orders")
	default:
		return bad("approve or return")
	}
	o.ReviewedBy, o.Version = actor.Name, o.Version+1
	o.History = append(o.History, OrderRevision{Version: o.Version, At: now.UTC(), By: actor.Name, Status: o.Status,
		Summary: action + "d" + map[bool]string{true: ": " + note, false: ""}[note != ""]})
	return nil
}
