#!/usr/bin/env bash
# Sanity check of the per-user data hooks after rebasing onto a new upstream tag.
#
#  1. every hook the feature needs is still present (a conflict resolved in
#     favour of upstream can silently drop one while everything still compiles);
#  2. HTTP handlers do not reach torrents / viewed marks around the per-user
#     wrappers (new upstream endpoints are the usual way isolation leaks);
#  3. informational: other front-ends that see every account's data.
#
# Exit code 1 if (1) or (2) found a problem. Run from anywhere inside the repo.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"
ALLOW=fork/peruser-allow.txt
fail=0
red() { printf '\033[31m%s\033[0m\n' "$*"; }

echo "== 1. hooks present"
# file|fixed string that must occur in it
while IFS='|' read -r file needle; do
  [[ -z "$file" || "$file" == \#* ]] && continue
  if ! grep -qF -- "$needle" "$file" 2>/dev/null; then
    red "MISSING  $file: $needle"; fail=1
  fi
done <<'EOF'
server/server.go|settings.PerUserStartup()
server/settings/args.go|PerUserData bool
server/settings/btsets.go|PerUserData      bool
server/cmd/main.go|PerUserData: params.PerUserData
server/torr/stream.go|sets.ViewedKeyForRequest(
server/torr/stream.go|Hash:      viewedKey,
server/web/api/torrents.go|torr.AddTorrentForUser(currentUser(c)
server/web/api/torrents.go|torr.ListTorrentForUser(currentUser(c))
server/web/api/torrents.go|torr.RemTorrentForUser(req.Hash, currentUser(c))
server/web/api/torrents.go|torr.RemTorrentForUser(hash, currentUser(c))
server/web/api/upload.go|torr.AddTorrentForUser(currentUser(c)
server/web/api/stream.go|requestUser(c, spec.InfoHash.HexString(), "")
server/web/api/stream.go|requestUser(c, spec.InfoHash.HexString(), lastPreload)
server/web/api/stream.go|rememberPreloadUser(
server/web/api/play.go|requestUser(c, "", "")
server/web/api/m3u.go|torr.ListTorrentForUser(user)
server/web/api/m3u.go|requestUser(c, tor.Hash().HexString(), lastViewed)
server/web/api/m3u.go|userParam(user) + "&index="
server/web/api/m3u.go|sets.ListViewedForUser(tor.Hash, user)
server/web/api/viewed.go|sets.SetViewedForUser(
server/web/api/viewed.go|sets.RemViewedForUser(
server/web/api/viewed.go|sets.ListViewedForUser(
server/web/api/settings.go|settingsForUser(c)
web/src/components/Settings/SecondarySettingsComponent.jsx|<PerUserSettings
EOF
[ $fail = 0 ] && echo "ok"

echo "== 2. unscoped access in HTTP handlers"
pattern='torr\.(ListTorrent|AddTorrent|RemTorrent)\(|\bsets?\.(SetViewed|RemViewed|ListViewed)\(|c\.JSON\(200, sets\.BTsets\)'
found=0
while IFS= read -r hit; do
  file=${hit%%:*}; rest=${hit#*:}; code=${rest#*:}
  code=$(sed -E 's/^[[:space:]]+//' <<<"$code")
  if grep -qxF -- "$file: $code" "$ALLOW" 2>/dev/null; then continue; fi
  red "UNSCOPED $hit"; found=1
done < <(grep -rnE "$pattern" server/web --include='*.go' | grep -v '_test.go:' | grep -v '/peruser.go:')
if [ $found = 1 ]; then
  echo "  -> wrap it (AddTorrentForUser/ListTorrentForUser/RemTorrentForUser/*ViewedForUser)"
  echo "     or, if it is fine to ignore the user there, add the line to $ALLOW as"
  echo "     '<file>: <trimmed code>'"
  fail=1
else
  echo "ok"
fi

echo "== 3. info: front-ends that see all accounts (not isolated by design)"
for d in server/mcp server/tgbot server/dlna server/torrfs; do
  [ -d "$d" ] || continue
  n=$(grep -rnE "$pattern" "$d" --include='*.go' 2>/dev/null | grep -vc '_test.go:')
  printf '  %-16s %s call sites\n' "$d" "$n"
done
for d in $(git ls-files server | grep -E '\.go$' | xargs grep -lE "$pattern" 2>/dev/null | xargs -r -n1 dirname | sort -u); do
  case "$d" in server/web*|server/mcp|server/tgbot|server/dlna|server/torrfs|server/torr|server/settings|server/cmd) ;;
    *) echo "  NEW?             $d" ;;
  esac
done

exit $fail
