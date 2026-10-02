package game

import "encoding/json"

// migrateBoard rewrites a room saved before markers moved freely: units and
// places that sat on a hex (col, row) get the hex's center as their point,
// and [col,row] spots (reported positions, order marks) become points. It
// walks the whole room, so snapshots kept for corrections are fixed too.
func migrateBoard(msg json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(msg, &v) != nil {
		return msg
	}
	if !walkLegacy(v, "") {
		return msg
	}
	b, err := json.Marshal(v)
	if err != nil {
		return msg
	}
	return b
}

func walkLegacy(v any, key string) bool {
	changed := false
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			if a, ok := c.([]any); ok && (k == "reported" || k == "mark") && len(a) == 2 {
				col, ok1 := a[0].(float64)
				row, ok2 := a[1].(float64)
				if ok1 && ok2 {
					p := center(int(col), int(row))
					x[k] = map[string]any{"x": p.X, "y": p.Y}
					changed = true
					continue
				}
			}
			if walkLegacy(c, k) {
				changed = true
			}
		}
	case []any:
		for _, c := range x {
			if m, ok := c.(map[string]any); ok && (key == "units" || key == "lockedUnits" || key == "features") {
				col, ok1 := m["col"].(float64)
				row, ok2 := m["row"].(float64)
				if _, has := m["x"]; ok1 && ok2 && !has {
					p := center(int(col), int(row))
					m["x"], m["y"] = p.X, p.Y
					delete(m, "col")
					delete(m, "row")
					changed = true
				}
			}
			if walkLegacy(c, key) {
				changed = true
			}
		}
	}
	return changed
}
