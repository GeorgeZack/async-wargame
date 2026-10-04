package playtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"asyncwargame/internal/game"
)

// Continue from a finished playtest into a NEW directory. Source evidence stays
// untouched, and this run receives its own call/time budgets and transcript.
func loadPrevious(cat *game.Catalog, from string) (*game.Catalog, []byte, string, int, error) {
	b, err := os.ReadFile(filepath.Join(from, "summary.json"))
	if err != nil {
		return nil, nil, "", 0, err
	}
	var summary Summary
	if err = json.Unmarshal(b, &summary); err != nil {
		return nil, nil, "", 0, err
	}
	if summary.Finished.IsZero() || len(summary.Games) != 1 {
		return nil, nil, "", 0, errors.New("continue requires a finished single-game playtest")
	}
	b, err = os.ReadFile(filepath.Join(from, "campaigns.json"))
	if err != nil {
		return nil, nil, "", 0, err
	}
	var rooms []*game.Room
	if err = json.Unmarshal(b, &rooms); err != nil {
		return nil, nil, "", 0, err
	}
	if len(rooms) != 1 || rooms[0].Code != summary.Games[0].Code {
		return nil, nil, "", 0, errors.New("playtest summary and campaign do not match")
	}
	id := rooms[0].ScenarioID
	copy := &game.Catalog{Modules: cat.Modules, Scenarios: make(map[string]*game.Scenario, len(cat.Scenarios)+1)}
	for k, v := range cat.Scenarios {
		copy.Scenarios[k] = v
	}
	if copy.Scenarios[id] == nil {
		// The ID is data, never a path supplied by the previous model.
		if filepath.Base(id) != id || id == "." || id == ".." {
			return nil, nil, "", 0, errors.New("invalid scenario id")
		}
		raw, e := os.ReadFile(filepath.Join(from, "scenarios", id+".json"))
		if e != nil {
			return nil, nil, "", 0, e
		}
		var sc game.Scenario
		if e = json.Unmarshal(raw, &sc); e != nil {
			return nil, nil, "", 0, e
		}
		if sc.ID != id || copy.Modules[sc.Module] == nil {
			return nil, nil, "", 0, errors.New("missing source scenario module")
		}
		copy.Scenarios[id] = &sc
	}
	return copy, b, id, summary.Players, nil
}
func previousSeats(store *game.Store) (string, []seat, error) {
	codes := store.Codes()
	if len(codes) != 1 {
		return "", nil, errors.New("continue needs one game")
	}
	var seats []seat
	err := store.Read(codes[0], func(r *game.Room) {
		for _, m := range r.Members {
			if m.InPlay() {
				profile := playerProfiles[len(seats)%len(playerProfiles)]
				if r.Wayward != nil {
					profile = "Explore cooperatively using your character’s strengths. Follow discovered clues, change approach after failure, and avoid combat or resource spending."
				}
				seats = append(seats, seat{m.ID, profile})
			}
		}
	})
	if err == nil && (len(seats) == 0 || len(seats) > 100) {
		err = fmt.Errorf("continue needs 1–100 player seats, got %d", len(seats))
	}
	return codes[0], seats, err
}
