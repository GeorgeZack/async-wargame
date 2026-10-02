# Async Wargame

Browser-based, referee-led asynchronous wargames. See [design-doc.md](design-doc.md).

## Run

```sh
mise install               # installs the pinned Go toolchain (see mise.toml)
go run .                   # http://127.0.0.1:8080
go run . -addr :8080       # accept connections from other machines on the LAN
go test ./...
```

State is kept in `data/campaigns.json` (`-data` to change).

### Hosting

Run one copy on an always-on machine: the state lives in one file, and a
clock in the server locks turns on schedule. Put it behind a reverse proxy
or tunnel that provides HTTPS, and tell it so:

```sh
AW_HOST_PASSWORD='something long' ./async-wargame -addr 127.0.0.1:8080 -proxy -data /var/lib/async-wargame/campaigns.json
```

- `-proxy` trusts the proxy's `X-Forwarded-Proto` (so login cookies are
  marked Secure) and `X-Forwarded-For`. Leave it off if the server faces the
  internet directly.
- `AW_HOST_PASSWORD` is needed to create a room; players joining don't need
  it. Ten wrong guesses from one address lock it out for an hour. Without it,
  anyone who can reach the server can create rooms.
- Back up the `-data` file.

[deploy/](deploy/README.md) has a systemd unit, a Caddyfile, nightly
backups, a one-command `deploy.sh`, and step-by-step setup for a small VPS.

### The AI assistant

The referee's assistant can use a language model through any
OpenAI-compatible chat API. Pick one with `-ai`:

```sh
# OpenCode Zen (an account and API key from opencode.ai; free models included)
AW_AI_KEY=... ./async-wargame -ai opencode -ai-model big-pickle

# Ollama running on the same machine
./async-wargame -ai ollama -ai-model llama3.1

# A model file run by the server itself, via llama.cpp's llama-server
./async-wargame -ai local -ai-local-model /srv/models/qwen3-8b-q4_k_m.gguf

# Anything else that speaks the API: LM Studio, vLLM, a llama-server you run yourself
./async-wargame -ai http://127.0.0.1:1234/v1 -ai-model my-model
```

- **OpenCode Zen** needs `AW_AI_KEY`: without one, it refuses with "free tier
  can only be used from within OpenCode". At startup the server checks the
  model exists and lists the free ones if it doesn't. Free models change,
  and several log prompts or use them for training (see
  [opencode.ai/docs/zen](https://opencode.ai/docs/zen/)).
- **`-ai local`** starts `llama-server` (from [llama.cpp](https://github.com/ggml-org/llama.cpp);
  `-ai-llama-server` if it isn't on the `PATH`) on a private port with your
  GGUF model, waits for it to load, and stops it on shutdown. `-ai-context`
  sets the context size (default 8192). A 7–8B model at 4-bit
  quantization needs about 6 GB of RAM and is slow without a GPU, but the
  assistant works in the background, so slow is fine.
- Nothing is sent until the referee switches the assistant on for their
  room (Room screen). What's sent: unit names and stats, orders and their
  intent, positions, and report lines. Not messages, and not anyone's name.

## How it plays

A referee-led, old-school game played over days: players say what their
characters do in their own words, the referee decides what happens, and the
dice decide only how well things went where the outcome is in doubt. The
starting scenario, **St. Ines**, is four colonial marines (a squad leader and
three marines, each played by one person) plus a referee: land at Red Beach,
cross the canal, reach the hospital, and bring the survivors out.

- **Orders are free text.** "Okafor crosses the bridge low and fast, stacks up
  on the side door, and if the shamblers turn, tosses a grenade and pulls
  back." A spot on the map can be marked, but nothing else is required.
  Orders are drafted, committed, and frozen at the lock; the squad leader can
  approve one or send it back with a note.
- **Unit cards** show a marine's usual bonus (quality, plus veterancy),
  kit, status, notes, and established facts. The card doesn't list
  everything, and doesn't need to: the referee interprets.
- **Facts are co-created.** Players propose something true about their
  marine or the world from a card ("the hospital has a loading dock on the
  canal side"); the referee accepts it, changes it, or turns it down on the
  Notes screen. Accepted facts show on the cards and in "What we know" on
  the map screen (or stay private between one player and the referee).
- **The dice:** when a situation is in doubt, each side rolls a d10 plus one
  bonus, everything that matters summed (quality, veterancy, cover, ambush,
  weather). The result is just the totals, "Okafor 5, the horde 9", and the
  referee reads them against what each side wanted, like an old Braunstein
  game. The server rolls, or the referee enters dice from the table.
- **The desk.** At the lock, every order lands on the referee's Turn screen.
  For each, the referee rolls where needed, writes what happened, and
  chooses who hears about it (the whole squad, only that player, or nobody
  yet). They move counters and change units (status, skills, kit, notes,
  radio) as they go; players keep seeing the board as it stood at the lock
  until the turn is published. Secret rolls stay on the referee's side of
  the report.
- **The assistant (optional):** an AI model can suggest rolls from the
  orders (who rolls, and what counts for each side), draft outcomes from the rolls, and
  draft an opening narrative for each report. Suggestions sit unrolled until
  the referee rolls them; drafts until the referee keeps or rewrites them.
- **Fog and radios, by hand.** The referee decides what players know about:
  revealing a unit shows it as a contact, identifying it says what it is. A
  marine whose radio is out keeps their orders to themselves, and the rest
  of the squad sees them where they last called in.
- **Random events** (stragglers, runners, a screamer, the brute, a drone
  overflight, radio static) are proposals for the referee to apply or
  discard, with odds by threat level in the editor.
- **Reports** show each order with its outcome and any rolls players may
  see, plus anything else that happened. Until the next lock, the referee
  can reopen a published turn to correct it, with a reason players see.
- **The rest carries over:** rooms and room codes, referee approval and
  invite links, a squad leader (commander) and the players under them, test
  seats for trying it from several tabs, messages (all hands, players only,
  the command, private with the referee), web push notifications, the
  scenario editor, scheduled locks in the room's time zone, leaving and
  handing on units, and campaigns of several operations where survivors keep
  what happened to them.

A blind referee doesn't see players' orders until the lock (admin view shows
them, and players are told).

What's next: [docs/roadmap.md](docs/roadmap.md). Writing your own setting:
[docs/modules.md](docs/modules.md).

## Layout

```
main.go                       flags, wiring
content/                      embedded module + scenario JSON
internal/rules/               module schema and the dice (d10 + bonus)
internal/game/                campaigns: rooms, orders, resolution, fog, views, store
internal/web/                 HTTP handlers, templates, static JS/CSS
internal/push/                web push: VAPID and payload encryption
internal/ai/                  OpenAI-compatible model client; managed llama-server
internal/assist/              background worker for the referee assistant
deploy/                       systemd, Caddy, Tailscale, backups, deploy script
```
