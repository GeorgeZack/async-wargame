# Async Wargame

Referee-led asynchronous browser wargames. A Go server stores the campaign
and serves the pages; players and a referee take turns in the browser on
their own time, and an optional language model can judge a turn.

Work in progress.

## Run locally

```sh
mise install
go run .
```

Open http://127.0.0.1:8080. The campaign is stored in `data/campaigns.json`.
Copy `async-wargame.example.conf` to `async-wargame.conf` to set the listen
address, a host password, or an AI provider.

```sh
go test ./...
```

Go standard library only. The browser client is HTML, CSS and JavaScript.
