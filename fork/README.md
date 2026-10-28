# Fork notes

This fork is **an upstream release tag + a few linear commits** on top of it.
Upstream is never merged in: when a new tag is released, the fork commits are
rebased onto it, `master` is replaced, and the release is tagged
`<upstream tag>.MU.rN` (e.g. `MatriX.147.1.MU.r1`).

Fork commits:

- per-user data separation (`server/**/peruser*.go`, `PerUserSettings.jsx` and hooks);
- fork tooling: this directory and `.github/workflows/fork-upstream-sync.yml`.

## Updating to a new upstream tag

One-time setup: enable **Settings → General → Features → Issues** (disabled in
forks by default) and make sure **Actions** are enabled.

From the GitHub UI:

1. The **Fork upstream sync** workflow checks YouROK/TorrServer tags daily
   (or run it manually: **Actions → Fork upstream sync → Run workflow**, empty inputs).
2. It opens an issue:
   - `Upstream <tag>: ready` — rebased, built and checked; branch `sync/<tag>` is pushed;
   - `Upstream <tag>: needs attention` — conflicts or a failed check; resolve locally (below).
3. **Run workflow** again with `tag = <tag>` and **publish** checked: `master`
   becomes `sync/<tag>` and the issue is closed.
4. **Releases → Draft a new release → Choose a tag** → `<tag>.MU.r1` →
   *Create new tag*, target `master` → **Publish release**. This triggers
   *Build and Release*, which attaches the binaries.

GitHub disables scheduled workflows after 60 days of repository inactivity;
re-enable it on the workflow page.

Locally (or in a Codespace), required when there are conflicts:

```sh
bash fork/rebase-onto.sh              # onto the newest MatriX.* tag
bash fork/rebase-onto.sh MatriX.148   # onto a given tag
# on conflicts: fix, git add …, git rebase --continue, then
bash fork/rebase-onto.sh --verify     # build + fork tests + check-peruser.sh
git push --force-with-lease origin sync/MatriX.148:master
git tag MatriX.148.MU.r1 && git push origin MatriX.148.MU.r1   # triggers ts_release
```

The script enables `git rerere`, so a conflict resolved once is replayed on
the next attempt.

Do not push upstream tags to the fork (`git push --tags`): `ts_release.yml`
builds a release for every pushed tag.

## Resolving conflicts

All logic lives in fork-only files, which never conflict:

| file | contents |
|---|---|
| `server/settings/peruser.go` | flag/setting, account list, viewed keys, LastUser, owner storage, startup migration |
| `server/torr/peruser.go` | `AddTorrentForUser`, `RemTorrentForUser`, `ListTorrentForUser` |
| `server/web/api/peruser.go` | `currentUser`, `requestUser`, `userParam`, `settingsForUser` |
| `server/gstreamer/peruser.go` | records the user who starts a GStreamer probe/transcode |
| `web/src/components/Settings/PerUserSettings.jsx` | settings switch and its translations |

Upstream files only carry one-line hooks marked `fork: per-user data`. On a
conflict, **take the upstream side and re-apply the hook**; the full list of
hooks with their exact text is at the top of `fork/check-peruser.sh`. If
upstream rewrote the function, put the hook in the equivalent place.

`fork/check-peruser.sh` verifies that:

1. every hook is present (a conflict resolved in favour of upstream can drop
   one silently while everything still compiles);
2. no HTTP handler reaches torrents or viewed marks around the per-user
   wrappers — the usual way isolation leaks when upstream adds an endpoint.
   Intentional exceptions are listed in `fork/peruser-allow.txt` as whole code
   lines, so a change to such a line upstream is flagged again.

The web UI is built with `CI=true`, where lint warnings are errors. Imports
must be ordered: packages, blank line, relative paths.

## Per-user data separation

Enabled by the `--peruserdata` flag (forced) or the "Per-user data separation"
switch in settings. Only meaningful with `--httpauth`; accounts come from `accs.db`.

Storage (bbolt/json through the shared `tdb`):

| bucket | key | value |
|---|---|---|
| `TorrentUsers` | hash | JSON array of owner accounts |
| `Viewed` | `<user>:<hash>` | the user's viewed marks (plain `<hash>` = shared) |
| `LastUser` | hash | last account that listed viewed marks / preloaded |
| `PerUser` | `owners_migrated` | the legacy `TorrentDB.users` field has been imported |

Values are JSON objects: buckets without a route of their own are stored in
the default DB, which is JSON when settings are kept in JSON, and the JSON DB
only stores objects.

To recover owners from a backup of a `config.db` that still has the legacy
`TorrentDB.users` field, put it next to `config.db` as `peruser-import.db` and
restart: owners are imported for torrents that have none, and the file is
renamed to `peruser-import.db.imported`.

Rules:

- a torrent without owners is shared and visible to every account;
- adding a new torrent makes the user its only owner; adding an existing owned
  torrent adds the user to its owners; a shared torrent stays shared;
- all owners are equal; removing a torrent removes the user from its owners,
  and the torrent itself is deleted when no owners are left;
- a user cannot remove a torrent they do not own;
- accounts no longer in `accs.db` are ignored; a torrent whose owners are all
  gone becomes shared;
- viewed marks are per user; shared marks made before the mode was enabled are
  copied to a user on first access;
- a request without credentials acts for `?user=` or, failing that, for the
  last user whose authenticated request touched the torrent (stream, play,
  viewed, ffprobe, GStreamer; stored in `LastUser`). Such requests are common:
  TorrServer's own loopback reads (preload/ffprobe, GStreamer) carry no
  credentials, and FFmpeg-based players (mpv, VLC) keep `user:pass@` from the
  URL but send it only after a 401, which a known torrent never triggers.
  With FFmpeg, `auth_type=basic` (mpv: `stream-lavf-o=auth_type=basic`) sends
  the credentials up front and makes the attribution exact.
  Playlists carry `&user=<login>`. `?user=` only affects viewed marks;
  ownership is changed by authenticated requests only.

Not isolated (see all accounts' data): DLNA, WebDAV/FUSE (`torrfs`), the
Telegram bot, MCP, and torrent `get`/`set` by hash.
