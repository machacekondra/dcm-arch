# DCM Application Design Document

**Status:** Draft
**Date:** 2026-04-23
**Authors:** DCM Team

---

## 1. Overview

The DCM App Specification (Application) is a declarative, YAML-based format that allows developers to describe an application's specification as a single unit. Developers submit Applications through the self-service portal or commit them to Git repositories managed by gitops.

### 1.1 Design Principles

1. **Declare what, not where.** Developers specify what resources they need and how they connect. Environment selection (dev/staging/prod, datacenter, region) is handled by platform policy and scheduling — developers don't need to care about it.

2. **Consume, don't define.** Resource type schemas are owned by platform engineers. Developers reference resource types by name and provide inputs.

3. **CEL-based wiring.** Resources reference each other's outputs via CEL expressions in `${}` syntax, forming an implicit dependency graph.

4. **GitOps-native.** Applications are designed to live in Git and be applied through the gitops pipeline, enabling version control, review, and auditability.

---

## 2. Top-Level Structure

```yaml
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: my-web-app
spec:
  resources:
    - type: database.postgresql
      name: 'my-db'
      properties:
        size: 'S'
    - type: compute.container
      name: 'my-fe'
      properties:
        image: 'quay.io/dcm-project/frontend-example'
        ports:
          - 80:7000
      requirements:
        - my-db
```

### 2.1 Field Reference

| Field | Required | Description |
|-------|----------|-------------|
| `apiVersion` | Yes | API version. Currently `dcm.io/v1alpha1`. |
| `kind` | Yes | Always `Application`. |
| `metadata.name` | Yes | Unique name within a namespace. |
| `spec.resources` | Yes | List of resource declarations (see [Section 4](#4-resource-declarations)). |

## 3. Resource Declarations

Each entry in `spec.resources` declares a single resource.

```yaml
resources:
  - type: compute.virtual-machine
    properties:
      cpu: 2
      memory: 8GiB
      os: "rhel-10"
    requirements:
      - mydb
```

### 4.1 Field Reference

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Name of a registered Generic Resource Type (e.g., `virtual-machine`, `load-balancer`, `database`). |
| `properties` | Yes | Key-value map of inputs matching the resource type input schema. Values can be literals or CEL expressions. |
| `requirements` | No | Explicit list of resource `name` that must be provisioned before this resource, supplementing implicit CEL-based dependencies. |

## 5. CEL Expressions

Application uses [Common Expression Language (CEL)](https://cel.dev/) for dynamic values, following the same `${}` syntax.

### 5.1 Syntax

All CEL expressions are wrapped in `${}`:

```
${expression}
```

Expressions can be standalone values or embedded within strings for interpolation:

```yaml
# Standalone — the entire value is a CEL expression
targetPort: "${db.appPort}"

# Interpolated — CEL embedded within a string
connectionUrl: "postgres://${db.host}:${db.port}/${params.dbName}"
```

### 5.2 Expression Contexts

| Prefix | Resolves To | Example |
|--------|-------------|---------|
| `<resourceName>.<field>` | An output of another resource | `${db.host}` |

Cross-resource references (e.g., `${db.host}`) resolve to the resource type **output schema** for that resource. The available output fields are defined by the resource types, not by the developer.

## 6. Conditional Inclusion

Resources can be conditionally included using the `includeWhen` field. The value is a CEL expression that must evaluate to a boolean.

```yaml
resources:
  - id: cache
    type: redis-cache
    includeWhen: "${params.enableCache == true}"
    inputs:
      memoryGB: 4

  - id: monitoring
    type: monitoring-agent
    includeWhen: "${params.enableMonitoring == true}"
    inputs:
      targetHost: "${web.privateIP}"
```

### 6.1 Behavior

- When `includeWhen` evaluates to `true` (or is omitted): the resource is provisioned normally.
- When `includeWhen` evaluates to `false`: the resource is **not** provisioned. It does not appear in the dependency graph.
- **Null propagation:** If resource B references an output of conditional resource A (e.g., `${cache.host}`), and A is excluded, the reference evaluates to the zero value for its type (`""` for strings, `0` for integers, `false` for booleans). Use the `?` operator for explicit null handling.

### 6.2 Soundness Warning

If resource B references resource A, and A has `includeWhen` but B does not, the system emits a compile-time warning. This alerts the developer that B may receive zero/null values when A is excluded. To resolve, either:
- Add a matching `includeWhen` to B, or
- Use the `?` operator and handle the absence in the input logic.

---

## 7. Dependency Graph

Application resources form a Directed Acyclic Graph (DAG) that determines provisioning order.

### 7.1 Implicit Dependencies

Dependencies are **automatically** detected by scanning CEL expressions for cross-resource references:

```yaml
resources:
  - name: db
    type: database
    properties:
      dbName: "${params.dbName}"

  - name: app
    type: virtual-machine
    properties:
      dbUrl: "${db.connectionString}"   # app depends on db
```

The reference `${db.connectionString}` creates an implicit dependency: `app` depends on `db`.

### 7.2 Explicit Dependencies

The `requirements` field allows declaring dependencies that cannot be inferred from data references:

```yaml
- name: frontend
  type: compute.container
  requirements:
    - db
  parameters:
    image: quay.io/dcm-project/myproject
```

This will inject into the container as environment variables all output variables of the `db`.

### 7.3 Provisioning Order

The system resolves the DAG via topological sort and provisions resources in dependency order. Resources with no dependencies between them may be provisioned in parallel.

### 7.4 Cycle Detection

Circular dependencies are detected at compile time. If the dependency graph contains a cycle, the Application is rejected with an error identifying the cycle:

```
Error: circular dependency detected: app -> db -> cache -> app
```

---

## 8. Validation

The system performs compile-time validation before any provisioning begins. All errors are reported together, not one at a time.

| Check | Description |
|-------|-------------|
| **resource type existence** | Every `type` must reference a registered resource type. |
| **Input schema validation** | Every key in `proeprties` must exist in the resource type input schema. Types must match. Required fields must be present. Values must satisfy constraints (min, max, enum, pattern). |
| **CEL expression parsing** | All `${}` expressions must be syntactically valid CEL. |
| **Output reference validation** | Cross-resource references (e.g., `${db.host}`) must match a field in the referenced resource's resource type output schema. |
| **DAG validation** | No circular dependencies. |
| **Unique IDs** | All resource `id` values must be unique within the Application. |
| **Conditional soundness** | Warning if a resource references a conditional resource without its own `includeWhen` or `?` operator. |

---

## Glossary

| Term | Definition |
|------|------------|
| **Application** | A declarative YAML document describing an full application. |
| **resource type** | An infrastructure abstraction (e.g., VM, LoadBalancer) defined by platform engineers, with a fixed input/output schema. |
| **CEL** | Common Expression Language — used for dynamic values, cross-resource references, and conditionals. |
| **DAG** | Directed Acyclic Graph — the dependency graph formed by cross-resource references. |
