[中文版本](https://github.com/cloudreve/cloudreve/blob/master/README_zh-CN.md)

<h1 align="center">
  <br>
  <a href="https://cloudreve.org/" alt="logo" ><img src="https://raw.githubusercontent.com/cloudreve/frontend/master/public/static/img/logo192.png" width="150"/></a>
  <br>
  Cloudreve
  <br>
</h1>
<h4 align="center">Self-hosted file management system with multi-cloud support.</h4>

<p align="center">
  <a href="https://dev.azure.com/abslantliu/cloudreve/_build?definitionId=6">
    <img src="https://img.shields.io/github/check-runs/cloudreve/cloudreve/master"
         alt="Azure pipelines">
  </a>
  <a href="https://github.com/cloudreve/cloudreve/releases">
    <img src="https://img.shields.io/github/v/release/cloudreve/cloudreve?include_prereleases" />
  </a>
  <a href="https://github.com/cloudreve/cloudreve/releases">
     <img src="https://badgen.net/static/release%20size/34%20MB/blue"/>
  </a>
  <a href="https://hub.docker.com/r/cloudreve/cloudreve">
  <img alt="Docker Pulls" src="https://img.shields.io/docker/pulls/cloudreve/cloudreve" />
  </a>
</p>
<p align="center">
  <a href="https://cloudreve.org">Homepage</a> •
  <a href="https://demo.cloudreve.org">Try it</a> •
  <a href="https://github.com/cloudreve/cloudreve/discussions">Discussion</a> •
  <a href="https://docs.cloudreve.org">Documents</a> •
  <a href="https://github.com/cloudreve/cloudreve/releases">Download</a> •
  <a href="https://t.me/cloudreve_official">Telegram</a> •
  <a href="https://discord.com/invite/WTpMFpZT76">Discord</a>
</p>

![Screenshot](https://raw.githubusercontent.com/cloudreve/docs/master/images/homepage.png)

## :sparkles: Features

- :cloud: Support storing files into Local, Remote node, OneDrive, S3 compatible API, Qiniu Kodo, Aliyun OSS, Tencent COS, Huawei Cloud OBS, Kingsoft Cloud KS3, Upyun.
- :outbox_tray: Upload/Download in directly transmission from client to storage providers.
- 💾 Integrate with Aria2/qBittorrent to download files in background, use multiple download nodes to share the load.
- 📚 Compress/Extract/Preview archived files, download files in batch.
- 💻 WebDAV support covering all storage providers.
- :zap:Drag&Drop to upload files or folders, with parallel resumable upload support.
- :card_file_box: Extract media metadata from files, search files by metadata or tags.
- :family_woman_girl_boy: Multi-users with multi-groups.
- :link: Create share links for files and folders with expiration date.
- :eye_speech_bubble: Preview videos, images, audios, ePub files online; edit texts, diagrams, Markdown, images, Office documents online.
- :art: Customize theme colors, dark mode, PWA application, SPA, i18n.
- :rocket: All-in-one packaging, with all features out of the box.
- 🌈 ... ...

## :hammer_and_wrench: Deploy

To deploy Cloudreve, you can refer to [Getting started](https://docs.cloudreve.org/overview/quickstart) for a quick local deployment to test.

When you're ready to deploy Cloudreve to a production environment, you can refer to [Deploy](https://docs.cloudreve.org/overview/deploy/) for a complete deployment.

## :gear: Build

Please refer to [Build](https://docs.cloudreve.org/overview/build/) for how to build Cloudreve from source code.

## :bookmark: About this repository (fork)

This is a private fork of `cloudreve/Cloudreve` for internal deployment. **The upstream
documentation does not describe this fork exactly**; the notes below take precedence.

### Differences from upstream

1. **A user group can be bound to several storage policies** (many-to-many
   `group_storage_policies`; the old single-policy column is kept but deprecated).
2. **Per-directory preferred storage policy**: uploads walk the parent chain and land on
   the chosen policy.
3. **Files and folders can be relocated between storage policies** (with rollback).
4. **The frontend sources under `assets/` are committed normally** (upstream keeps them
   in a git submodule; `.gitmodules` was removed here).
5. **Releases go through GitHub Actions container images**, not goreleaser or Azure
   Pipelines.

### Versioning

`BackendVersion` in `application/constants/constants.go` is **both** the database schema
version marker and the value compared against the embedded frontend's `version.json`.
**Changing it requires changing all three places, otherwise startup logs
`Static resource version mismatch`**:

| Where | Purpose |
|---|---|
| `application/constants/constants.go` → `BackendVersion` | version embedded in the binary |
| `assets/build-frontend.ps1` → `$BackendVersion` default | version stamped when packing locally |
| the git tag | CI overwrites `version.json` from the tag, so it must match the other two |

Tags carry **no `v` prefix** and look like `4.16.0`. This fork claims **minor** versions
(4.16.0) so it stays distinct from upstream patch releases (4.15.1, 4.15.2, ...) when
upstream code is merged later.

> Schema patches are gated on `Patch.EndVersion` compared against the versions already
> recorded in the database (`inventory/migration.go`), so bumping the version alone never
> re-runs an old patch. Add an entry to `patches` only when a new patch is needed.

### Release procedure

```bash
# 1. Bump the version in the two source locations listed above.
# 2. Verify locally, then commit.
git add -A && git commit -m "release: bump BackendVersion to 4.17.0"
git push origin main

# 3. Tag and push - this triggers the release.
git tag -a 4.17.0 -m "Cloudreve 4.17.0 fork release"
git push origin 4.17.0
```

Pushing the tag makes `.github/workflows/release-image.yml`:

1. install the frontend dependencies, run `vite build`, stamp `build/version.json` from
   the tag, and pack `application/statics/assets.zip`;
2. build a `linux/amd64` + `linux/arm64` image with Buildx and push it to
   `ghcr.io/shinyes/cloudreve` tagged `<version>`, `latest`, and `v4`;
3. build each architecture separately and export it with `docker save` + `gzip`,
   uploading `cloudreve_<version>_linux_amd64.tar.gz` and
   `cloudreve_<version>_linux_arm64.tar.gz`;
4. create the GitHub release for the tag and attach both archives.

**A `v` prefix breaks this twice**: `v4.17.0` does not match the trigger pattern
(`[0-9]+.[0-9]+.[0-9]+`) so nothing runs, and it would not match `version.json` either.

First-time prerequisites, both set in the GitHub web UI: on a private repository the
default `GITHUB_TOKEN` is read-only, so enable **Read and write permissions** under
*Settings → Actions → General → Workflow permissions*; and the pushed package starts
private, so make it public under *Packages → cloudreve → Package settings* if it should
be pullable without a login.

### Building and testing locally

`application/statics/assets.zip` is **gitignored** (CI generates it), so a fresh clone
cannot `go build` until the frontend is packed - `//go:embed` fails on the missing file:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File assets\build-frontend.ps1 -BackendVersion 4.16.0
go build -o cloudreve.exe .
```

```powershell
# Unit tests: encryption metadata contract, relocation encryption matrix.
go test ./inventory/ ./pkg/filemanager/manager/

# End-to-end: three suites, each on its own throwaway instance.
powershell -NoProfile -ExecutionPolicy Bypass -File tests\relocate\run.ps1
```

Do **not** use `npm run build-prod` for the frontend: it runs `tsc` first and the
repository carries pre-existing type errors unrelated to any given change. Both CI and
local builds use `npm run build`.

### Relationship with upstream (important)

This fork is based on upstream **4.19.1** and **never publishes anything upstream**: no pull
requests, no branches, no tags. See [`UPSTREAM_POLICY.md`](UPSTREAM_POLICY.md).

- `upstream` is configured as a **fetch-only mirror** (its `pushurl` does not resolve), so
  the shared git ancestor is available while pushing there is impossible;
- `.githooks/pre-push` refuses a push to an upstream URL (enable per clone with
  `git config core.hooksPath .githooks`);
- `.github/workflows/upstream-guard.yml` checks on every push and PR that no remote can
  push upstream, **and** that the shared ancestor with upstream still exists.

**Read [`UPGRADING.md`](UPGRADING.md) before changing code here.** It records the standing
rules - never re-import upstream sources, never rewrite published history, keep fork
changes identifiable - and the procedure for merging an upstream release, which is a normal
three-way merge:

```bash
git fetch upstream master --no-tags
git switch -c merge/upstream-<version> main
git merge upstream/master
```

## :rocket: Contributing

If you're interested in contributing to Cloudreve, please refer to [Contributing](https://docs.cloudreve.org/api/contributing/) for how to contribute to Cloudreve.

## :alembic: Stacks

- [Go](https://golang.org/) + [Gin](https://github.com/gin-gonic/gin) + [ent](https://github.com/ent/ent)
- [React](https://github.com/facebook/react) + [Redux](https://github.com/reduxjs/redux) + [Material-UI](https://github.com/mui-org/material-ui)

## :scroll: License

GPL V3
