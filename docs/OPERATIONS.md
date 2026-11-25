# Controlled internal pilot runbook

## Before enabling writes

Close the pilot gates in [READINESS.md](READINESS.md). Record the owner and backup,
approved resources/services, principal, region, timezone, tag namespace, cadence,
report retention location and alert destination in an access-controlled internal
record. Do not commit tenant inventories or credentials here.

Build a reviewed revision with `make check`. Record revision, toolchain version
and binary checksum; retain the previous known-good binary. Run with a dedicated,
least-privilege principal. Have the IAM reviewer confirm Resource Search visibility
and service power permissions, including that unrelated resources are excluded.
Restrict Schedule tag editing to approved administrators.

Use one executor and a scheduler that prevents overlapping invocations. Scheduler
protection must also cover manual runs. A process-local state check cannot prevent
multiple hosts issuing requests. Set explicit region, timezone, namespace, timeout
and action. Set `-rate` and `-burst` to the budget allocated to this tool in each
region (defaults: 2 attempts/second, burst 4), leaving headroom for other callers.
Review regional API usage summaries for attempts, retries, throttling and cumulative
limiter wait. Independent invocations do not share budgets. Do not rely on host
timezone or the all-regions default.

## Preview and enable

For an explicitly authorized sandbox target (replace example values):

```sh
/tmp/oci-frugal -dry-run -region us-ashburn-1 -tz America/New_York -tag PilotSchedule -timeout 10m
```

This authenticates and reads OCI. Inspect the entire JSON report: resource IDs,
actions, all DB node operations, skipped resources, errors and expected inventory.
Discovery is not proof that all intended resources were visible. Compare against
an independently approved inventory. Fix all planning failures before execution.

After approval for sandbox writes, use the same options without `-dry-run`.
Execution replans; it does not consume the previously reviewed dry-run artifact.
Changes to tags/inventory between runs therefore affect the actions. Keep the
pilot small and review the fresh execution report. Compute defaults to soft stop;
use hard stop only when explicitly approved. Verify actual final states in OCI:
`submitted` only confirms request acceptance.

Store stdout JSON and stderr separately with restrictive access. Alert on exit 1
(runtime failure), exit 2 (configuration error), missing/late runs and per-operation
failures. Exit 0 does not establish eventual completion. Monitor completion via
OCI during the pilot. Do not automatically loop failed invocations.

## Failure and recovery

1. Disable the external scheduler and ensure no other invocation remains active.
   Cancellation prevents future work where possible; accepted cloud requests persist.
2. Preserve the JSON report, diagnostics, binary revision and OCI request IDs in
   restricted storage. Inspect every node operation for partially successful DB work.
3. Reconcile actual cloud states and audit events, particularly after request
   timeouts. Do not assume an error means OCI rejected a write.
4. Correct IAM, tags or resource state through the approved operator process.
   Restore desired power state selectively if needed; there is no transactional undo.
5. Preview again and authorize a scoped retry only after states are stable. Re-enable
   the scheduler once the cause is understood and the owner approves recovery.

For a binary regression, disable scheduling, restore the retained known-good
binary/configuration, preview and validate before resuming. Reverting code does
not reverse power operations already accepted by OCI.
