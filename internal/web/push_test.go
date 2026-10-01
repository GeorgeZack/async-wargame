package web

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"asyncwargame/internal/game"
	"asyncwargame/internal/push"
)

func TestPushNotificationsReachTheBrowser(t *testing.T) {
	// A fake push service, and a fake browser's keys.
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	got := make(chan []byte, 4)
	svc := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- b
		w.WriteHeader(http.StatusCreated)
	}))
	defer svc.Close()
	sender, err := push.LoadOrCreate(filepath.Join(t.TempDir(), "vapid.pem"), "mailto:ref@example.com")
	if err != nil {
		t.Fatal(err)
	}
	sender.WithClient(svc.Client())
	srv, ts := serverWith(t, Options{Push: sender})
	store := srv.store

	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	hid := memberByName(store, code, "Hicks").ID
	ref.post(base+"/members/"+hid+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)

	_, body := hicks.get(base + "/people")
	mustContain(t, body, "Notifications on this device", `data-key="`+sender.PublicKey()+`"`)

	enc := base64.RawURLEncoding.EncodeToString
	sub := map[string]any{"endpoint": svc.URL + "/push/hicks", "keys": map[string]string{"p256dh": enc(ua.PublicKey().Bytes()), "auth": enc(auth)}}
	bad := map[string]any{"endpoint": "http://insecure.example/x", "keys": sub["keys"]}
	if res, _ := postJSON(hicks, base+"/push/subscribe", mustJSON(bad)); res.StatusCode != 400 {
		t.Errorf("accepted a plain-http endpoint: %d", res.StatusCode)
	}
	if res, _ := postJSON(hicks, base+"/push/subscribe", mustJSON(sub)); res.StatusCode != 204 {
		t.Fatalf("subscribe: %d", res.StatusCode)
	}
	if n := len(memberByName(store, code, "Hicks").Push); n != 1 {
		t.Fatalf("%d subscriptions stored", n)
	}

	// The referee writes to Hicks privately: his browser hears about it.
	ref.post(base+"/messages", url.Values{"channel": {"ref-" + hid}, "text": {"Your point man hears engines to the north."}})
	select {
	case b := <-got:
		plain, err := push.Decrypt(b, ua, auth)
		if err != nil {
			t.Fatal(err)
		}
		var n game.Notice
		json.Unmarshal(plain, &n)
		if n.Title != "Ref · With the referee" || n.Body != "Your point man hears engines to the north." || n.Room != code || n.Screen != "messages?c=ref-"+hid {
			t.Errorf("notice %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no push arrived")
	}

	// Turning it off on this device stops it.
	if res, _ := postJSON(hicks, base+"/push/unsubscribe", mustJSON(sub)); res.StatusCode != 204 || len(memberByName(store, code, "Hicks").Push) != 0 {
		t.Errorf("unsubscribe: %d, %d left", res.StatusCode, len(memberByName(store, code, "Hicks").Push))
	}
	res, _ := http.Get(ts.URL + "/sw.js")
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Errorf("service worker: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
