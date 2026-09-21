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

Resolve conflicts, then run the full check before committing the merge:

```bash
go build ./...
go test ./inventory/ ./pkg/filemanager/...
powershell -NoProfile -ExecutionPolicy Bypass -File tests\relocate\run.ps1
```

Merge into `main` once those pass, then bump the version and tag a release as described in
the README.

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
