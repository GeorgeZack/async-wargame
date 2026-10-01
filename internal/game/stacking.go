package game

// Stacking. Each friendly class uses some stacking points and each terrain
// holds a number of them, so several squads and a vehicle fit in a street
// but not in a marsh. Civilians and the infected don't count. Units may pass
// through a full hex; they just can't end a move there.
//
// While orders are being written, a hex's load is "projected": every
// friendly unit counts where its current order (draft or committed) will
// leave it, so two squads can't both claim the last space.
//
// What a member is shown, and checked against, is their own picture: a unit
// off the net counts where it last reported from, and an order they aren't
// allowed to see doesn't count at all (the unit counts where it stands).
// Resolution uses the real positions and stops a move short of a full hex,
// so a picture that's out of date costs time, not the rules.

// StackLimit is how many stacking points the hex holds.
func (r *Room) StackLimit(mod *Module, col, row int) int {
	t := mod.TerrainAt(r.Map.TerrainCode(col, row))
	if t.Stack > 0 || t.Water() {
		return t.Stack
	}
	return mod.Stacking.Default
}

// stackPoints is what a unit counts against its hex; passengers don't count.
func stackPoints(mod *Module, u *Unit) int {
	if u.Aboard != "" {
		return 0
	}
	return classStack(mod, u)
}

// classStack is what a unit needs when it stands on its own.
func classStack(mod *Module, u *Unit) int {
	if s := mod.Side(u.Side); s == nil || !s.Playable {
		return 0
	}
	return mod.Class(u.Class).Stack
}

// plannedHex is where a friendly unit will end the turn under its current order.
func (r *Room) plannedHex(u *Unit) [2]int {
	if o := r.CurrentOrder(u.ID); o != nil && o.HasDest && o.Status != OrderLapsed && o.FromCol == u.Col && o.FromRow == u.Row {
		return [2]int{o.DestCol, o.DestRow}
	}
	return [2]int{u.Col, u.Row}
}

// ProjectedLoad sums stacking points planned to end in each hex, as me
// knows it (nil: as it really is), leaving out one unit.
func (r *Room) ProjectedLoad(mod *Module, except string, me *Member) map[[2]int]int {
	load := map[[2]int]int{}
	for _, u := range r.Units {
		if u.ID == except {
			continue
		}
		pts := stackPoints(mod, u)
		if me != nil && !r.seesOrder(me, u) {
			at, _ := r.reportedAt(u, me)
			load[at] += pts
			continue
		}
		if o := r.CurrentOrder(u.ID); o != nil && o.Status != OrderLapsed {
			switch {
			case o.Transport != "": // will be aboard: doesn't count where it boards
				pts = 0
			case u.Aboard != "" && o.HasDest: // getting off: counts where it lands
				pts = classStack(mod, u)
			}
		}
		load[r.plannedHex(u)] += pts
	}
	return load
}

// Load sums the stacking points in each hex right now.
func (r *Room) Load(mod *Module) map[[2]int]int {
	load := map[[2]int]int{}
	for _, u := range r.Units {
		load[[2]int{u.Col, u.Row}] += stackPoints(mod, u)
	}
	return load
}

// LoadFor sums the stacking points in each hex as me knows them.
func (r *Room) LoadFor(mod *Module, me *Member) map[[2]int]int {
	load := map[[2]int]int{}
	for _, u := range r.Units {
		at, _ := r.reportedAt(u, me)
		load[at] += stackPoints(mod, u)
	}
	return load
}

// fits reports whether unit u can end in a hex given a load that excludes it.
func (r *Room) fits(mod *Module, u *Unit, h [2]int, load map[[2]int]int) bool {
	need := classStack(mod, u)
	return need == 0 || load[h]+need <= r.StackLimit(mod, h[0], h[1])
}

func (r *Room) stackFullError(mod *Module, u *Unit, h [2]int, load map[[2]int]int) error {
	return bad("%s is full: %d of %d stacking points are already planned to end there, and %s needs %d",
		HexID(h[0], h[1]), load[h], r.StackLimit(mod, h[0], h[1]), u.Name, classStack(mod, u))
}
