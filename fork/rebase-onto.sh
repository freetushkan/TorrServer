#!/usr/bin/env bash
# Move the fork commits onto a new upstream tag.
#
#   fork/rebase-onto.sh                 # onto the newest upstream MatriX.* tag
#   fork/rebase-onto.sh MatriX.148      # onto a given tag
#   fork/rebase-onto.sh --verify        # only build/test/check the current tree
#
# The fork is kept as "<upstream tag> + a few linear commits". The script
# rebases those commits from the tag they currently sit on (OLD_BASE, detected
# with git describe) onto the new tag in a new branch sync/<tag>, then builds,
# runs the fork tests and fork/check-peruser.sh. Your current branch is not
# modified. git rerere is enabled, so a conflict resolved once is replayed
# automatically next time.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

UPSTREAM_URL=${UPSTREAM_URL:-https://github.com/YouROK/TorrServer.git}
TAG_GLOB='MatriX.*'
FORK_TAG_GLOB='*.MU.*' # fork release tags, e.g. MatriX.147.1.MU.r1

verify() {
  echo "== go build"
  (cd server && go build -tags=nosqlite -o /dev/null ./cmd)
  echo "== go test (fork tests + web/api)"
  (cd server && go test -tags=nosqlite -count=1 -run 'PerUser|Viewed|LastUser|Startup' ./settings/ &&
    go test -tags=nosqlite -count=1 ./web/api/)
  echo "== hooks"
  bash fork/check-peruser.sh
  echo "== all checks passed"
}

if [ "${1:-}" = "--verify" ]; then
  verify
  exit
fi

if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "working tree is not clean" >&2
  exit 1
fi

git remote get-url upstream >/dev/null 2>&1 || git remote add upstream "$UPSTREAM_URL"
git fetch --quiet --no-tags upstream "+refs/tags/$TAG_GLOB:refs/tags/$TAG_GLOB"

NEW=${1:-$(git tag -l "$TAG_GLOB" | grep -v -- '.MU.' | sort -V | tail -1)}
OLD=${OLD_BASE:-$(git describe --tags --abbrev=0 --match "$TAG_GLOB" --exclude "$FORK_TAG_GLOB" HEAD)}
git rev-parse -q --verify "refs/tags/$NEW" >/dev/null || { echo "no tag $NEW" >&2; exit 1; }

echo "fork commits on $OLD:"
git log --oneline "$OLD..HEAD"
if [ "$(git rev-parse "$OLD^{commit}")" = "$(git rev-parse "$NEW^{commit}")" ]; then
  echo "already on $NEW"
  exit 0
fi
if git merge-base --is-ancestor "$NEW" "$OLD"; then
  echo "$NEW is older than the current base $OLD" >&2
  exit 1
fi

git config rerere.enabled true
git config rerere.autoupdate true

BR="sync/$NEW"
git switch --quiet -C "$BR" HEAD
echo "rebasing onto $NEW in branch $BR ..."
in_rebase() { [ -d "$(git rev-parse --git-path rebase-merge)" ] || [ -d "$(git rev-parse --git-path rebase-apply)" ]; }
git rebase --onto "$NEW" "$OLD" "$BR" || true
# rerere may already have resolved everything from an earlier run
for _ in 1 2 3 4 5 6 7 8 9 10; do
  in_rebase && [ -z "$(git diff --name-only --diff-filter=U)" ] || break
  echo "conflicts resolved from rerere cache, continuing"
  GIT_EDITOR=true git rebase --continue || true
done
if in_rebase; then
  git diff --name-only --diff-filter=U | sed 's/^/  conflict: /'
  cat <<EOF

Conflicts. Hints:
  - Upstream files only carry one-line hooks marked "fork: per-user data";
    take the upstream side and re-apply the hook (fork/README.md lists them).
  - New files (*peruser*) never conflict.
then:  git add <files> && git rebase --continue
and:   fork/rebase-onto.sh --verify
EOF
  exit 1
fi
verify
cat <<EOF

Done: $BR = $NEW + fork commits. To publish:
  git push --force-with-lease origin $BR:master
  git tag $NEW.MU.r1 && git push origin $NEW.MU.r1
EOF
