# Readiness bootstrap

`gz-git integrate bootstrap` is the deliberately narrow recovery path for a
target that predates the target-owned readiness contract.

```sh
gz-git integrate bootstrap plan --issuer alice --target origin/master \
  --output /tmp/readiness-bootstrap.json
gz-git integrate bootstrap apply --plan /tmp/readiness-bootstrap.json \
  --confirm <CONFIRM_DIGEST>
```

`plan` never changes remote refs, though it may update local fetch state. It emits an expiring canonical
confirmation plan (it is not a signed authorization). `apply` is an
explicitly human-operated action: it requires `--confirm <sha256>` equal to
the displayed canonical plan digest. It succeeds only when the source is exactly one fast-forward
commit ahead, the target has no readiness declaration, and that commit changes
only `.gz-git.yaml` by adding `branch.readiness` plus regular files below
`.gz-git/readiness/`. The plan records the repository URL, remote, both refs
and object IDs, manifest/runner/tree IDs, tree digest, issuer, operation ID,
and expiry.

`apply` accepts only a plan file. It fetches and recomputes all of those facts,
fails closed on drift or expiry, and performs one exact
`--force-with-lease` fast-forward push. Once the target declares readiness the
plan cannot be reused. No cleanup is attempted before the push.

This is a bootstrap transaction, not a general-purpose file copier: symlinks,
submodules, additional config changes, and multi-commit branches are rejected.

The CLI does not authenticate the operator. The downstream agent policy and
PreToolUse hook must deny `integrate bootstrap apply`; execution is reserved
for a separately reviewed human-operated procedure.
