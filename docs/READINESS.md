# Internal readiness assessment

Assessed 2026-11-25 against the current working tree, including pre-existing edits
in src/main.go and src/pkg/controller/tagcontroller.go. Target: a maintained,
controlled internal pilot, not an unattended organization-wide service.

## Verdict

Suitable foundation; not ready for broad internal execution yet. Local behavior
checks pass, but live correctness, IAM boundaries and an owned operating process
remain unverified. Read-only evaluation can be the first explicitly authorized
sandbox activity. Close the pilot gates below before enabling scheduled writes.
This review is source inspection and local validation, not a penetration test or
certification of OCI behavior.

## Evidence and strengths

- src/main.go joins all regional planning before execution, freezes one schedule
  instant and applies a run timeout and signal cancellation.
- controller/tagcontroller.go paginates/deduplicates discovery, limits workers,
  and blocks execution of failed plans.
- controller/handler/handler.go supports Compute, VM DB nodes and Analytics;
  rechecks lifecycle state and distinguishes no-op, submitted and failed outcomes.
- ociapi/call.go applies a shared regional limiter and per-call deadline.
- Scheduler parsing and precedence, partial failures, cancellation, pagination,
  dry-run boundaries and CLI outcomes have mocked regression coverage.
- Existing GitHub Actions checks formatting, race tests, vet and build with
  read-only repository permissions and checkout credentials disabled.

## Remaining work, ordered by adoption gate

| Gate | Task and risk | Acceptance evidence |
| --- | --- | --- |
| Pilot blocker | Assign service owner, backup, security contact and pilot scope. Execution defaults to enabled, region defaults to all subscribed regions, and there is no compartment filter. | Approved pilot inventory; explicit region/timezone/namespace in scheduler; IAM restricted to disposable resources; prove unrelated resources cannot be changed. |
| Pilot blocker | Validate least-privilege discovery/read/power IAM and schedule-tag edit permissions. Tag writers influence execution. No deployable IAM example exists. | Reviewed sandbox policy examples and positive/negative authorization tests for each adopted service and principal type. |
| Pilot blocker | Exercise real OCI behavior. Mock tests cannot establish discovery completeness, SDK retry behavior, eventual consistency or asynchronous completion. | Authorized sandbox start/stop/no-op for each adopted service, multi-node DB verification, denied permissions, cancellation, DST examples and evidence of final states. Disable unvalidated services through scope/IAM. |
| Pilot blocker | Prevent overlapping invocations. State rechecks are not a distributed lock; two processes can race. | Single scheduler with enforced non-overlap, including manual runs, tested against a run exceeding its interval. Distributed leases can wait if only one executor exists. |
| Pilot blocker | Own failures and ambiguous submissions. A timeout can occur after OCI accepts a write, and multi-node results can be partial. | Operator rehearses stop-scheduler, reconcile-state and selective recovery; reports and OCI request IDs retained securely; no blind retry. |
| Pilot blocker | Verify dependencies and toolchain security. go.sum is tracked; no vulnerability scanner is enforced. | Recorded govulncheck run and dependency/license review, remediation or documented risk acceptance. Pin an approved scanner version before adding a required CI scan. |
| Pilot blocker | Require human review and passing CI in repository settings; documentation alone does not enforce them. | Named maintainers, protected default branch, required Go checks and reviewed dependency PRs. |
| Pilot blocker | Install monitoring and reproducible delivery process. No scheduler/deployment manifest or release workflow exists. | Build from reviewed revision; record binary checksum, revision and Go version; retain previous binary; alert on nonzero exits, missing runs and failed/ambiguous operations; test alert delivery. |
| Before wider internal use | Add explicit compartment/resource allowlist and maximum-operation guard, preferably explicit execution opt-in. Current principal and region are the only hard external scope boundaries. | Negative tests prove out-of-scope resources never get writes; operator reviews default behavior change. |
| Before wider internal use | Tune concurrency/limiting against realistic regional inventory. Eight workers and fixed per-region quotas have no comparative performance evidence. | Measured discovery/plan/run times fit the cadence and timeout under throttling; resource and memory bounds recorded. |
| Before wider internal use | Stabilize report contract and identity. JSON has timestamps/request IDs but no explicit schema version, build revision or unique run ID. | Consumers have compatibility fixtures; artifacts correlate run, revision and cloud audit events. |
| Production follow-up | Durable job history, distributed coordination, completion polling and stronger release provenance as requirements warrant. | Separate design and operational acceptance. These are not mandatory to a single-executor pilot with documented external controls. |

The future HTTP API and capacity scaling are separate product milestones, not
requirements for this CLI pilot. Exposing an API would require a new review of
identity, authorization, request validation, job persistence and network controls.

## Local validation

`go test -race -cover ./...` and `go vet ./...` passed on Go 1.26.5 darwin/arm64.
Coverage ranges from 16.7% to 85.0% in tested packages; task has 0.0%. Main is
84.8%, controller 77.8%, handler 76.4%. These are observations, not coverage gates.
The declared Go 1.24 toolchain still needs CI confirmation; this machine used a
newer Go version. No live OCI calls or vulnerability scan were run (govulncheck
is not installed). Build/format verification is recorded in the change handoff.

## Scaffolding supplied

AGENTS.md defines architecture, safety invariants, offline checks and AI handoff
rules. CONTRIBUTING.md, SECURITY.md, the PR template, dependency-update config,
Makefile and operations runbook establish a lightweight development baseline.
Dependabot requires repository-host support/enabling; branch protection, security
contacts, monitoring and IAM remain administrator/operator work.
