# Readiness contract update

`gz-git integrate readiness update` is the deliberately narrow, human-operated
transaction for changing an existing target-owned Readiness V1 contract. It is
not a target rollout mechanism: both the target and source must already have a
valid V1 contract.

```sh
gz-git integrate readiness update plan \
  --branch feature/readiness-change \
  --target origin/master \
  --issuer alice \
  --output /tmp/readiness-update.json

# Run only from an interactive terminal, after an independent human review.
gz-git integrate readiness update apply \
  --plan /tmp/readiness-update.json \
  --confirm <CONFIRM_DIGEST>
```

## Plan is evidence, not authorization

`plan` makes no remote ref changes (although it may refresh local fetch state).
It requires `--issuer`, a canonical remote-tracking `--target`, and a lifetime
of at most 15 minutes. The emitted plan is strict JSON whose exact fields are
the reviewed evidence; the CLI separately prints their canonical confirmation
digest. Unknown, missing, or duplicate fields are rejected when the plan is
read back.

The source must be exactly one fast-forward commit ahead of the target. That
commit may change only the selected existing manifest (`.gz-git.yaml`,
`.gz-git.yml`, or `.gz-git.json`) and paths under `.gz-git/readiness/`; it may
not migrate the selected manifest path. The final manifest and readiness files
must be regular blobs. A runner rename and adding, changing, or deleting
readiness helpers are valid contract changes, including deletion of the old
runner, provided the resulting V1 contract is valid.

The plan binds the repository and push endpoint, source and target refs/object
IDs, destination ref, operation-marker ref, and both old and new manifest,
runner, readiness-tree, tree-digest, and contract-digest evidence. It does not
run the old runner: this transaction is specifically able to repair a broken
or replaced runner.

## Apply is a human-only, one-use transaction

`apply` accepts only a saved plan plus a `--confirm` value exactly equal to its
canonical digest. The supported CLI requires an interactive TTY and has no
`--yes` or environment bypass. A TTY and digest do not authenticate the
operator: the digest only acknowledges review, and downstream policy and hook
enforcement are required to keep automated agents outside this boundary.

Before pushing, `apply` recomputes the complete snapshot and fails closed on
expiry, plan tampering, source/target drift, endpoint changes, or a changed old
contract. It then performs one atomic push with exact leases for the target and
for an absent, unique operation marker. The successful push advances the target
and writes the operation ref together. While that marker is preserved, it
blocks reuse of the plan even if the target is later rolled back. If the remote
does not support atomic push or its custom operation ref, the command fails
closed and changes neither ref.

Operation refs below `refs/gz-git/readiness-update/operations/` are intended to
be permanent audit and replay-protection markers. Preserve them; deleting one
removes both its audit record and the replay protection it provides.

The downstream agent policy and PreToolUse hook must deny
`integrate readiness update apply`. Only a separately reviewed, human-operated
procedure may invoke `apply`.
