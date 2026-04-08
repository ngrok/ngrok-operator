# Spec Format Reference

Canonical Mermaid and Gherkin examples for the `spec-synthesis` skill.
Copy these patterns directly into generated spec files.

---

## 1. Architecture Diagrams — `graph TD` / `graph LR`

Use `graph TD` (top-down) for layered architectures:

````markdown
```mermaid
graph TD
    Client["Client (Browser / CLI)"]
    Gateway["API Gateway"]
    Auth["Auth Service"]
    Core["Core Business Logic"]
    DB[("Database")]
    Cache[("Cache")]
    Queue[/"Message Queue"/]

    Client -->|HTTP Request| Gateway
    Gateway -->|Validate Token| Auth
    Auth -->|Claims| Gateway
    Gateway -->|Invoke| Core
    Core -->|Read / Write| DB
    Core -->|Cache Lookup| Cache
    Core -->|Publish Event| Queue
```
````

Use `graph LR` (left-right) to show horizontal layering with subgraphs:

````markdown
```mermaid
graph LR
    subgraph Presentation
        UI["Web UI"]
        CLI["CLI"]
    end
    subgraph Application
        Handler["Request Handlers"]
        Service["Business Services"]
    end
    subgraph Infrastructure
        Repo["Repositories"]
        Queue["Message Queue"]
        Cache[("Cache")]
    end

    UI --> Handler
    CLI --> Handler
    Handler --> Service
    Service --> Repo
    Service --> Queue
    Service --> Cache
```
````

---

## 2. Entity Relationship Diagrams — `erDiagram`

Use logical domain names, not database column names.
Cardinality notation: `||--||` (one-to-one), `||--o{` (one-to-many), `}o--o{` (many-to-many).

````markdown
```mermaid
erDiagram
    USER {
        string id PK
        string email
        string displayName
        datetime createdAt
        datetime updatedAt
    }
    ORGANIZATION {
        string id PK
        string name
        string slug
    }
    PROJECT {
        string id PK
        string name
        string status
        string orgId FK
        string ownerId FK
    }
    MEMBERSHIP {
        string userId FK
        string orgId FK
        string role
    }

    USER ||--o{ PROJECT : "owns"
    ORGANIZATION ||--o{ PROJECT : "contains"
    USER ||--o{ MEMBERSHIP : "has"
    ORGANIZATION ||--o{ MEMBERSHIP : "has"
```
````

---

## 3. Logic Flow Diagrams — `flowchart TD`

Use rounded rectangles for start/end (`([...])`), diamonds for decisions (`{...}`),
rectangles for actions (`[...]`).

````markdown
```mermaid
flowchart TD
    Start([Request Received]) --> Validate{Input Valid?}
    Validate -->|No| BadRequest([Return 400 Bad Request])
    Validate -->|Yes| Auth{Token Present?}
    Auth -->|No| Unauth([Return 401 Unauthorized])
    Auth -->|Yes| VerifyToken[Verify Token Signature]
    VerifyToken --> Expired{Token Expired?}
    Expired -->|Yes| Unauth
    Expired -->|No| Authz{Has Permission?}
    Authz -->|No| Forbidden([Return 403 Forbidden])
    Authz -->|Yes| Process[Execute Business Logic]
    Process --> Persist[Persist to Store]
    Persist --> Notify[Publish Domain Event]
    Notify --> Success([Return 200 OK])
```
````

---

## 4. State Machine Diagrams — `stateDiagram-v2`

Use `stateDiagram-v2` for entity lifecycle or workflow state machines.

````markdown
```mermaid
stateDiagram-v2
    [*] --> Draft : create()

    Draft --> Submitted : submit()
    Draft --> Cancelled : cancel()

    Submitted --> UnderReview : assign(reviewer)
    Submitted --> Draft : retract()

    UnderReview --> Approved : approve()
    UnderReview --> ChangesRequested : requestChanges()

    ChangesRequested --> Submitted : resubmit()

    Approved --> Published : publish()
    Approved --> Rejected : reject()

    Rejected --> Draft : revise()

    Published --> Archived : archive()
    Archived --> [*]
    Cancelled --> [*]
```
````

---

## 5. Sequence Diagrams — `sequenceDiagram`

Use for documenting request/response flows between components.

````markdown
```mermaid
sequenceDiagram
    participant C as Client
    participant G as API Gateway
    participant A as Auth Service
    participant S as Business Service
    participant D as Database
    participant Q as Event Queue

    C->>G: POST /api/orders {payload}
    G->>A: ValidateToken(bearer)
    A-->>G: Claims{userId, roles}
    G->>S: CreateOrder(userId, payload)
    S->>D: INSERT order
    D-->>S: Order{id, status: "pending"}
    S->>Q: Publish(OrderCreated{orderId})
    Q-->>S: ack
    S-->>G: OrderDTO{id, status}
    G-->>C: 201 Created {id, status}
```
````

---

## 6. Gherkin Feature / Scenario Blocks

Use for behavioral specifications. Describe observable outcomes, not implementation.

````markdown
```gherkin
Feature: Order Creation
  As an authenticated customer
  I want to submit a new order
  So that my purchase is recorded and fulfillment begins

  Background:
    Given the user is authenticated with role "customer"
    And the product catalog is available

  Scenario: Successful order with valid items
    Given the cart contains 2 units of product "P-001"
    And the product "P-001" has sufficient inventory
    When the user submits the order
    Then the system creates an order in "pending" status
    And an "OrderCreated" event is published
    And the user receives a confirmation with an order ID

  Scenario: Order rejected due to insufficient inventory
    Given the cart contains 5 units of product "P-002"
    And the product "P-002" has only 3 units in inventory
    When the user submits the order
    Then the system returns a 409 Conflict error
    And the error message identifies "P-002" as the out-of-stock item
    And no order is created
    And no event is published

  Scenario: Order rejected for unauthenticated user
    Given the user is not authenticated
    When the user attempts to submit an order
    Then the system returns a 401 Unauthorized error
```
````

---

## 7. Pending Clarification Format

Emit this blockquote wherever logic cannot be determined from the code. Be specific
about what is ambiguous and what information would resolve it.

```markdown
> ⚠️ **Pending Clarification:** The retry logic in `PaymentService.processRefund()`
> references a `MAX_RETRIES` constant that is not defined in the observed codebase.
> It may be injected via an environment variable or a missing configuration file.
> **To resolve:** Confirm whether `PAYMENT_MAX_RETRIES` env var controls this limit,
> and document the backoff strategy (linear vs. exponential) and the behavior after
> the maximum is exceeded (dead-letter queue, alert, silent fail, etc.).
```

---

## 8. Source References Table

Every spec file must end with this section, listing the actual files and line ranges
examined when writing the spec. This makes specs auditable and falsifiable.

```markdown
## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| `OrderService` class | `src/services/order.service.ts` | 1–180 |
| `Order` domain model | `src/domain/order.ts` | 1–55 |
| `OrderCreated` event | `src/events/order.events.ts` | 12–28 |
| Order creation route | `src/routes/orders.router.ts` | 34–60 |
| Inventory check logic | `src/services/inventory.service.ts` | 88–115 |
```

---

## 9. Full Spec File Template

````markdown
# <Title>

> <One-sentence description of what this spec covers>

<!-- Last updated: YYYY-MM-DD -->

## Overview

<2–4 sentence narrative introduction to the scope of this spec file.>

## <Major Section 1>

<Content — Mermaid diagrams, tables, prose.>

> ⚠️ **Pending Clarification:** <if applicable>

## <Major Section 2>

<Content — Gherkin scenarios, flowcharts, etc.>

## Source References

| Symbol / Concept | File | Lines |
|-----------------|------|-------|
| `ExampleClass` | `src/example.ts` | 1–50 |
````
