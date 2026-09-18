# Container image release

`release-image.yml` runs when a tag matching `x.y.z` is pushed and produces two things:

1. a multi-architecture image (`linux/amd64` + `linux/arm64`) pushed to
   `ghcr.io/<owner>/cloudreve`, tagged with the release tag, `latest`, and `v4`;
2. one compressed `docker save` archive per architecture attached to the GitHub
   release for that tag, named `cloudreve_<tag>_linux_amd64.tar.gz` and
   `cloudreve_<tag>_linux_arm64.tar.gz`.

## Publishing a release

```bash
git tag 4.16.0
git push origin 4.16.0
```

The tag **must not carry a `v` prefix**. It is used in two places that both expect a
bare version:

- `docker/metadata-action` derives the image tags from it, so `v4.16.0` would publish
  `:v4.16.0`;
- it is compiled in as `constants.BackendVersion` and written into the frontend's
  `version.json`. `application/statics` compares the two at startup and refuses to
  serve a UI whose version does not match, so the two must be the same string.

A tag like `v4.16.0` does not match the trigger pattern at all and silently produces
no run.

## How the jobs fit together

| Job | Purpose |
|---|---|
| `frontend` | Installs the frontend dependencies, runs `vite build`, stamps `version.json` with the tag, and packages `application/statics/assets.zip`. The Go build embeds that zip with `//go:embed`, so the archive has to exist before any compile happens. |
| `image` | Builds both platforms in one Buildx invocation and pushes the resulting manifest list to GHCR. |
| `archives` | Builds one platform at a time, loads it into the local daemon, and exports it with `docker save` + `gzip`. A manifest list cannot be saved as a single archive, hence one build per platform. |
| `release` | Creates the GitHub release for the tag if it does not exist and attaches the two archives. It runs last so the release is never published with missing files. |

The image build needs no QEMU even for `linux/arm64`: the Dockerfile cross-compiles Go
using the `TARGETARCH` build argument, so no foreign-platform code is executed. The
`image` job still sets QEMU up to stay correct if the Dockerfile ever runs a foreign
binary during the build.

## Required repository permissions

The workflow relies on the default `GITHUB_TOKEN`:

- `packages: write` to push the image. If the repository or organisation restricts
  package creation, grant Actions write access under
  *Settings → Actions → General → Workflow permissions*.
- `contents: write` to create the release. If that is disabled, the release has to be
  created beforehand or the token replaced with a PAT.

On the first push the package is created as **private**. Make it public under
*Packages → cloudreve → Package settings* if the image should be pullable without a
login.

## Frontend version note

The `frontend` job writes `assets/build/version.json` from the tag instead of running
`yarn version` (the upstream `.build/build-assets.sh` does the latter, modifying
`package.json` and the lockfile in the working tree). Only the stamped file matters at
runtime; `package.json`'s version is not read by the built application.

`npm run build` is used rather than `build-prod`, because the latter runs `tsc` first
and the repository carries pre-existing type errors unrelated to the release.

## What has and has not been verified

Verified while writing this workflow:

- the YAML parses and its job graph, triggers, permissions, and matrix are as intended;
- the zip packing step enforces the layout `statics.go` requires
  (`assets/build/` prefix, forward slashes, `version.json` present);
- `.dockerignore` reduces the build context from ~1.2GB to ~21MB while keeping every
  input the Dockerfile reads.

Not verified, because this machine has no Docker daemon and no way to execute GitHub
Actions:

- that the image actually builds and that both architectures produce a working binary;
- that the container starts and serves the UI;
- that GHCR accepts the push under your account's permissions.

The first tag push is therefore the real test. Watch the `image` job log first: the Go
compile is where a Dockerfile mistake will surface.
