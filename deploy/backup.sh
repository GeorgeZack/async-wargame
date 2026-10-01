#!/bin/sh
# Copies the campaign state to a dated, compressed backup and keeps the last
# 30 days. The server replaces the state file atomically, so a plain copy is
# always a consistent snapshot.
#   STATE   state file (default /var/lib/async-wargame/campaigns.json)
#   DEST    backup directory (default /var/backups/async-wargame)
#   KEEP    days to keep (default 30)
set -eu
STATE=${STATE:-/var/lib/async-wargame/campaigns.json}
DEST=${DEST:-/var/backups/async-wargame}
KEEP=${KEEP:-30}

[ -f "$STATE" ] || { echo "no state file at $STATE" >&2; exit 1; }
mkdir -p "$DEST"
chmod 700 "$DEST"
out="$DEST/campaigns-$(date -u +%Y%m%dT%H%M%SZ).json.gz"
gzip -c "$STATE" > "$out.tmp" && mv "$out.tmp" "$out"
find "$DEST" -name 'campaigns-*.json.gz' -type f -mtime +"$KEEP" -delete
echo "backed up to $out"
