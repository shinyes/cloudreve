# Merging upstream work into this fork

This fork is based on upstream **4.19.1** (`a8becb9f`) and keeps a real git ancestry link to
it, so upstream changes merge as ordinary three-way merges. This file is the procedure to
follow, and the record of what makes it possible.

## Baseline

| | |
|---|---|
| Upstream repository | `cloudreve/Cloudreve` |
| Baseline commit | `a8becb9f` (tag `4.19.1`) |
| Relationship | upstream is an ancestor of `main`; `git merge-base main upstream/master` reports `a8becb9f` |
| Our changes on top | 26 new files and 49 modified files (backend and CI); the frontend under `assets/` is a larger, separate divergence |

Check the link is still intact before merging:

```bash
git merge-base --is-ancestor upstream/master main && echo "ancestry ok"
```

If that fails, the ancestry was lost (for example by re-importing upstream sources instead
of merging them) and this procedure no longer applies cleanly.

## Development rules for this fork

These are the standing rules for anyone - human or AI - adding code here. They exist
because the fork has to stay mergeable with a project that keeps moving.

1. **Never re-import upstream sources.** Upstream code enters this repository only through
   `git merge upstream/master`. Copying upstream files over the tree and committing them
   severs the shared ancestor, and the damage is invisible until the next merge attempt.
   `.github/workflows/upstream-guard.yml` asserts the ancestry on every push for exactly
   this reason.
2. **Keep fork changes identifiable.** New behaviour goes in new files where practical, and
   edits to upstream files stay as small as the change allows. The conflict surface is the
   list of files we touched, so a smaller surface means cheaper upgrades.
3. **Do not rewrite published history.** No `rebase`, `filter-branch` or force-push on
   `main` beyond the one-time reconnection described at the end of this file. Rewriting
   changes every commit id and invalidates the ancestry the guard checks.
4. **Never add a pushable upstream remote.** See `UPSTREAM_POLICY.md`.
5. **Record the baseline when it moves.** After merging an upstream release, update the
   baseline table above and `UPSTREAM_BASELINE` in the guard workflow, so the next person
   can tell what this fork is based on without archaeology.
6. **Re-run the post-merge checklist after every merge**, not just the Go build. The
   relocation suites and the frontend build are what catch an upstream change that quietly
   breaks this fork's features.
7. **Keep this file current.** If an upgrade needs a step that is not written here, add it
   in the same commit that discovers it.

## The upstream remote is fetch-only

```bash
git remote -v
# origin    git@github.com:shinyes/cloudreve.git        (fetch/push)
# upstream  https://github.com/cloudreve/Cloudreve.git  (fetch)
# upstream  no-push-per-upstream-policy                 (push)
```

The `pushurl` is a placeholder that does not resolve, so git cannot push through this
remote even by accident, while fetching works normally. `UPSTREAM_POLICY.md` explains why
nothing may ever be published upstream, and `.github/workflows/upstream-guard.yml` fails a
build if a pushable upstream remote ever appears.

To recreate the mirror in a fresh clone:

```bash
git remote add upstream https://github.com/cloudreve/Cloudreve.git
git config remote.upstream.pushurl "no-push-per-upstream-policy"
git config remote.upstream.tagOpt --no-tags
git config --add remote.upstream.fetch "+refs/heads/*:refs/remotes/upstream/*"
git config core.hooksPath .githooks
```

## Merging an upstream release

```bash
git fetch upstream master --no-tags
git log --oneline main..upstream/master        # what is coming

git switch -c merge/upstream-<version> main
git merge upstream/master                       # a normal three-way merge
```

Resolve conflicts, then work through the checklist below before committing the merge.

### Post-merge checklist

Run all of it on the merge commit, not on pieces of it. Each item states what it protects,
so a failure tells you which of this fork's features the upstream change broke.

**1. It compiles**

```bash
go build ./...
go vet ./inventory/ ./pkg/filemanager/...
```

For formatting, check the files this fork touched rather than the whole tree, since several
upstream files are not gofmt-clean by design (`ent/` is generated,
`pkg/webdav/internal/xml` and `application/migrator/model` are upstream's):

```powershell
gofmt -l (git ls-files '*.go' | Select-String -NotMatch '^ent/')
```

`go vet` reports a few pre-existing unkeyed-struct warnings in the drivers; anything new is
worth a look.

**2. This fork's own tests pass**

```bash
go test ./inventory/ ./pkg/filemanager/...
```

These cover the things upstream has no tests for and would not notice breaking: the
many-to-many group/policy binding, the group backfill patch, the three-state encryption
metadata contract on relocation, and the encryption matrix of a move.

**3. The relocation workflow still works end to end**

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File tests\relocate\run.ps1
```

Expect **21 + 11 + 8** assertions to pass. This is the only check that exercises real
blobs, real S3-driver code paths and rollback; a green `go build` says nothing about it.
Rebuild the binary first if the frontend changed.

**4. The database still migrates**

```bash
# fresh instance: must start and report the new version
./cloudreve.exe          # then: curl /api/v4/site/ping
```

A patch whose `EndVersion` is at or below the recorded version is skipped, so a schema
change merged from upstream that expects a new patch needs one added to
`inventory/migration.go` with a bumped `BackendVersion`. Start an instance against a copy of
an **existing** database as well, not just a fresh one: that is what catches a merge that
changed the schema without a patch.

**5. The embedded frontend still matches the backend**

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File assets\build-frontend.ps1 -BackendVersion <version>
go build -o cloudreve.exe .
```

Then check the log on startup for `Static resource version mismatch`; the server refuses to
serve a UI whose `version.json` disagrees with `BackendVersion`, and the mismatch is easy to
create by bumping one of the three places and not the others.

**6. The fork's UI is still there**

Open the file manager and confirm the preference picker and the relocate dialog still render
and still have their strings. A merge that takes upstream's `assets/` wholesale silently
removes them, and nothing else in this checklist would notice.

**7. Nothing upstream is missing**

```bash
git merge-base --is-ancestor upstream/master HEAD && echo "upstream is merged"
git rev-list --count HEAD..upstream/master      # expect 0 after merging a release
```

**8. Only then commit, bump and tag**

`BackendVersion` in `application/constants/constants.go`, the `$BackendVersion` default in
`assets/build-frontend.ps1`, and the git tag must all agree, and the tag must stay above
upstream's numbering. Update the baseline table at the top of this file and
`UPSTREAM_BASELINE` in `.github/workflows/upstream-guard.yml` in the same commit.

### Conflicts to expect

The files this fork has actually changed are the ones that can conflict. They are worth
looking at first when a merge gets noisy:

- **Group/policy model**: `ent/schema/group.go`, `ent/schema/policy.go`, the generated
  `ent/group*`, `ent/storagepolicy*`, `ent/migrate/schema.go`, `inventory/group.go`,
  `inventory/policy.go`, `inventory/migration.go`, `inventory/types/types.go`
- **File and upload paths**: `inventory/file.go`, `pkg/filemanager/fs/dbfs/*`,
  `pkg/filemanager/fs/fs.go`, `pkg/filemanager/manager/*`
- **Routing and API**: `routers/router.go`, `routers/controllers/*`,
  `service/explorer/*`, `service/user/policy.go`
- **Drivers**: `pkg/filemanager/driver/s3/s3.go`, `driver/local/local.go`

Generated ent code will conflict whenever the schema does. Regenerate rather than hand
merge: `go generate ./ent/...`, or re-run the ent codegen the repository already uses.

## The frontend is a deliberate exception

Upstream keeps the frontend in a git submodule (`.gitmodules` -> `cloudreve/frontend`).
This fork vendors it as ordinary files under `assets/`, because it carries its own UI:
the preferred-policy picker, the relocation dialog, task titles, and strings in all 13
locales. A merge will therefore always report `assets` vs `assets/**` as a structural
conflict; that is expected, not a mistake.

Handle it by keeping the vendored tree and re-applying the frontend part of an upstream
release by hand if it matters:

```bash
git checkout --ours assets          # keep the vendored frontend
git add assets
```

Then, if the upstream release changed the frontend, port those changes deliberately and
rebuild:

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File assets\build-frontend.ps1 -BackendVersion <version>
go build -o cloudreve.exe .
```

Do not try to be clever about merging the submodule: the two representations cannot be
reconciled automatically, and the fork's UI work would be lost.

## Versioning

`BackendVersion` in `application/constants/constants.go` is both the reported version and
the database schema marker, so it must match the git tag: `application/statics` compares it
against the embedded frontend's `version.json` and refuses to serve a mismatch.

Because the fork's baseline is upstream 4.19.1, its version numbers must stay **above**
upstream's to avoid two different builds sharing a number. Upstream has already used
4.15.0 through 4.19.1, so start at 4.20.0 and keep incrementing the minor version for
releases that include upstream merges or new features.

When bumping, change both places and let the tag be the third:

1. `application/constants/constants.go` -> `BackendVersion`
2. `assets/build-frontend.ps1` -> the `$BackendVersion` default
3. the git tag, which the release workflow compiles in and stamps into `version.json`

## What made the ancestry possible

The fork started as source files imported into a fresh repository, so its first commit had
no relationship to upstream and a merge would have had to reconcile two unrelated trees.
The history was therefore rewired: the fork's first commit was rebuilt with upstream
`a8becb9f` as its parent, keeping the tree byte-identical, and every later commit was
replayed on top. The resulting history is linear, contains the whole upstream history, and
places the fork's commits after the 4.19.1 commit.

Consequences worth knowing:

- Older clones must be reset (`git fetch && git reset --hard origin/main`); their `main`
  points at commits that no longer exist on the remote.
- The `4.16.0` and `4.17.0` tags still reference the pre-rewrite commits, which is why the
  releases built from them remain valid. Do not reuse those tag names.
