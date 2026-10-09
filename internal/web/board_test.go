package web

import (
	"bytes"
	"html"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"asyncwargame/internal/game"
)

func upload(t *testing.T, b *browser, path string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, _ := w.CreateFormFile("image", "map.png")
	f.Write(data)
	w.Close()
	res, err := b.c.Post(b.ts.URL+path, w.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res
}

func TestMapImageAndScaleOverHTTP(t *testing.T) {
	ts, store, ref, hicks, code, _ := setupSquad(t)
	base := "/r/" + code
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 800, 600)))

	if res := upload(t, hicks, base+"/board/image", pic.Bytes()); res.StatusCode != 403 {
		t.Errorf("a player uploaded a map: %d", res.StatusCode)
	}
	if res := upload(t, ref, base+"/board/image", []byte("not a picture")); res.StatusCode != 400 {
		t.Errorf("accepted a non-image: %d", res.StatusCode)
	}
	if res := upload(t, ref, base+"/board/image", pic.Bytes()); res.StatusCode != 200 {
		t.Fatalf("upload: %d", res.StatusCode)
	}
	res, _ := ref.post(base+"/board", url.Values{"background": {"image"}, "grid": {"square"}, "cell": {"40"},
		"distance": {"5"}, "distance_unit": {"ft"}, "turn": {"1"}, "turn_unit": {"minutes"}})
	if res.StatusCode != 200 {
		t.Fatalf("board: %d", res.StatusCode)
	}
	var name string
	store.Read(code, func(r *game.Room) { name = r.Board.Image })
	if !strings.HasPrefix(name, code+"-") || hicksGet(hicks, base+"/board/"+name) != 200 {
		t.Errorf("a member can't see the map image %q", name)
	}
	if res, _ := newBrowser(t, ts).get(base + "/board/" + name); res.StatusCode != 404 {
		t.Errorf("a stranger saw the map image: %d", res.StatusCode)
	}
	ref.post(base+"/start", nil)
	_, body := hicks.get(base)
	mustContain(t, body, "1 square = 5 ft · 1 turn = 1 minute", `class="board-image"`, `class="grid-lines"`)
}

func hicksGet(b *browser, path string) int {
	res, _ := b.get(path)
	return res.StatusCode
}

func TestEnemyRevelationIsRefereeOnlyOverHTTP(t *testing.T) {
	_, store, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	form := url.Values{"revealed_to_enemy_set": {"1"}, "revealed_to_enemy": {"on"}}
	if res, _ := player.post(base+"/units/m2/edit", form); res.StatusCode != 403 {
		t.Fatal("player changed enemy awareness")
	}
	if res, _ := ref.post(base+"/units/m2/edit", form); res.StatusCode != 200 {
		t.Fatal("referee could not reveal a unit")
	}
	store.Read(code, func(r *game.Room) {
		if !r.Unit("m2").RevealedToEnemy {
			t.Error("revelation not persisted")
		}
	})
	_, body := ref.get(base + "/units/m2")
	mustContain(t, body, "Revealed to enemies", `name="revealed_to_enemy" checked`)
	_, body = player.get(base + "/units/m2")
	mustNotContain(t, body, "Revealed to enemies", "revealed_to_enemy", "revealedToEnemy")
	_, body = player.get(base)
	mustNotContain(t, body, "revealedToEnemy")
}

func TestBriefsAndRevealsOverHTTP(t *testing.T) {
	ts, store, ref, hicks, code, h := setupSquad(t)
	base := "/r/" + code
	vasq := newBrowser(t, ts)
	vasq.post("/join", url.Values{"code": {code}, "name": {"Vasquez"}})
	ref.post(base+"/members/"+memberByName(store, code, "Vasquez").ID+"/approve", nil)
	vasq.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)

	ref.post(base+"/units/m2/edit", url.Values{"brief": {"Your sister's girlfriend works at the hospital."}})
	_, body := hicks.get(base)
	mustContain(t, body, "Character background and private instructions", "sister&#39;s girlfriend")
	_, body = vasq.get(base)
	mustNotContain(t, body, "sister&#39;s girlfriend")
	if res, _ := vasq.get(base + "/units/m2"); res.StatusCode == 200 {
		_, body = vasq.get(base + "/units/m2")
		mustNotContain(t, body, "sister&#39;s girlfriend")
	}

	// The crawler is shown to Hicks alone.
	ref.post(base+"/units/z3/edit", url.Values{"known_to_set": {"1"}, "known_to": {h.ID}})
	_, body = hicks.get(base)
	mustContain(t, body, `data-unit="`)
	if !strings.Contains(body, "Crawler") && !strings.Contains(body, "Contact") {
		t.Error("Hicks can't see what was shown to him")
	}
	_, body = vasq.get(base)
	mustNotContain(t, body, "Basement crawler", "Crawler")
	// Players move markers only through orders; the referee drags them.
	if res, _ := hicks.post(base+"/units/m2/edit", url.Values{"x": {"10"}, "y": {"10"}}); res.StatusCode != 403 {
		t.Errorf("a player moved a counter: %d", res.StatusCode)
	}
	ref.post(base+"/units/m2/edit", url.Values{"x": {"200.5"}, "y": {"150"}})
	store.Read(code, func(r *game.Room) {
		if u := r.Unit("m2"); u.X != 200.5 || u.Y != 150 {
			t.Errorf("the referee's drag: %v,%v", u.X, u.Y)
		}
	})
}

func TestMoveSeveralOverHTTP(t *testing.T) {
	_, store, ref, hicks, code, _ := setupSquad(t)
	base := "/r/" + code
	for _, id := range []string{"m1", "m2", "m3"} {
		if res, _ := ref.post(base+"/units/"+id+"/split", nil); res.StatusCode != 200 {
			t.Fatal("split failed")
		}
	}
	form := url.Values{"unit": {"m1", "m2", "m3"}, "x": {"100", "120", "140"}, "y": {"200", "200", "200"}}
	if res, _ := hicks.post(base+"/units/move", form); res.StatusCode != 403 {
		t.Errorf("a player moved counters: %d", res.StatusCode)
	}
	if res, _ := ref.post(base+"/units/move", url.Values{"unit": {"m1", "m2"}, "x": {"1"}, "y": {"1", "2"}}); res.StatusCode != 400 {
		t.Errorf("unmatched coordinates: %d", res.StatusCode)
	}
	if res, _ := ref.post(base+"/units/move", form); res.StatusCode != 200 {
		t.Fatalf("move: %d", res.StatusCode)
	}
	store.Read(code, func(r *game.Room) {
		for i, id := range []string{"m1", "m2", "m3"} {
			if u := r.Unit(id); u.X != float64(100+20*i) || u.Y != 200 {
				t.Errorf("%s at %v,%v", id, u.X, u.Y)
			}
		}
	})
	_, body := ref.get(base)
	mustContain(t, body, "Select several", `id="select-box"`)
	_, body = hicks.get(base)
	mustNotContain(t, body, "Select several")
}

func TestFormationAndPrivatePlansOverHTTP(t *testing.T) {
	_, store, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	for _, action := range []string{"split", "rejoin"} {
		if res, _ := player.post(base+"/units/m2/"+action, nil); res.StatusCode != 403 {
			t.Fatalf("player %s: %d", action, res.StatusCode)
		}
	}
	if res, _ := ref.post(base+"/units/m2/split", nil); res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	_, body := ref.get(base)
	mustContain(t, body, `data-unit="f-lantern"`, `class="formation-tether"`, `formation-pip detached`)
	if res, _ := ref.post(base+"/units/m2/rejoin", nil); res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	_, body = player.get(base + "/units/m2?squad=1")
	mustContain(t, body, "Lantern Squad", "Sgt. Reyes", "LCpl. Okafor", "PFC Doyle", "HM3 Lin")
	// setupSquad assigns this player m2. The squad card must preserve brief privacy.
	var ownBrief, otherBrief string
	store.Read(code, func(r *game.Room) { ownBrief = r.Unit("m2").Brief; otherBrief = r.Unit("m1").Brief })
	mustContain(t, body, html.EscapeString(ownBrief))
	mustNotContain(t, body, html.EscapeString(otherBrief))
	form := url.Values{"unit": {"m2"}, "x": {"211"}, "y": {"177"}}
	if res, _ := player.post(base+"/plans", form); res.StatusCode != 200 {
		t.Fatalf("plan: %d", res.StatusCode)
	}
	_, body = player.get(base)
	mustContain(t, body, `"plans":[`, `"to":{"x":211,"y":177}`)
	_, body = ref.get(base)
	mustNotContain(t, body, `"plans":[`)
	if res, _ := player.post(base+"/plans/apply", nil); res.StatusCode != 403 {
		t.Fatal("player applied plans")
	}
	form.Set("x", "NaN")
	if res, _ := ref.post(base+"/plans", form); res.StatusCode != 400 {
		t.Fatal("nonfinite plan accepted")
	}
}
