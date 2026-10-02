package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestMessagesOverHTTP(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	hid := memberByName(store, code, "Hicks").ID
	ref.post(base+"/members/"+hid+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)

	res, _ := hicks.post(base+"/messages", url.Values{"channel": {"ref-" + hid}, "text": {"Requesting <b>air</b> support."}})
	if res.StatusCode != 200 || !strings.Contains(res.Request.URL.String(), "?c=ref-"+hid) {
		t.Fatalf("post: %d %s", res.StatusCode, res.Request.URL)
	}
	hicks.post(base+"/messages", url.Values{"channel": {"players"}, "text": {"Plan: hit the hospital at dawn."}})

	// The referee sees an unread badge on the Messages tab, then reads it.
	_, body := ref.get(base + "/people")
	mustContain(t, body, `>Table <span`)
	_, body = ref.get(base + "?c=ref-" + hid)
	mustContain(t, body, "Requesting &lt;b&gt;air&lt;/b&gt; support.", "With Hicks")
	mustNotContain(t, body, "hit the hospital", "Players only")
	_, body = ref.get(base + "/people")
	mustNotContain(t, body, `>Table <span`)

	// A blind referee can't post to or read the players' channel.
	if res, _ := ref.post(base+"/messages", url.Values{"channel": {"players"}, "text": {"Hi"}}); res.StatusCode != 403 {
		t.Errorf("referee posted to players: %d", res.StatusCode)
	}
	_, body = ref.get(base + "?c=players")
	mustNotContain(t, body, "hit the hospital")
}
