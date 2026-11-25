# Security

This CLI can shut down cloud resources. Only an explicitly designated, tested
internal pilot environment should receive execution permissions before the
readiness gates in docs/READINESS.md are closed. No supported release/security
response commitment has been established yet.

Report suspected vulnerabilities privately to the repository's designated internal
maintainer through the organization's approved security channel. If no maintainer
is assigned, have the repository administrator establish that channel before
adoption. Do not open public issues with credentials, tenant IDs or resource dumps.
Include the affected revision, synthetic reproduction and expected impact.

Keep OCI credentials outside this repository. Prefer managed principals where
appropriate; review discovery and service power permissions separately. Restrict
who can edit scheduling tags: tag edits can effectively request power changes.
Treat JSON reports and debug output as sensitive infrastructure metadata and use
access-controlled storage with an organization-approved retention period.

Run dependency vulnerability review before pilot and on updates. Existing CI
checks test behavior; they do not establish absence of vulnerabilities or prove
least-privilege IAM. See CONTRIBUTING.md for the optional local scanner target.
