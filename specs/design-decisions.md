# Design Decisions

This document captures architectural trade-offs that have been weighed and decided. These are not part of the spec itself, but provide context for why the spec is shaped the way it is. Future contributors should understand these decisions before proposing changes that revisit them.

## Follow Kubernetes API conventions

All CRD types follow the conventions outlined in the [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md). This includes field naming, serialization (`omitempty`), optional vs required semantics, pointer usage, and status conventions. When in doubt, defer to that document.

## Defer validation to the ngrok API

The operator does not attempt to replicate ngrok API validation rules at admission time (e.g., via CEL x-validation or validating webhooks). Fields like `spec.url` accept complex, multi-format input that would require brittle duplication of server-side logic to validate client-side. Instead, the operator passes values through to the ngrok API and surfaces any errors via status conditions.

**Trade-off:** Users see validation errors asynchronously (after reconciliation) rather than synchronously (at apply time). This is acceptable because:
- The ngrok API is the authoritative source of validation rules, and those rules change over time.
- Duplicating validation creates a maintenance burden and risks divergence between client and server rules.
- Status conditions and events provide clear feedback when the API rejects a value.

## Compute wildcard domain coverage client-side, but confirm it with the API

The operator derives a hostname's wildcard parent locally — `a.example.com` →
`*.example.com` — and then asks the ngrok API whether that wildcard is
reserved, so it can skip reserving the child. See
[controllers/domain.md](controllers/domain.md#wildcard-domain-coverage).

This may look like it cuts against [Defer validation to the ngrok
API](#defer-validation-to-the-ngrok-api), but it does not: the operator is not
replicating a validation rule, it is deciding which resource to *query* for.
Authority over what is reserved stays with the API, which answers the question.
What is computed locally is only DNS wildcard semantics, which are fixed by the
DNS and TLS specifications rather than by ngrok — a wildcard matches exactly one
label, and never its own apex.

**Trade-off:** the operator needs its own notion of "direct child", so a change
in how ngrok matches wildcards would require a code change here. This is
acceptable because those semantics come from the DNS standard, not from ngrok
policy, and the alternative — reserving every subdomain and letting the API
sort it out — is the problem this behavior exists to solve.

No public-suffix list is consulted. For a two-label hostname (such as `example.com`),
no wildcard parent is derived because `WildcardParentDomain` requires at least three
labels to avoid synthesizing `*.<tld>` (which could not be reserved anyway); the
operator queries only for the exact hostname. Carrying a PSL to distinguish multi-label
public suffixes (like `co.uk`) would add an embedded table and a staleness obligation
for no behavioral gain.
