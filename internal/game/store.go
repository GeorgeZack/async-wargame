package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store owns every campaign's canonical state. All access goes through its
// lock; each successful mutation is persisted before subscribers are notified.
//
// Persistence is a single JSON file replaced atomically on each write. That is
// adequate for a few small private campaigns; swap in a database once turn
// history and analysis records grow.
type Store struct {
	mu    sync.Mutex
	path  string
	rooms map[string]*Room
	subs  map[string]map[chan int64]struct{}
	Cat   *Catalog
	// Deliver, if set, receives the notifications an update raised, after
	// the update is saved and the store is unlocked.
	Deliver func([]Notice)
	// retired holds rooms built on a module this server no longer has (an
	// older version of the game). They aren't served, but they're written
	// back untouched so nothing is lost.
	retired []json.RawMessage
}

func OpenStore(path string, cat *Catalog) (*Store, error) {
	s := &Store{path: path, rooms: map[string]*Room{}, subs: map[string]map[chan int64]struct{}{}, Cat: cat}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var rooms []*Room
	for _, msg := range raw {
		var r Room
		if err := json.Unmarshal(migrateBoard(msg), &r); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if cat.Modules[r.ModuleID] == nil || cat.Scenarios[r.ScenarioID] == nil {
			s.retired = append(s.retired, msg)
			log.Printf("room %s (%s) uses %s, which this version doesn't have; kept in the file but not served", r.Code, r.Name, r.ModuleID)
			continue
		}
		rooms = append(rooms, &r)
	}
	for _, r := range rooms {
		if r.Threat == 0 { // saved before threat levels existed
			r.Threat = MinThreat
		}
		if r.Schedule.TimeZone == "" { // saved before the turn clock existed
			r.Schedule = DefaultSchedule()
			if r.Phase == PhasePlanning {
				r.Deadline = r.nextDeadline(time.Now())
			}
		}
		if r.Map == nil { // saved before rooms kept their own map
			if sc := cat.Scenarios[r.ScenarioID]; sc != nil {
				r.Map, r.Briefing, r.RefereeNotes = sc.Map.Clone(), sc.Briefing, sc.RefereeNotes
			}
		}
		if r.Board.Background == "" { // saved before boards had settings
			if sc := cat.Scenarios[r.ScenarioID]; sc != nil {
				r.Board = sc.Board
			}
		}
		if r.Objectives == nil {
			r.Objectives = append([]string{}, cat.Scenarios[r.ScenarioID].Objectives...)
		}
		r.Board = r.Board.withDefaults()
		s.rooms[r.Code] = r
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	rooms := make([]any, 0, len(s.rooms)+len(s.retired))
	for _, r := range s.rooms {
		rooms = append(rooms, r)
	}
	for _, r := range s.retired {
		rooms = append(rooms, r)
	}
	b, err := json.MarshalIndent(rooms, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// Create starts a new campaign and returns its room code.
func (s *Store) Create(name, scenarioID, refName, tokenHash string) (string, error) {
	sc := s.Cat.Scenarios[scenarioID]
	if sc == nil {
		return "", fmt.Errorf("%w: unknown scenario", ErrBadRequest)
	}
	mod := s.Cat.Modules[sc.Module]
	s.mu.Lock()
	defer s.mu.Unlock()
	code := newCode()
	for s.rooms[code] != nil {
		code = newCode()
	}
	r := newRoom(code, name, sc, mod, refName, tokenHash)
	r.Version = 1
	s.rooms[code] = r
	if err := s.saveLocked(); err != nil {
		delete(s.rooms, code)
		return "", err
	}
	return code, nil
}

// Read calls fn with the room under lock. fn must not retain the room.
func (s *Store) Read(code string, fn func(*Room)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[code]
	if r == nil {
		return ErrNotFound
	}
	fn(r)
	return nil
}

// Codes lists the rooms.
func (s *Store) Codes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.rooms))
	for code := range s.rooms {
		out = append(out, code)
	}
	return out
}

// Touch makes a bookkeeping change that nobody else needs to see, such as a
// read mark: no new version and no notifications. fn reports whether it
// changed anything; only then is the state saved.
func (s *Store) Touch(code string, fn func(*Room) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[code]
	if r == nil {
		return ErrNotFound
	}
	if !fn(r) {
		return nil
	}
	return s.saveLocked()
}

// Update applies fn to a copy of the room. If fn succeeds the copy becomes
// canonical, is saved, and subscribers are notified; otherwise nothing changes.
func (s *Store) Update(code string, fn func(*Room) error) error {
	var notices []Notice
	defer func() { // runs after the unlock below
		if len(notices) > 0 && s.Deliver != nil {
			s.Deliver(notices)
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	orig := s.rooms[code]
	if orig == nil {
		return ErrNotFound
	}
	r, err := cloneRoom(orig)
	if err != nil {
		return err
	}
	if err := fn(r); err != nil {
		return err
	}
	if mod := s.Cat.Modules[r.ModuleID]; mod != nil {
		r.RefreshComms(mod)
	}
	r.Version++
	s.rooms[code] = r
	if err := s.saveLocked(); err != nil {
		s.rooms[code] = orig
		return err
	}
	notices, r.Outbox = r.Outbox, nil
	for ch := range s.subs[code] {
		select {
		case ch <- r.Version:
		default: // subscriber is behind; it will catch up on the next send
		}
	}
	return nil
}

func cloneRoom(r *Room) (*Room, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var c Room
	return &c, json.Unmarshal(b, &c)
}

// Subscribe returns a channel that receives the room version after each change.
func (s *Store) Subscribe(code string) (<-chan int64, func()) {
	ch := make(chan int64, 1)
	s.mu.Lock()
	if s.subs[code] == nil {
		s.subs[code] = map[chan int64]struct{}{}
	}
	s.subs[code][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs[code], ch)
		if len(s.subs[code]) == 0 {
			delete(s.subs, code)
		}
		s.mu.Unlock()
	}
}

// Tick runs the turn clock for every room that is due: locking turns whose
// deadline passed and letting the AI take over turns left unresolved.
func (s *Store) Tick(now time.Time) {
	s.mu.Lock()
	var due []string
	for code, r := range s.rooms {
		if r.Due(now) {
			due = append(due, code)
		}
	}
	s.mu.Unlock()
	for _, code := range due {
		err := s.Update(code, func(r *Room) error {
			if !r.Due(now) {
				return errNotDue
			}
			r.Tick(s.Cat.Modules[r.ModuleID], now)
			return nil
		})
		if err != nil && !errors.Is(err, errNotDue) {
			log.Printf("turn clock, room %s: %v", code, err)
		}
	}
}

var errNotDue = errors.New("not due")

// RunClock ticks the turn clock until stop is closed.
func (s *Store) RunClock(every time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	s.Tick(time.Now())
	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			s.Tick(now)
		}
	}
}

// Delete removes a closed room permanently. check runs under the lock and
// must approve the deletion (it verifies the referee).
func (s *Store) Delete(code string, check func(*Room) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.rooms[code]
	if r == nil {
		return ErrNotFound
	}
	if err := check(r); err != nil {
		return err
	}
	if !r.Closed {
		return bad("close the room before deleting it")
	}
	delete(s.rooms, code)
	if err := s.saveLocked(); err != nil {
		s.rooms[code] = r
		return err
	}
	for ch := range s.subs[code] {
		select {
		case ch <- r.Version + 1:
		default:
		}
	}
	return nil
}

// MapDir is where uploaded map images are kept: next to the state file.
func (s *Store) MapDir() string { return filepath.Join(filepath.Dir(s.path), "maps") }
