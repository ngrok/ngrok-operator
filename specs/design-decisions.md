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

**Scope:** This decision covers *value* validation — whether a given field value is
acceptable to the ngrok API. It does not cover **immutability**, which is a statement about
a CRD's own lifecycle rather than a server-side rule being duplicated. CEL transition rules
(`self == oldSelf`) are used where the ngrok API has no corresponding mutation at all, so
there is nothing to defer to and nothing that can diverge:

- `Domain.spec.domain` — see [Domain CRD: Immutability](crds/domain.md#immutability).

Deferring is not an option in these cases. Because the API exposes no way to perform the
change, the operator never sends a request and the API never returns an error, so there is
no asynchronous failure to surface. The edit is silently dropped instead, leaving the
resource reporting `Ready` against the old value. Rejecting at admission is the only way
the user learns the change did not happen.
