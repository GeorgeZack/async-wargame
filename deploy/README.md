# Deploying

The server is one binary that keeps every campaign in one JSON file and
runs a clock that locks turns on schedule. So it needs **one always-on
machine**: no serverless or scale-to-zero hosting, and only one copy at a
time.

| File | What it is |
|---|---|
| `async-wargame.service` | systemd unit: runs the server as its own user, sandboxed, state in `/var/lib/async-wargame` |
| `Caddyfile` | Caddy in front: HTTPS with automatic certificates, live updates passed straight through |
| `backup.sh`, `async-wargame-backup.{service,timer}` | nightly compressed copy of the state, 30 days kept |
| `deploy.sh` | from your machine: test, build for Linux, upload, restart |

Two ways to put HTTPS in front: a domain with Caddy (below), or
[Tailscale](#tailscale-instead-of-a-domain-and-caddy), private to your
tailnet or public through Funnel.

## Quick test without a server

To try it with friends for an evening, run it on your own machine behind a
tunnel. It only runs while your computer is on, so turns won't lock on
schedule while it's off.

```sh
AW_HOST_PASSWORD='pick-one' go run . -proxy -data ./playtest.json
cloudflared tunnel --url http://localhost:8080   # prints an https://….trycloudflare.com link
```

## A small VPS

Any small Linux VPS works (1 CPU, 1 GB is plenty): Hetzner, DigitalOcean,
Linode, Vultr. These steps are for Debian or Ubuntu; run them as root on the
server.

**1. A domain name.** Point an A record (and AAAA, if the server has IPv6)
for, say, `game.yourdomain.com` at the server's address.

**2. Firewall.** Allow SSH, HTTP, and HTTPS only:

```sh
apt install -y ufw
ufw allow OpenSSH && ufw allow 80,443/tcp && ufw enable
```

**3. A user and the password file.**

```sh
useradd --system --home /var/lib/async-wargame --shell /usr/sbin/nologin async-wargame
mkdir -p /etc/async-wargame
printf 'AW_HOST_PASSWORD=%s\nAW_PUSH_CONTACT=mailto:you@example.com\n' "$(openssl rand -base64 18)" > /etc/async-wargame/env
chmod 600 /etc/async-wargame/env
cat /etc/async-wargame/env    # note the password: you need it to create rooms
```

**4. The service.** From your machine, in the repository:

```sh
scp deploy/async-wargame.service root@SERVER:/etc/systemd/system/
mise exec -- deploy/deploy.sh root@SERVER   # builds, uploads, starts
```

That starts the server. Make it start on boot too:

```sh
ssh root@SERVER systemctl enable async-wargame
```

**5. Caddy for HTTPS.**

```sh
apt install -y caddy     # Debian 12+/Ubuntu 22.04+; older: see caddyserver.com/docs/install
```

Copy `deploy/Caddyfile` to `/etc/caddy/Caddyfile`, replace
`game.example.com` with your domain, and `systemctl reload caddy`. Caddy
gets a certificate on the first request. Open `https://game.yourdomain.com`.

**6. Backups.**

```sh
scp deploy/backup.sh root@SERVER:/usr/local/bin/async-wargame-backup
scp deploy/async-wargame-backup.service deploy/async-wargame-backup.timer root@SERVER:/etc/systemd/system/
ssh root@SERVER 'chmod 755 /usr/local/bin/async-wargame-backup && systemctl daemon-reload && systemctl enable --now async-wargame-backup.timer'
```

Backups land in `/var/backups/async-wargame` on the same server. Copy them
off it from time to time (`scp`, or your provider's snapshot feature): a
backup on the machine that died doesn't help.

## Tailscale instead of a domain and Caddy

With [Tailscale](https://tailscale.com), the server gets HTTPS and a name
like `wargame.your-tailnet.ts.net` without a domain, DNS records, Caddy, or
open ports. It can run on a VPS or on a spare machine at home. Do steps 3, 4
and 6 above (user and password file, service, backups) and skip 1, 2 and 5.

```sh
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --hostname wargame
```

Then pick who can reach it:

**Only your tailnet (private).** Players install Tailscale and you share the
machine with them (admin console → the machine → Share), or invite them to
your tailnet.

```sh
tailscale serve --bg --https=443 localhost:8080
tailscale serve status        # shows the https://wargame.….ts.net address
```

**Anyone with the link (public), via Funnel.** Players need nothing but a
browser. Keep `AW_HOST_PASSWORD` set: the site is on the open internet.

```sh
tailscale funnel --bg 8080
tailscale funnel status
```

Notes:

- The first time, the CLI may ask you to turn on HTTPS certificates (and, for
  Funnel, to allow Funnel in your tailnet policy) and prints a link to do it.
- With `--bg` both survive reboots. Turn them off with
  `tailscale serve --https=443 off` or `tailscale funnel --https=443 off`
  (or clear everything with `tailscale serve reset`).
- Tailscale sends `X-Forwarded-Proto` and `X-Forwarded-For`, so keep `-proxy`
  in the service, and keep the server on `127.0.0.1`.
- Funnel traffic has bandwidth limits Tailscale doesn't publish; a few
  players' text and small maps are well within them.
- If you leave `ufw` on, Tailscale doesn't need any ports opened for it.

## Updating

```sh
mise exec -- deploy/deploy.sh root@SERVER
```

It runs the tests, builds, uploads, and restarts. Restarting is safe at any
time: state is written atomically after every change, and browsers
reconnect to live updates by themselves.

## Restoring

```sh
systemctl stop async-wargame
gunzip -c /var/backups/async-wargame/campaigns-YYYYMMDDTHHMMSSZ.json.gz > /var/lib/async-wargame/campaigns.json
chown async-wargame: /var/lib/async-wargame/campaigns.json
systemctl start async-wargame
```

## Notes

- Notifications (web push) need HTTPS, which both Caddy and Tailscale give
  you. Put a real address in `AW_PUSH_CONTACT` (`mailto:` or `https:`): push
  services use it if something's wrong with your pushes. The server's push
  key is `/var/lib/async-wargame/vapid.pem`; if it's lost, everyone has to
  turn notifications on again.

- `-proxy` makes the server trust `X-Forwarded-Proto` and `X-Forwarded-For`
  from Caddy. Keep the server listening on `127.0.0.1` so nothing else can
  reach it and forge them.
- Without `AW_HOST_PASSWORD`, anyone who finds the site can create rooms.
- Logs: `journalctl -u async-wargame -f`.
