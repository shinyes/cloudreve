# Upstream policy: this fork never publishes to Cloudreve upstream

This repository is a private fork of `cloudreve/Cloudreve`, maintained for an internal
deployment. It **never** publishes anything back to upstream:

- no pull requests,
- no branches pushed to an upstream remote,
- no tags pushed to an upstream remote,
- no issues or comments created on upstream by automation in this repository.

Pulling upstream code in is fine and expected. Publishing out is not.

## How the constraint is enforced

Relying on remembering this is not enough, because the only local action that can
violate it is a `git push`, and a push is easy to get wrong. Three layers:

### 1. No upstream remote exists

```bash
git remote -v
# origin  git@github.com:shinyes/cloudreve.git (fetch)
# origin  git@github.com:shinyes/cloudreve.git (push)
```

`origin` is this fork. There is deliberately no `upstream` remote, so there is nothing
to accidentally push to and nothing for a tool to discover as a push target.

### 2. A pre-push hook refuses upstream URLs

`.githooks/pre-push` inspects the resolved push URL and aborts if it resolves to the
upstream repository, including a differently cased URL and an aliased remote. Enable it
in a fresh clone with:

```bash
git config core.hooksPath .githooks
```

This setting is per clone, so a new clone has to run it once. The CI check below
catches the case where it was forgotten.

### 3. CI fails if the policy is violated

`.github/workflows/upstream-guard.yml` runs on every push and pull request and fails
when a remote pointing at upstream has appeared in the repository configuration, or
when a pull request targets the upstream repository.

## How to reuse upstream code

Fetch it by URL, without registering a remote:

```bash
# See what upstream has without storing anything.
git ls-remote https://github.com/cloudreve/Cloudreve.git

# Bring a specific upstream branch into a local branch for inspection or merge.
git fetch https://github.com/cloudreve/Cloudreve.git master:upstream-latest

# Merge or cherry-pick from it as usual.
git merge upstream-latest
```

Because no remote is configured, `git push` cannot reach upstream by accident, and
`upstream-latest` is a local branch that nothing publishes.

If you prefer a named remote for convenience, point it at upstream **but make it
fetch-only** so a push is impossible:

```bash
git remote add upstream https://github.com/cloudreve/Cloudreve.git
git config remote.upstream.pushurl "no-push-upstream-policy"
```

The hook from layer 2 still blocks an explicit push to it, and the CI check from
layer 3 fails the build while it exists.

## If you ever do need to contribute upstream

Do it from a separate clone or a branch of a personal fork that has no relationship to
this repository. Do not lift the guard here, and do not add an upstream push target: a
single repository that both pulls from and pushes to upstream is exactly the situation
this policy exists to prevent.
