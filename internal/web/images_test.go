package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"asyncwargame/internal/game"
	"asyncwargame/internal/imageai"
	"asyncwargame/internal/imagework"
)

func TestVignetteReviewPrivacyAndPersistence(t *testing.T) {
	ts, store, ref, player, code, _ := setupSquad(t)
	var pic bytes.Buffer
	png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 128, 128)))
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct{ Model, Prompt string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "image-only" || !strings.Contains(req.Prompt, "Ancient phalanx at dawn") || strings.Contains(req.Prompt, "private briefing") {
			t.Errorf("bad prompt or model: %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(pic.Bytes())}}})
	}))
	defer api.Close()
	client, err := imageai.New(imageai.Config{BaseURL: api.URL, Model: "image-only"})
	if err != nil {
		t.Fatal(err)
	}
	ts.Config.Handler.(*Server).SetImageAI(client)
	base := "/r/" + code
	target := base + "/vignettes/1/0"
	form := url.Values{"prompt": {"Ancient phalanx at dawn"}, "caption": {"Soldiers at dawn"}}
	if res, _ := player.post(target+"/generate", form); res.StatusCode != 403 {
		t.Fatal("player generated image")
	}
	if res, body := ref.post(target+"/generate", form); res.StatusCode != 200 {
		t.Fatalf("queue: %d %s", res.StatusCode, body)
	}
	if res, _ := ref.post(target+"/generate", form); res.StatusCode != 400 {
		t.Fatal("duplicate request accepted")
	}
	worker := &imagework.Worker{Store: store, Client: client}
	if !worker.Once(context.Background()) || calls != 1 {
		t.Fatal("image worker did not run independently of text AI")
	}
	var draft game.Vignette
	store.Read(code, func(r *game.Room) {
		draft = *r.BriefingVignette
		if r.Board.Image != "" {
			t.Error("vignette changed map")
		}
	})
	path := base + "/vignettes/image/" + draft.File
	if res, _ := ref.get(path); res.StatusCode != 200 {
		t.Fatal("referee cannot preview")
	}
	if res, _ := player.get(path); res.StatusCode != 404 {
		t.Fatal("player saw draft")
	}
	if res, _ := newBrowser(t, ts).get(path); res.StatusCode != 404 {
		t.Fatal("stranger saw draft")
	}
	_, body := ref.get(base + "/setup")
	mustContain(t, body, "private preview", "Approve vignette for players", "Ancient phalanx at dawn")
	_, body = player.room(base)
	mustNotContain(t, body, "Soldiers at dawn", "Ancient phalanx at dawn", "image-only")
	decide := url.Values{"id": {draft.ID}, "action": {"approve"}}
	if res, _ := player.post(target+"/decide", decide); res.StatusCode != 403 {
		t.Fatal("player approved image")
	}
	if res, _ := ref.post(target+"/decide", decide); res.StatusCode != 200 {
		t.Fatal("approve failed")
	}
	if res, _ := player.get(path); res.StatusCode != 200 || res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatal("approved image not served privately")
	}
	reopened, err := game.OpenStore(filepath.Join(filepath.Dir(store.MapDir()), "state.json"), store.Cat)
	if err != nil {
		t.Fatal(err)
	}
	reopened.Read(code, func(r *game.Room) {
		if r.BriefingVignette == nil || !r.BriefingVignette.Approved || r.BriefingVignette.File != draft.File {
			t.Error("approved vignette did not survive reload")
		}
	})
	decide.Set("action", "hide")
	ref.post(target+"/decide", decide)
	if res, _ := player.get(path); res.StatusCode != 404 {
		t.Fatal("hidden image still served")
	}
	// A cancelled queued call is never sent, and a stale approval cannot approve its replacement.
	ref.post(target+"/generate", form)
	store.Read(code, func(r *game.Room) { decide.Set("id", r.BriefingVignette.ID) })
	decide.Set("action", "cancel")
	ref.post(target+"/decide", decide)
	if worker.Once(context.Background()) || calls != 1 {
		t.Fatal("cancelled image was sent")
	}
	decide.Set("action", "approve")
	decide.Set("id", draft.ID)
	if res, _ := ref.post(target+"/decide", decide); res.StatusCode != 400 {
		t.Fatal("stale approval accepted")
	}
}

func TestIndividualCardsAndEditableObjectives(t *testing.T) {
	_, store, ref, player, code, _ := setupSquad(t)
	base := "/r/" + code
	_, body := player.get(base + "/units/m2")
	if strings.Count(body, `class="card unit-card"`) != 1 {
		t.Fatal("individual link still displays whole squad")
	}
	mustContain(t, body, `id="card-h-m2"`)
	mustNotContain(t, body, `id="card-h-m1"`, `id="card-h-m3"`, `id="card-h-m4"`)
	form := url.Values{"campaign": {"Objectives test"}, "briefing": {"The squad has arrived."}, "notes": {"Private intelligence."}, "objectives": {"Secure the bridge.\nBring everyone home."}}
	if res, _ := player.post(base+"/edit/text", form); res.StatusCode != 403 {
		t.Fatal("player edited objectives")
	}
	if res, body := ref.post(base+"/edit/text", form); res.StatusCode != 200 {
		t.Fatalf("edit: %d %s", res.StatusCode, body)
	}
	store.Read(code, func(r *game.Room) {
		if len(r.Objectives) != 2 || r.Objectives[0] != "Secure the bridge." {
			t.Error("objectives not saved")
		}
	})
	ref.post(base+"/start", nil)
	_, body = player.get(base)
	mustContain(t, body, "Objectives", "Secure the bridge.", "Bring everyone home.")
	mustNotContain(t, body, "Private intelligence.")
}

func TestRefereeOrdersAndStagedMovesOverHTTP(t *testing.T) {
	_, store, ref, _, code, _ := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	ref.post(base+"/admin-mode", url.Values{"on": {"1"}})
	_, body := ref.get(base)
	mustContain(t, body, `id="order-form"`)
	// The normal drag endpoint stages referee moves during adjudication.
	store.Update(code, func(r *game.Room) error { r.Tick(store.Cat.Modules[r.ModuleID], r.Deadline); return nil })
	var before game.Pos
	store.Read(code, func(r *game.Room) { before = r.Unit("m1").Pos() })
	if res, _ := ref.post(base+"/units/move", url.Values{"unit": {"m1"}, "x": {"180"}, "y": {"120"}}); res.StatusCode != 200 {
		t.Fatal("staged drag failed")
	}
	store.Read(code, func(r *game.Room) {
		if r.Unit("m1").Pos() != before || len(r.PlansFor(r.Referee())) != 4 {
			t.Error("drag did not stage the formation")
		}
	})
	_, body = ref.get(base)
	mustContain(t, body, "applied when you publish")
	mustNotContain(t, body, "Apply my plans")
	if res, _ := ref.post(base+"/turn/next", nil); res.StatusCode != 200 {
		t.Fatal("publish failed")
	}
	store.Read(code, func(r *game.Room) {
		if r.Unit("m1").Pos() != (game.Pos{X: 180, Y: 120}) || len(r.Plans) != 0 {
			t.Error("publish did not finalize movement")
		}
	})
}
