package playtest

import "asyncwargame/internal/game"

// Compact only the already-filtered View. No raw Room data enters a bot prompt.
func playerMap(v *game.View) any {
	units := make([]map[string]any, 0, len(v.Data.Units))
	for _, u := range v.Data.Units {
		units = append(units, map[string]any{"id": u.ID, "name": u.Name, "role": u.Role, "at": game.Pos{X: u.X, Y: u.Y}, "where": u.Where, "strength": u.Strength, "status": u.Status, "destroyed": u.Destroyed, "stale": u.Stale, "formation": u.Formation, "members": u.Members})
	}
	counts := map[string]int{}
	dominant := ""
	for _, cell := range v.Map.Cells {
		counts[cell.T]++
		if counts[cell.T] > counts[dominant] {
			dominant = cell.T
		}
	}
	var exceptions []any
	for _, cell := range v.Map.Cells {
		if cell.T != dominant {
			exceptions = append(exceptions, []any{cell.X, cell.Y, cell.T})
		}
	}
	return map[string]any{"width": v.Data.Width, "height": v.Data.Height, "cell": v.Data.Cell, "grid": v.Data.Grid, "units": units, "places": v.Data.Places, "terrainLegend": v.Data.Terrain, "defaultTerrain": dominant, "terrainExceptionsXY": exceptions, "sides": v.Data.Sides}
}
func playerReports(v *game.View) any {
	var reports []any
	for i, r := range v.Reports {
		if i == 2 {
			break
		}
		var entries []game.EntryView
		for _, e := range r.Entries {
			if e.Mine || e.Seen != game.SeenByAll || r.Narrative == "" {
				entries = append(entries, e)
			}
		}
		reports = append(reports, map[string]any{"turn": r.Turn, "narrative": r.Narrative, "personalUpdates": entries})
	}
	return reports
}
