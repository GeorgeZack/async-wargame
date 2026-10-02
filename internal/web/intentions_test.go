package web

import (
	"net/url"
	"testing"

	"asyncwargame/internal/game"
)

func TestIntentionsAndQuestionsOverHTTP(t *testing.T) {
	_, store, ref, hicks, code, h := setupSquad(t)
	base := "/r/" + code
	ref.post(base+"/start", nil)
	ref.post(base+"/intentions", url.Values{"unit": {"z1"}, "text": {"Shuffle toward the bridge."}})
	if res, _ := hicks.post(base+"/intentions", url.Values{"unit": {"z1"}, "text": {"Go home."}}); res.StatusCode != 403 {
		t.Errorf("a player set an intention: %d", res.StatusCode)
	}
	_, body := ref.get(base + "/turn")
	mustContain(t, body, "The other side", "Shuffle toward the bridge.")
	_, body = hicks.room(base)
	mustNotContain(t, body, "Shuffle toward the bridge.", "The other side")

	hicks.post(base+"/orders", url.Values{"unit": {"m2"}, "text": {"Okafor deals with it."}, "submit": {"commit"}})
	ref.post(base+"/turn/lock", nil)
	var orderID string
	store.Read(code, func(r *game.Room) { orderID = r.LockedOrders()[0].ID })
	_, body = ref.get(base + "/turn")
	mustContain(t, body, "Ask Hicks about this order", "Shuffle toward the bridge.")
	res, msg := ref.post(base+"/desk/ask", url.Values{"order": {orderID}, "text": {"Deal with what, exactly?"}})
	if res.StatusCode != 200 {
		t.Fatalf("ask: %d %s", res.StatusCode, msg)
	}
	_, body = hicks.get(base + "/messages?c=ref-" + h.ID)
	mustContain(t, body, "About LCpl. Okafor&#39;s order this turn: Deal with what, exactly?")
}
