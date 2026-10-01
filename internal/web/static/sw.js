// Service worker: shows web push notifications and opens the right screen
// when one is tapped. It does no caching; the game always loads fresh.
"use strict";

// Pages tell us which address their seat uses for each room ("/r/CODE" or a
// seat-key address), so a tap opens the same seat. It's kept in the browser.
const BASES = "aw-bases";

self.addEventListener("message", (e) => {
  const { room, base } = e.data || {};
  if (!room || !base || !base.startsWith("/r/")) return;
  e.waitUntil(caches.open(BASES).then((c) => c.put("/base/" + room, new Response(base))));
});

self.addEventListener("push", (e) => {
  let n = {};
  try { n = e.data ? e.data.json() : {}; } catch { n = { title: "Async Wargame", body: e.data ? e.data.text() : "" }; }
  e.waitUntil(self.registration.showNotification(n.title || "Async Wargame", {
    body: n.body || "",
    tag: n.tag || undefined,
    renotify: !!n.tag,
    data: { room: n.room, screen: n.screen || "" },
  }));
});

self.addEventListener("notificationclick", (e) => {
  e.notification.close();
  const { room, screen } = e.notification.data || {};
  e.waitUntil((async () => {
    let base = "/";
    if (room) {
      base = "/r/" + room;
      const hit = await (await caches.open(BASES)).match("/base/" + room);
      if (hit) base = await hit.text();
    }
    const path = !room ? "/" : (screen && screen !== "map" ? base + "/" + screen : base);
    const url = new URL(path, self.location.origin).href;
    const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const w of wins) {
      if (room && new URL(w.url).pathname.startsWith(base)) {
        await w.focus();
        return w.navigate(url);
      }
    }
    return self.clients.openWindow(url);
  })());
});
