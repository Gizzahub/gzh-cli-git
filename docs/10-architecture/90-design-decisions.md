# 12-13. Design Decisions and Future Considerations

> gzh-cli-gitforge 아키텍처 문서 · [인덱스](README.md) · [ARCHITECTURE.md](../../ARCHITECTURE.md)

## 12. Design Decisions

### 12.1 Decision Log

#### D1: Git CLI vs. go-git Library

**Decision**: Use Git CLI
**Rationale**:

- Maximum compatibility with all Git features
- Simpler implementation (no need to reimplement Git logic)
- Users already have Git installed
- Easier to debug (same commands users run manually)

**Trade-offs**:

- External dependency on Git binary
- Slower than pure Go (process spawning overhead)
- Parsing text output vs. structured API

**Alternatives Considered**:

- go-git/v5: Pure Go, no external deps, but incomplete feature set
- Hybrid: Use go-git for simple ops, Git CLI for complex (too complex)

#### D2: Library-First Architecture

**Decision**: Design library (`pkg/`) with zero CLI dependencies
**Rationale**:

- Enables reuse in gzh-cli and other projects
- Better API design (forced to think about interfaces)
- Easier testing (no CLI framework mocks)
- Clear separation of concerns

**Trade-offs**:

- More upfront design effort
- Indirection layer between CLI and logic
- Some code duplication (CLI and library versions)

**Alternatives Considered**:

- CLI-first, extract library later (risky, usually doesn't happen)
- Monolithic design (violates single responsibility)

#### D3: Functional Options Pattern

**Decision**: Use functional options for all complex operations
**Rationale**:

- API extensibility without breaking changes
- Sensible defaults
- Self-documenting (option names are clear)
- Idiomatic Go pattern

**Trade-offs**:

- More verbose (but clearer)
- Slightly more allocations (usually negligible)

**Example**:

```go
// Instead of:
Clone(ctx, url, path, branch, depth, progress, recursive)

// Use:
Clone(ctx, url, path,
    WithBranch("main"),
    WithDepth(1),
    WithProgress(os.Stdout),
)
```

#### D4: Context Propagation

**Decision**: All operations accept `context.Context` as first parameter
**Rationale**:

- Cancellation support (user can Ctrl+C)
- Timeout support (prevent infinite hangs)
- Request-scoped values (trace IDs, etc.)
- Idiomatic Go concurrency pattern

**Trade-offs**:

- Every function signature includes ctx
- Must remember to pass context through

#### D5: Interface-Driven Design

**Decision**: Define interfaces for all major components
**Rationale**:

- Testability (easy to mock)
- Extensibility (consumers can provide implementations)
- Decoupling (depend on interfaces, not concretions)

**Trade-offs**:

- More files (interface + implementation)
- Indirection (but worth it for benefits)

#### D6: Read-Only Context Reference and CE Observation

**Status**: Conditionally accepted. In this document that means the decision's boundary is
settled and binding, but no code may be derived from it until the named prerequisites hold:
a released CE follow-up satisfying item 8, and an implementation card that records the
released capability, schema, and exit mapping verbatim. It is not a draft, and its
boundaries are not reopened by implementation convenience.

**Decision**: Add a library-first, read-only observation capability for repository and
worktree context references and CE-owned Git-gate diagnostics. It reports identity and
state only. It does not load agent instructions, interpret Skill content, execute
descriptor commands, or mutate Git configuration, the index, worktree files, modes, or
hooks.

The first delivery has these boundaries:

1. **One strict context transport.** A repository opts in with the tracked, root-local
   `.gz-git-context.yaml` manifest, schema `gz-git.context-reference/v1`. Its only v1
   payload is a set of `context.entrypoints`; author order has no precedence and output
   is sorted by canonical UTF-8 byte order. The manifest contains no commands, arguments,
   environment, runtime precedence, hooks, or Skill IDs. Hook declaration remains solely
   in CE's typed project contract. The manifest is never upward-merged or inherited.

   Root-local constrains **discovery**, not merely merging. The only path examined is
   `.gz-git-context.yaml` at Git's selected worktree root. No parent directory is walked
   and no subdirectory is scanned. A file of that name elsewhere is neither read nor
   reported: it is not a finding, because gz-git does not claim to have searched for it.
   This deliberately differs from the sibling `.gz-git.yaml` convention, whose
   `DetectConfigFile` walks upward to `$HOME`; that upward walk is not a precedent here.

   It is one strict YAML document with exact known fields. Duplicate keys, multiple
   documents, anchors, aliases, custom tags, and implicit type coercion are rejected. It
   must be index-tracked. Every entrypoint must resolve to an index blob with regular mode
   `100644` or `100755`. Untracked paths, symlinks, gitlinks, trees, and other modes
   produce a typed finding without reading or digesting worktree bytes. An entry added
   only to the index is reported as `index-only`; index/worktree divergence is `dirty`.
   Observation reports distinct HEAD, index, and worktree manifest identities and dirty
   state; worktree bytes are the parsing source.

   The manifest requires **both** index-tracked status and worktree presence, and the two
   properties are independent, so all four states are defined below. The manifest's own
   state is never inferred from the entrypoint rules above: those govern entries within an
   already-parsed manifest and do not reach the manifest file itself.

   | index     | worktree | Result                                                                                                                 |
   | --------- | -------- | ---------------------------------------------------------------------------------------------------------------------- |
   | tracked   | present  | Parsed from worktree bytes. Normal path.                                                                               |
   | tracked   | absent   | `context.manifest-not-materialized`. Not parsed. Reachable via `skip-worktree`, a sparse checkout, or manual deletion. |
   | untracked | present  | `context.manifest-untracked`. **Not parsed, not digested.**                                                            |
   | untracked | absent   | `context.not-declared`.                                                                                                |

   All four are successful observations and therefore report `componentOutcome=observed`;
   the state is carried by the reason code, not by the outcome token. `componentOutcome`
   describes aggregation and transport only, so a repository-state problem never presents
   as `fault`, and a transport fault never presents as a repository state.

   The `untracked | present` row is fail-closed by construction. A manifest that exists on
   disk but is not in the index is not a partially valid manifest to be read anyway.
   Reading it would silently defeat the tracked-transport requirement, because an
   unreviewed local file would then steer observation.

1. **Absence is not discovery.** Manifest absence is `context.not-declared`, never
   `native-discovery`. gz-git cannot infer an agent runtime's search, precedence, or
   successful load. A future runtime-owned contract may report that fact.

1. **Host-independent paths and sources.** V1 paths are UTF-8 and use `/` only. Leading
   slash, backslash, drive/UNC form, NUL/control bytes, trailing slash, and empty, `.` or
   `..` components are rejected. Windows reserved names and components ending in dot or
   space are also rejected. Opens use encoded components verbatim without Unicode or case
   normalization; paths resolving to the same opened terminal identity are duplicate/
   alias findings. HEAD and index identity use algorithm-qualified Git object IDs.
   Optional worktree observation reports tracked state, source, Git mode,
   dirty state, size, and a `sha256:<lowercase-hex>` digest, never content. Git OIDs and
   content digests are separate fields. A worktree digest is an observation, not a
   signature or semantic-integrity proof.

1. **Fail-closed path opening.** Git's selected worktree root is opened no-follow and
   non-reparse first, and its identity is bound to the root handle. Every component is
   then opened relative to its
   retained parent handle without following links. Duplicate paths, symlinks, mount
   escapes, Windows reparse points, non-regular terminals, and unsupported types are
   rejected. Terminal identity, size, and mtime/ctime or Windows change metadata are
   checked before and after bounded streaming; change discards the digest. A platform
   without an equivalent secure-open primitive returns `context.unsupported-platform`, with
   no lexical fallback.

   The repository's existing `.gz-git.*` loaders are **not reuse candidates** for any of
   this. `pkg/config/paths.go` probes with `os.Stat`, which follows symlinks, and
   `pkg/config/symlink.go` creates `.gz-git.yaml` as a symlink on purpose. Both are correct
   for their own feature and both violate this item. Sharing a filename prefix is not a
   reason to share a resolver.

   `context.unsupported-platform` is a reason code in the same `context.` namespace as
   `context.not-declared`, and pairs with `componentOutcome=unsupported`. It is
   deliberately not the convention used by `pkg/integrate/readiness.go:133`, which reports
   an absent platform capability as `checkFail`. That surface answers "is this branch
   ready", where an unanswerable check is a failure; this surface reports observations,
   where an unanswerable check is an absence. The divergence is intended, and neither
   surface should be changed to match the other.

1. **Bounded input.** Initial v1 operational limits are a 64 KiB manifest, 32 unique
   entrypoints, 1 MiB per entrypoint, and 4 MiB aggregate per repository observation.
   Handles and streaming both enforce the limits. The pilot measures corpus sizes and
   exercises every limit. Raising a limit is compatible; lowering it or changing its
   meaning requires a schema revision or explicit compatibility decision.

1. **CE remains the gate-semantics owner.** gz-git accepts a trusted CE descriptor from
   an approved installation resolver, never portfolio/repository data, hook declarations,
   ambient unpinned `PATH`, or command text. It contains an absolute canonical executable
   path, expected released contract/capability identity, expected SHA-256 release digest,
   and stable opened-file identity. The regular, non-symlink/non-reparse executable is
   checked before and after handshake and observation; both use the same resolver and
   transport.

   Invocation starts from a documented environment allowlist and inherits no `GIT_*`
   variable. Any Git variables required by the fixed invocation are set explicitly from
   trusted values. This excludes repository, common-dir, object, alternate-object, ref,
   index, and config redirects instead of maintaining a partial denylist. CE v2 must
   resolve its internal Git
   executable through an approved descriptor instead of ambient `PATH`, and define
   whether retained HOME/global Git configuration participates in observation. The result
   records that the same-user installation owner and local administrator are trusted;
   portable verified-handle execution is not claimed.

1. **Bounded subprocess transport.** After an exact released-capability handshake, the
   library invokes fixed CE arguments with `context.Context` in the target worktree and
   propagates cancellation throughout. Default timeout is 10 seconds and implementation
   maximum is 30 seconds. Stdout/stderr are concurrently drained with
   independent 1 MiB caps. Stdout is exactly one JSON document plus optional trailing
   whitespace.
   Unknown fields are rejected for the negotiated exact schema; new fields require a new
   negotiated capability/schema.

   Start/wait failure, timeout, cancellation, cap overflow, malformed/trailing JSON,
   capability mismatch, or unexpected exit is a gz-git transport fault. So is an exit code
   that contradicts the decoded envelope: exit `0` carrying a finding envelope, or exit `1`
   carrying a pass envelope. Neither is an "unexpected exit" — both codes are in the
   contract — so this case is named explicitly rather than left to that clause. A
   disagreeing pair is never resolved by trusting one side; the decoded JSON is discarded
   and the observation is a fault, because a CE that misreports its own outcome has not
   demonstrated that either channel is reliable. Stderr remains diagnostic only. Timeout or overflow cancels the process tree, discards partial JSON,
   drains and cleans up for at most 2 seconds, and reports any orphan as a fault.

1. **CE v2 is a prerequisite, not an assumption.** Current CE task-doctor v1 lacks the
   capability handshake and required provenance, and follows symlinks while hashing. A
   tag of that implementation is insufficient. The follow-up must expose schema and
   capability ID, repository ID, common Git-directory identity, linked-worktree identity,
   observation source, `hooksPath` origin/scope/raw/resolved, declared gate identity/event/
   path/version/digest, observed existence, source revision/blob, dirty state, Git mode,
   digest, executable state, provider status/remediation, and a symlink/reparse-safe
   resolution guarantee.

   Its exit contract is exactly `0 + valid JSON = pass`,
   `1 + valid JSON = provider finding`, and
   `2 = CE invocation fault with no JSON guarantee`; any other exit is a transport fault.
   The implementation card records the released capability, schema, and exit mapping
   verbatim before code is written.

   **CE exit codes are never passed through as gz-git exit codes.** The two contracts
   assign different meanings to the same integers, so passthrough is a category error, not
   a shortcut: `pkg/cliutil/exit.go` defines `1` as `ExitToolError` and `2` as
   `ExitPartialFailed`, while CE assigns `1` to a provider finding and `2` to its own
   process fault. Passing `2` through would render a CE process fault as
   `ExitPartialFailed`, the findings-shaped meaning this decision forbids. The mapping is
   therefore fixed:

   | CE exit                                       | gz-git exit         | Why                                                                   |
   | --------------------------------------------- | ------------------- | --------------------------------------------------------------------- |
   | `0` (pass) or `1` (provider finding)          | `0` `ExitOK`        | The observation succeeded. Findings are payload, not gz-git failures. |
   | `2` (CE invocation fault)                     | `1` `ExitToolError` | gz-git could not obtain an observation.                               |
   | any other exit, or envelope/exit disagreement | `1` `ExitToolError` | Transport fault.                                                      |

   This surface never produces `2` `ExitPartialFailed` or `3` `ExitReclaimIncomplete`.
   Because gz-git's exit describes gz-git's own outcome, a caller distinguishes pass from
   finding by reading the emitted JSON, not the exit code.

1. **No apply surface in this delivery.** The PRODUCT hook-manager non-goal controls.
   No context-reference or hook-wiring install, update, repair, plan/apply/verify surface,
   apply capability, implicit clone/sync/integrate mutation, or apply work item is
   authorized. Reconsideration requires a new product-scope decision after the read-only
   pilot proves linked-worktree observation and external-owner preservation.

**Result vocabulary and namespacing**:

- `componentOutcome: observed | absent | unsupported | fault` describes only
  aggregation/transport. Manifest absence is also `context.not-declared`.
- `unknown` is deliberately **not** a member. An earlier draft listed it, but no condition
  in this decision produces it, and an enum member without a producing condition becomes a
  per-repository catch-all — exactly the divergence this vocabulary exists to prevent. A
  future member requires a condition defined at the same time.
- `fault` alone cannot keep the two fault classes separate, so every `fault` result also
  carries `faultDomain: gz-git-transport | ce-invocation`. `ce-invocation` is set only by
  CE exit `2`; every other fault, including envelope/exit disagreement, is
  `gz-git-transport`. Without this field the requirement below — that a CE process fault
  never becomes a provider finding — is unverifiable by a consumer, because both classes
  would present as the same token.
- `providerStatus: adopted | not-adopted | drift | unsupported` preserves CE's raw status
  and remediation. CE gate absence remains `gate.not-adopted`.
- Capability/schema/platform absence uses `componentOutcome=unsupported`; it is never
  confused with CE's valid `providerStatus=unsupported` exit-1 finding.

Component results stay separate: absent context cannot erase a valid CE observation, and
a CE process fault cannot become a provider finding.

**Implementation gates**:

- The DevBox owns separate implementation and Sigdock pilot cards; this repository's
  `tasks/` remains an issue-pointer log.
- The implementation card cites the released CE follow-up satisfying item 8 and allocates
  the final gz-git capability ID and JSON schema. This decision reserves no unimplemented
  CLI command name.
- Fixtures cover hostile YAML/path grammar, every size limit and corpus measurement,
  terminal/intermediate symlink or reparse swaps, same-size writes, HEAD/index/worktree
  divergence, strict JSON and stream caps, timeout/cancellation, cleanup timeout/orphan
  prevention, hostile common/object/alternate-object Git environment, CE exits 0/1/2, envelope/exit
  disagreement in both directions (exit 0 with a finding envelope and exit 1 with a pass
  envelope), and linked worktrees with different effective states.
- Before/after evidence proves Git config, index/staged set, worktree bytes/modes, and hook
  bytes are unchanged.
- Repository gates are `make quality-check` and `git diff --check`, followed by independent
  review with no unresolved P0/P1/P2.

**Rationale**: The portfolio needs to answer "which context does this repository declare,
and is its Git gate adopted" across many repositories, and to answer it the same way in
every one. Two properties make that answer trustworthy: the input is reviewed like code,
and reading it cannot change anything. A tracked manifest gives the first; a read-only
observation surface gives the second. Everything else in this decision follows from
refusing to weaken either — which is why the boundary is unusually long for a §12 entry,
and why the unresolved seams found in independent review are closed here in the document
rather than left to be settled by whoever writes the code first.

**Alternatives Considered**:

- **Infer context from the agent runtime's native discovery.** Rejected. gz-git cannot
  observe another runtime's search order, precedence, or whether a load actually
  succeeded, so any report would be a guess presented as an observation. Manifest absence
  is therefore `context.not-declared`, never `native-discovery`. A runtime-owned contract
  may report that fact later; gz-git will not synthesize it.
- **Guarded apply (install, repair, or plan/apply/verify for hooks and context).**
  Rejected. The PRODUCT hook-manager non-goal controls, and a mutation surface would make
  the first pilot's evidence a product of gz-git's own writes. Reconsideration requires a
  new product-scope decision after the read-only pilot proves linked-worktree observation
  and external-owner preservation, not an implementation-time judgment call.
- **Reuse the existing `.gz-git.*` config loaders.** Rejected; see item 4. They follow
  symlinks by design.
- **Pass CE's exit code through.** Rejected; see item 8. The two contracts assign
  different meanings to the same integers.

**Trade-offs**:

- A small manifest replaces guesses about runtime-native discovery.
- Secure per-platform opening and a CE follow-up delay implementation, but avoid publishing
  lexical-path or symlink-following behavior as a portfolio contract.
- Rejecting apply preserves product-owned installers and makes the first pilot evidential,
  not mutating.

______________________________________________________________________

## 13. Future Considerations

### 13.1 Potential Enhancements (v2.0+)

**Plugin Architecture:**

- Allow custom commit templates from plugins
- Custom conflict resolution strategies
- Extensible history analyzers

**Performance:**

- libgit2 integration for performance-critical paths
- Persistent cache (disk-based)
- Incremental updates

**Features:**

- Git hooks automation — gated by D6 item 9, which rejects any apply surface in the
  current delivery; reconsideration requires a new product-scope decision
- Submodule management
- Advanced visualizations (TUI)
- Team collaboration features (code review integration)

### 13.2 Scalability

**Large Repositories (100K+ commits):**

- Streaming APIs (don't load all commits into memory)
- Pagination for queries
- Parallel processing for bulk operations

**High Concurrency:**

- Connection pooling for Git operations
- Rate limiting for external APIs (GitHub, GitLab)
- Circuit breakers for error handling
