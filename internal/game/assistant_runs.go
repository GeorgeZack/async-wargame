package game

import (
	"slices"
	"time"
)

func (a *Assistant) queueRun(kind, target string, operation, turn int, now time.Time) {
	op := max(operation, 1)
	a.Runs = slices.DeleteFunc(a.Runs, func(x *Run) bool {
		return x.Kind == kind && x.Target == target && x.Turn == turn && x.Operation == op && (x.Status == RunPending || x.Status == RunFailed || x.Status == RunCancelled)
	})
	a.Runs = append(a.Runs, &Run{ID: newID(), Kind: kind, Target: target, Operation: op, Turn: turn, Status: RunPending, Queued: now.UTC()})
	if n := len(a.Runs); n > 60 {
		keep := slices.DeleteFunc(slices.Clone(a.Runs[:n-60]), func(x *Run) bool { return !x.Active() })
		a.Runs = append(keep, a.Runs[n-60:]...)
	}
}

func (a *Assistant) cancelRuns(match func(*Run) bool, now time.Time) int {
	n := 0
	for _, run := range a.Runs {
		if (run.Status == RunPending || run.Status == RunRunning) && match(run) {
			run.Status, run.Finished, run.Error = RunCancelled, now.UTC(), ""
			n++
		}
	}
	return n
}

func (a *Assistant) hasPendingRun() bool {
	return a.On && slices.ContainsFunc(a.Runs, func(x *Run) bool { return x.Status == RunPending })
}

func (a *Assistant) runCancelledNow(id string) bool {
	i := slices.IndexFunc(a.Runs, func(x *Run) bool { return x.ID == id })
	return i < 0 || a.Runs[i].Status == RunCancelled
}

func (a *Assistant) resetRunning() bool {
	changed := false
	for _, run := range a.Runs {
		if run.Status == RunRunning {
			run.Status, changed = RunPending, true
		}
	}
	return changed
}
