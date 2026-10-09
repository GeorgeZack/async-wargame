package game

import (
	"errors"
	"time"
)

// A referee can close a room: play stops, the clock stops, nobody new can
// join, and members see that the operation has ended. A closed room keeps
// its record and can be reopened, or deleted for good.

var ErrClosed = errors.New("this room is closed")

func (r *Room) Close(actor *Member, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if r.Closed {
		return bad("the room is already closed")
	}
	r.Assistant.Automatic, r.Assistant.Decision = false, nil
	r.Assistant.Batches = nil
	r.Assistant.cancelRuns(func(x *Run) bool { return x.Automatic() }, now)
	r.Closed, r.ClosedAt = true, now.UTC()
	r.logf(VisAll, "The referee closed the room. The operation has ended.")
	return nil
}

// Reopen resumes play. A planning deadline that passed while the room was
// closed moves to the next scheduled slot, so the clock doesn't fire at once.
func (r *Room) Reopen(actor *Member, now time.Time) error {
	if actor.Role != RoleReferee {
		return ErrForbidden
	}
	if !r.Closed {
		return bad("the room is open")
	}
	r.Closed, r.ClosedAt = false, time.Time{}
	if r.Phase == PhasePlanning && r.Deadline.Before(now) {
		r.Deadline, r.Deferred = r.nextDeadline(now), false
	}
	if r.Phase == PhaseResolving {
		r.LockedAt = now.UTC() // restart the takeover window
	}
	r.logf(VisAll, "The referee reopened the room.")
	return nil
}
