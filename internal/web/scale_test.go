package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"asyncwargame/internal/game"
)

func TestHundredConcurrentPlayersOrdersAndLiveUpdates(t *testing.T) {
	ts, store := newTestServer(t)
	code, err := store.Create("HTTP load", "st-ines", "Ref", game.HashToken("scale-ref"))
	if err != nil {
		t.Fatal(err)
	}
	tokens := make([]string, 100)
	err = store.Update(code, func(r *game.Room) error {
		mod, ref := store.Cat.Modules[r.ModuleID], r.Referee()
		for i := range tokens {
			tokens[i] = fmt.Sprintf("scale-seat-%03d", i)
			m, e := r.RequestJoin(fmt.Sprintf("Player %03d", i), game.HashToken(tokens[i]))
			if e != nil {
				return e
			}
			if e = r.Decide(ref, m.ID, "approve"); e != nil {
				return e
			}
			m.Briefed = true
			u := *r.Unit("m1")
			u.ID = fmt.Sprintf("load-%03d", i)
			u.Name = u.ID
			u.Formation = ""
			u.Brief = fmt.Sprintf("PRIVATE-SEAT-%03d", i)
			u.AssignedTo = m.ID
			u.X += float64(i%10) * 3
			u.Y += float64(i/10) * 3
			r.Units = append(r.Units, &u)
		}
		r.RefereeNotes = "SCALE-REFEREE-SECRET"
		r.Unit("z3").Name = "SCALE-HIDDEN-ENEMY"
		return r.StartOperation(ref, mod, time.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := &http.Client{}
	request := func(i int, method, path, body string) (*http.Response, error) {
		req, e := http.NewRequestWithContext(ctx, method, ts.URL+"/r/"+code+path, strings.NewReader(body))
		if e != nil {
			return nil, e
		}
		req.AddCookie(&http.Cookie{Name: cookiePrefix + code, Value: tokens[i]})
		req.Header.Set("Origin", ts.URL)
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("X-Requested-With", "fetch")
		}
		return client.Do(req)
	}
	// All 100 SSE subscriptions stay connected while orders and reads run.
	streams := make([]*http.Response, 100)
	readers := make([]*bufio.Reader, 100)
	var wg sync.WaitGroup
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, e := request(i, "GET", "/live", "")
			if e != nil {
				t.Error(e)
				return
			}
			if res.StatusCode != 200 {
				res.Body.Close()
				t.Errorf("live status %d", res.StatusCode)
				return
			}
			streams[i] = res
			readers[i] = bufio.NewReader(res.Body)
			for {
				line, e := readers[i].ReadString('\n')
				if e != nil {
					t.Error(e)
					return
				}
				if line == "\n" {
					break
				}
			}
		}(i)
	}
	wg.Wait()
	defer func() {
		for _, r := range streams {
			if r != nil {
				r.Body.Close()
			}
		}
	}()
	timings := make([]float64, 100)
	for i := range tokens {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := time.Now()
			body := url.Values{"unit": {fmt.Sprintf("load-%03d", i)}, "text": {"Hold position and cover the sector."}, "submit": {"commit"}}.Encode()
			res, e := request(i, "POST", "/orders", body)
			if e != nil {
				t.Error(e)
				return
			}
			data, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusNoContent {
				t.Errorf("order %d: %d %s", i, res.StatusCode, data)
				return
			}
			res, e = request(i, "GET", "", "")
			if e != nil {
				t.Error(e)
				return
			}
			data, _ = io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != 200 {
				t.Errorf("page %d: %d", i, res.StatusCode)
			}
			text := string(data)
			for _, secret := range []string{"SCALE-REFEREE-SECRET", "SCALE-HIDDEN-ENEMY", fmt.Sprintf("PRIVATE-SEAT-%03d", (i+1)%100)} {
				if strings.Contains(text, secret) {
					t.Errorf("seat %d leaked %s", i, secret)
				}
			}
			if !strings.Contains(text, fmt.Sprintf("PRIVATE-SEAT-%03d", i)) {
				t.Errorf("seat %d lost own brief", i)
			}
			timings[i] = time.Since(start).Seconds()
			if readers[i] != nil {
				line, e := readers[i].ReadString('\n')
				if e != nil || line != "event: update\n" {
					t.Errorf("seat %d missed live update: %q %v", i, line, e)
				}
			}
		}(i)
	}
	wg.Wait()
	store.Read(code, func(r *game.Room) {
		n := 0
		for _, o := range r.Orders {
			if o.Status == game.OrderCommitted {
				n++
			}
		}
		if n != 100 {
			t.Errorf("lost concurrent orders: %d", n)
		}
	})
	slices.Sort(timings)
	t.Logf("100 connected players, concurrent POST + full page: p50 %.3fs p95 %.3fs max %.3fs", timings[49], timings[94], timings[99])
}
