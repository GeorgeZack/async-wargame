package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"asyncwargame/internal/game"
)

const maxMapImage = 15 << 20

// formPos reads a point from two form fields; nil if either is missing.
func formPos(r *http.Request, x, y string) *game.Pos {
	xs, ys := strings.TrimSpace(r.FormValue(x)), strings.TrimSpace(r.FormValue(y))
	if xs == "" || ys == "" {
		return nil
	}
	px, err1 := strconv.ParseFloat(xs, 64)
	py, err2 := strconv.ParseFloat(ys, 64)
	if err1 != nil || err2 != nil {
		return nil
	}
	return &game.Pos{X: px, Y: py}
}

func formFloat(r *http.Request, name string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue(name)), 64)
	return f
}

// setBoard is the referee choosing the background, grid, and scale.
func (s *Server) setBoard(w http.ResponseWriter, r *http.Request) {
	e := game.BoardEdit{
		Background: r.FormValue("background"),
		Grid:       r.FormValue("grid"),
		Cell:       formFloat(r, "cell"),
		Distance:   game.Measure{Amount: formFloat(r, "distance"), Unit: r.FormValue("distance_unit")},
		Turn:       game.Measure{Amount: formFloat(r, "turn"), Unit: r.FormValue("turn_unit")},
	}
	s.act(w, r, func(room *game.Room, me *game.Member) error { return room.SetBoard(me, e) })
}

// uploadMap takes a map image from the referee. The file is kept under a
// name made from its contents, so a new upload never overwrites one an old
// correction snapshot still points to.
func (s *Server) uploadMap(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMapImage+1<<16)
	f, _, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Choose an image of up to 15 MB.", http.StatusBadRequest)
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxMapImage+1))
	if err != nil || len(b) > maxMapImage {
		http.Error(w, "Choose an image of up to 15 MB.", http.StatusBadRequest)
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		http.Error(w, "That doesn't look like a PNG, JPEG, or GIF image.", http.StatusBadRequest)
		return
	}
	ext := map[string]string{"png": ".png", "jpeg": ".jpg", "gif": ".gif"}[format]
	sum := sha256.Sum256(b)
	code := game.NormalizeCode(r.PathValue("code"))
	name := fmt.Sprintf("%s-%s%s", code, hex.EncodeToString(sum[:8]), ext)
	// Check the seat before writing anything to disk.
	var allowed bool
	s.store.Read(code, func(room *game.Room) {
		me := s.member(r, room)
		allowed = me != nil && me.Role == game.RoleReferee && !room.Closed
	})
	if !allowed {
		http.Error(w, "Only the referee can change the map.", http.StatusForbidden)
		return
	}
	dir := s.store.MapDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, "Couldn't save the image.", http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		http.Error(w, "Couldn't save the image.", http.StatusInternalServerError)
		return
	}
	s.act(w, r, func(room *game.Room, me *game.Member) error {
		return room.SetBoardImage(me, name, cfg.Width, cfg.Height)
	})
}

// mapImage serves a room's map image to its members.
func (s *Server) mapImage(w http.ResponseWriter, r *http.Request) {
	code := game.NormalizeCode(r.PathValue("code"))
	want := r.PathValue("name")
	ok := false
	s.store.Read(code, func(room *game.Room) {
		ok = s.member(r, room) != nil && room.Board.Image == want && want != ""
	})
	if !ok || strings.ContainsAny(want, `/\`) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(s.store.MapDir(), want))
}
