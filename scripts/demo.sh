#!/bin/bash
set -e
tmp="${TMPDIR:-/tmp}"
dir="${1:-${tmp%/}/callboard-demo/pebble}"
if [ -e "$dir" ]; then
  echo "✗ $dir already exists; remove it or pick another folder: scripts/demo.sh DIR" >&2
  exit 1
fi

ago() {
  date -v-"$1"d +%Y-%m-%dT"$2" 2>/dev/null || date -d "$1 days ago" +%Y-%m-%dT"$2"
}

mkdir -p "$dir"
cd "$dir"
git init -q -b main
git config user.name "Pebble dev"
git config user.email dev@example.com
mkdir -p .callboard

cat > README.md <<'EOF'
# Pebble

A small habit tracker for iPhone and the web. A made-up project, to show Callboard.
EOF

cat > backlog.md <<EOF
# Pebble backlog

## Now

- [ ] Streak counter on the home screen <!-- id:B-k3f9 by:claude at:$(ago 2 09:12) prio:high area:app status:todo -->
  Count days in a row per habit; a missed day resets it, a skipped day doesn't.
- [ ] Offline sync with a local SQLite cache <!-- id:B-7tg8 by:codex at:$(ago 3 14:40) prio:high area:sync needs:Q-sy2c status:todo -->
- [ ] Sign in with Apple <!-- id:B-a1pl by:claude at:$(ago 4 10:05) prio:high area:auth needs:R-dv8k,Q-au7h status:todo -->
- [ ] Push reminders at the time each habit is due <!-- id:B-pu5h by:claude at:$(ago 4 10:20) prio:medium area:app needs:R-ap3n,B-a1pl status:todo -->
- [ ] Settings screen: reminder time, week start, units <!-- id:B-se4t by:codex at:$(ago 2 16:02) prio:medium area:app status:todo -->
- [x] Onboarding: three screens and a first habit <!-- id:B-on1b by:claude at:$(ago 6 11:00) done:$(ago 1 17:45) area:app status:done -->
- [x] Dark mode <!-- id:B-dk2m by:codex at:$(ago 6 11:10) done:$(ago 0 09:30) area:app status:done -->

## Next

- [ ] Share a streak as an image <!-- id:B-sh6r by:claude at:$(ago 5 15:00) prio:medium area:app needs:Q-im9d,B-k3f9 status:todo -->
- [ ] Weekly summary email <!-- id:B-wk3e by:claude at:$(ago 12 12:00) prio:low area:email needs:R-pm4k status:todo -->
- [ ] Export habits as CSV <!-- id:B-cs8v by:codex at:$(ago 15 09:00) prio:low area:web status:todo -->
- [ ] Haptic tap when a habit is checked off <!-- id:B-ht4p by:you at:$(ago 1 21:30) status:todo area:app -->
- [x] Charts for the last 30 days <!-- id:B-ch5t by:claude at:$(ago 9 10:00) done:$(ago 2 18:20) area:app status:done -->

## Later

- [ ] Apple Watch complication <!-- id:B-wa9c by:you at:$(ago 20 20:00) prio:low area:app status:todo -->
- [ ] Home screen widgets <!-- id:B-wd2g by:you at:$(ago 20 20:05) prio:low area:app status:todo -->
EOF

cat > requests.md <<EOF
# Requests

- [ ] Create the Apple Developer account and put the team id in .env as APPLE_TEAM_ID <!-- id:R-dv8k by:claude at:$(ago 4 10:06) -->
  1. Enrol at developer.apple.com/programs (99 USD a year).
  2. Copy the team id from Membership details into .env: APPLE_TEAM_ID=...
- [ ] Make an APNs key for push reminders <!-- id:R-ap3n by:claude at:$(ago 4 10:21) needs:R-dv8k -->
  In Certificates, Identifiers & Profiles → Keys, add a key with Apple Push Notifications, save the .p8 in ~/keys and set APNS_KEY_PATH in .env.
- [ ] Sign up for Postmark and set POSTMARK_TOKEN <!-- id:R-pm4k by:claude at:$(ago 12 12:01) -->
- [x] Buy pebble-habits.app <!-- id:R-dn1a by:claude at:$(ago 8 09:00) done:$(ago 3 20:10) -->
EOF

cat > questions.md <<EOF
# Questions

- [ ] Which sign-in should Pebble have at launch? <!-- id:Q-au7h by:claude at:$(ago 4 10:07) -->
  - Apple, plus an email link for the web (recommended)
  - Apple and Google
  - Email and password only
- [ ] Where should synced habits live? <!-- id:Q-sy2c by:codex at:$(ago 3 14:41) assumed:"Supabase" -->
  - Supabase: Postgres with auth and row-level security built in (recommended)
  - Our own Postgres on Fly.io
  - iCloud only, no server
- [ ] How should a shared streak look? <!-- id:Q-im9d by:claude at:$(ago 5 15:01) -->
  - A square card with the habit, the streak and a tiny chart (recommended)
  - A story-sized image with a big number
- [x] Which chart library? <!-- id:Q-ch3l by:claude at:$(ago 9 09:50) done:$(ago 8 21:00) answer:"Swift Charts" why:"built in, and it matches the system look" answered-by:you -->
  - Swift Charts (recommended)
  - DGCharts
EOF

cat > .callboard/views.md <<EOF
# Views

- [ ] By status <!-- id:V-bs7x by:you at:$(ago 3 09:00) list:backlog layout:board group:status order:todo,doing,done -->
- [ ] Most important first <!-- id:V-mi2p by:you at:$(ago 3 09:01) list:backlog layout:table sort:-prio show:prio,area -->
EOF

git add -A
git commit -q -m "Pebble: first lists"
echo "✓ made the demo project in $dir"
echo "  Look at it: cd $dir && callboard show   (or callboard serve --open)"
