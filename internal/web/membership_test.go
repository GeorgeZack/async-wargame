package web

import (
	"net/url"
	"testing"
)

func TestLeaveOverHTTP(t *testing.T) {
	ts, store := newTestServer(t)
	ref := newBrowser(t, ts)
	code := createRoom(t, ref)
	base := "/r/" + code
	hicks := newBrowser(t, ts)
	hicks.post("/join", url.Values{"code": {code}, "name": {"Hicks"}})
	ref.post(base+"/members/"+memberByName(store, code, "Hicks").ID+"/approve", nil)
	hicks.post(base+"/briefed", nil)
	ref.post(base+"/start", nil)
	ref.post(base+"/units/m1/assign", url.Values{"member": {memberByName(store, code, "Hicks").ID}})

	_, body := hicks.get(base + "/people")
	mustContain(t, body, "Leave the operation", "go to the referee")
	res, _ := hicks.post(base+"/leave", nil)
	if res.Request.URL.Path != "/" {
		t.Errorf("after leaving: %s", res.Request.URL)
	}
	_, body = hicks.get(base)
	mustContain(t, body, "No longer a member")
	_, body = ref.get(base + "/desk")
	mustContain(t, body, "Hicks left the operation", "now with the referee")
}
