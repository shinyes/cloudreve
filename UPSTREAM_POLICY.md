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

### 1. No remote can push to upstream

```bash
git remote -v
# origin    git@github.com:shinyes/cloudreve.git        (fetch)
# origin    git@github.com:shinyes/cloudreve.git        (push)
# upstream  https://github.com/cloudreve/Cloudreve.git  (fetch)
# upstream  no-push-per-upstream-policy                 (push)
```

`origin` is this fork, and it is the only push target. `upstream` exists purely as a read
mirror: its `pushurl` is a placeholder that does not resolve, so git cannot push through it
even by accident. The mirror is what keeps a shared git ancestor with upstream, which is
what makes merging upstream releases a normal three-way merge - see
[`UPGRADING.md`](UPGRADING.md).

A remote that fetches from upstream is fine; a remote that can push there is not, and that
distinction is what the checks below enforce.

### 2. A pre-push hook refuses upstream URLs

`.githooks/pre-push` inspects the resolved push URL and aborts if it resolves to the
upstream repository, including a differently cased URL and an aliased remote. It resolves a
named remote through `pushurl` as well, so the fetch-only mirror is safe while a remote
that really points upstream is still blocked. Enable the hook in a fresh clone with:

```bash
git config core.hooksPath .githooks
```

This setting is per clone, so a new clone has to run it once. The CI check below
catches the case where it was forgotten.

### 3. CI fails if the policy is violated

`.github/workflows/upstream-guard.yml` runs on every push and pull request and fails when a
remote is **pushable** towards upstream - a `pushurl` resolving to it, or a plain `url`
with no `pushurl` override that does not mark itself as fetch-only - or when a pull request
targets the upstream repository.

## How to reuse upstream code

With the mirror in place, fetch normally:

```bash
git fetch upstream master --no-tags
```

To fetch something without any remote configured at all:

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

This repository takes the named-remote form above, configured as shown in layer 1, which
is why `git fetch upstream master` works here out of the box.

## If you ever do need to contribute upstream

Do it from a separate clone or a branch of a personal fork that has no relationship to
this repository. Do not lift the guard here, and do not give the mirror a real push
target: a repository that both pulls from and pushes to upstream is exactly the situation
this policy exists to prevent.

Changing the mirror's `pushurl` back to upstream is the one edit that would defeat every
layer at once, so it is the thing to never do. The pre-push hook and the CI check are
there to catch it if it happens.
