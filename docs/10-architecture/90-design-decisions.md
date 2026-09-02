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

**Status**: Conditionally accepted; implementation is blocked on a released CE
observation contract that satisfies the boundary below.

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

   It is one strict YAML document with exact known fields. Duplicate keys, multiple
   documents, anchors, aliases, custom tags, and implicit type coercion are rejected. It
   must be index-tracked. Every entrypoint must resolve to an index blob with regular mode
   `100644` or `100755`. Untracked paths, symlinks, gitlinks, trees, and other modes
   produce a typed finding without reading or digesting worktree bytes. An entry added
   only to the index is reported as `index-only`; index/worktree divergence is `dirty`.
   Observation reports distinct HEAD, index, and worktree manifest identities and dirty
   state; worktree bytes are the parsing source.

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
   without an equivalent secure-open primitive returns `unsupported-platform`, with no
   lexical fallback.

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
   independent 1 MiB caps. Stdout is exactly one JSON document plus trailing whitespace.
   Unknown fields are rejected for the negotiated exact schema; new fields require a new
   negotiated capability/schema.

   Start/wait failure, timeout, cancellation, cap overflow, malformed/trailing JSON,
   capability mismatch, or unexpected exit is a gz-git transport fault. Stderr remains
   diagnostic only. Timeout or overflow cancels the process tree, discards partial JSON,
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

1. **No apply surface in this delivery.** The PRODUCT hook-manager non-goal controls.
   No context-reference or hook-wiring install, update, repair, plan/apply/verify surface,
   apply capability, implicit clone/sync/integrate mutation, or apply work item is
   authorized. Reconsideration requires a new product-scope decision after the read-only
   pilot proves linked-worktree observation and external-owner preservation.

**Result vocabulary and namespacing**:

- `componentOutcome: observed | absent | unsupported | unknown | fault` describes only
  aggregation/transport. Manifest absence is also `context.not-declared`.
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
  prevention, hostile common/object/alternate-object Git environment, CE exits 0/1/2,
  and linked worktrees with different effective states.
- Before/after evidence proves Git config, index/staged set, worktree bytes/modes, and hook
  bytes are unchanged.
- Repository gates are `make quality-check` and `git diff --check`, followed by independent
  review with no unresolved P0/P1/P2.

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

- Git hooks automation
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
