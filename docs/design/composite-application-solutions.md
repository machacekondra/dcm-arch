# Composite Application Definition: Three Approaches

**Date:** 2026-05-11
**Status:** Discussion

---

## Problem Statement

How should DCM allow users to define a **composite application** — a group of related infrastructure resources (database, compute, networking, etc.) that together form a single deployable unit?

We present three approaches that differ in **who defines the composition**, **where the dependency logic lives**, and **how the separation of concerns maps to personas**.

---

## Solution A: Developer-Authored Application

> Current design as described in [appspec.md](appspec.md)

### How It Works

Developers write an `Application` resource that declares the resources they need, wires them together with CEL expressions, and submits it via GitOps or the self-service portal. Platform engineers own the `ResourceType` definitions (schemas, inputs/outputs). Developers **compose** those building blocks.

### Example

```yaml
# Written by: Developer
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: my-web-app
spec:
  resources:
    - type: database.postgresql
      name: my-db
      properties:
        size: "S"

    - type: compute.container
      name: my-fe
      properties:
        image: "quay.io/dcm-project/frontend-example"
        ports:
          - 80:7000
      requirements:
        - my-db
```

### Persona Responsibilities

| Persona | Does |
|---------|------|
| **Platform Engineer** | Defines `ResourceType` schemas (inputs/outputs), placement policies, validation policies |
| **Developer** | Authors `Application` specs — picks resource types, sets properties, wires dependencies |
| **Infra Operator** | Registers environments, manages capacity, binds recipes |

### Pros

- Developers have full self-service autonomy
- Fast iteration — developers change their app spec without platform engineer involvement
- Clear ownership: the developer owns the application definition
- Scales well for teams with many small services

### Cons

- Developers must understand resource types, CEL wiring, and dependency graphs
- Risk of inconsistent patterns across teams (team A wires a web app differently than team B)
- Platform engineers have limited control over application-level composition
- Validation policies can catch mistakes but can't enforce "golden paths"

### Best Fit

Organizations with experienced developers who want maximum flexibility and self-service velocity.

---

## Solution B: Platform-Engineer-Authored Application Templates

### How It Works

Same resource model as Solution A (`Application`, `ResourceType`, CEL wiring), but **only platform engineers** can define Application specs. Developers consume pre-built application templates by selecting one and providing a limited set of parameters. Think of it as a "service catalog" of approved application patterns.

### Example

**Platform Engineer defines the template:**

```yaml
# Written by: Platform Engineer
apiVersion: dcm.io/v1alpha1
kind: ApplicationTemplate
metadata:
  name: web-app-with-database
  annotations:
    dcm.io/description: "Standard web application with a PostgreSQL database"
spec:
  parameters:
    schema:
      type: object
      required: [appName, image]
      properties:
        appName:
          type: string
          description: "Application name"
        image:
          type: string
          description: "Container image"
        dbSize:
          type: string
          enum: ["S", "M", "L"]
          default: "S"

  resources:
    - type: database.postgresql
      name: db
      properties:
        size: "${params.dbSize}"

    - type: compute.container
      name: frontend
      properties:
        image: "${params.image}"
        ports:
          - 80:7000
      requirements:
        - db
```

**Developer instantiates it:**

```yaml
# Written by: Developer
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: my-web-app
spec:
  template: web-app-with-database
  parameters:
    appName: my-web-app
    image: "quay.io/my-team/frontend:v2.3"
    dbSize: "M"
```

### Persona Responsibilities

| Persona | Does |
|---------|------|
| **Platform Engineer** | Defines `ResourceType` schemas **and** `ApplicationTemplate` compositions with approved wiring patterns |
| **Developer** | Picks a template, fills in parameters — no direct resource wiring |
| **Infra Operator** | Registers environments, manages capacity, binds recipes |

### Pros

- Enforces golden paths — all web apps follow the same architecture
- Reduces cognitive load on developers (no CEL, no dependency graphs)
- Platform engineers control composition quality and security posture
- Easier to audit and standardize across the organization

### Cons

- Less flexibility — developers must request new templates for non-standard patterns
- Platform engineers become a bottleneck for new application archetypes
- Template proliferation risk (web-app-small, web-app-medium, web-app-with-cache, ...)
- Parameterization can get complex if templates try to cover too many variations

### Best Fit

Regulated enterprises or platform teams that prioritize standardization, compliance, and "paved road" developer experiences.

---

## Solution C: Meta Service Provider (Application as Code)

### How It Works

Instead of declaring the composite application in YAML, the entire application is defined **in code** — using a general-purpose programming language (Go, Python, TypeScript). A Meta Service Provider is a code module that uses the DCM SDK to programmatically declare resources, resolve dependencies, apply conditional logic, and wire outputs. Think Pulumi or AWS CDK, but for DCM resource composition.

The code runs at plan time: the DCM engine executes the provider, collects the resulting resource graph, validates it, and then provisions it through the normal execution pipeline.

### Example

**Platform Engineer writes a Meta Service Provider in Go:**

```go
// Written by: Platform Engineer
// providers/web_stack/main.go
package main

import (
    "github.com/dcm-io/dcm/sdk"
)

type WebStackInputs struct {
    Image    string `json:"image"`
    DBSize   string `json:"dbSize" default:"S"`
    Replicas int    `json:"replicas" default:"2"`
    EnableCache bool `json:"enableCache" default:"false"`
}

type WebStackOutputs struct {
    URL                string `json:"url"`
    DBConnectionString string `json:"dbConnectionString" sensitive:"true"`
}

func main() {
    sdk.RunProvider(func(ctx *sdk.Context, inputs WebStackInputs) (*WebStackOutputs, error) {

        // Provision database
        db, err := ctx.Resource("database.postgresql", "db", map[string]any{
            "size": inputs.DBSize,
        })
        if err != nil {
            return nil, err
        }

        // Conditionally provision cache
        var cacheHost string
        if inputs.EnableCache {
            cache, err := ctx.Resource("cache.redis", "cache", map[string]any{
                "memoryGB": 4,
            })
            if err != nil {
                return nil, err
            }
            cacheHost = cache.Output("host")
        }

        // Provision compute — with dynamic env vars based on what's enabled
        envVars := map[string]string{
            "DATABASE_URL": db.Output("connectionString"),
        }
        if inputs.EnableCache {
            envVars["CACHE_HOST"] = cacheHost
        }

        app, err := ctx.Resource("compute.container", "app", map[string]any{
            "image":    inputs.Image,
            "replicas": inputs.Replicas,
            "ports":    []string{"80:7000"},
            "env":      envVars,
        })
        if err != nil {
            return nil, err
        }
        app.DependsOn(db)

        // Provision ingress
        ingress, err := ctx.Resource("network.ingress", "ingress", map[string]any{
            "targetPort":     80,
            "targetResource": "app",
        })
        if err != nil {
            return nil, err
        }
        ingress.DependsOn(app)

        return &WebStackOutputs{
            URL:                ingress.Output("url"),
            DBConnectionString: db.Output("connectionString"),
        }, nil
    })
}
```

**Same provider in Python:**

```python
# Written by: Platform Engineer
# providers/web_stack/main.py
import dcm

@dcm.provider(
    inputs={"image": str, "dbSize": "S", "replicas": 2, "enableCache": False},
    outputs={"url": str, "dbConnectionString": dcm.Sensitive(str)},
)
def web_stack(ctx: dcm.Context, inputs: dict) -> dict:

    # Provision database
    db = ctx.resource("database.postgresql", "db", size=inputs["dbSize"])

    # Conditionally provision cache
    cache = None
    if inputs["enableCache"]:
        cache = ctx.resource("cache.redis", "cache", memoryGB=4)

    # Build env vars dynamically
    env = {"DATABASE_URL": db.output("connectionString")}
    if cache:
        env["CACHE_HOST"] = cache.output("host")

    # Provision compute
    app = ctx.resource("compute.container", "app",
        image=inputs["image"],
        replicas=inputs["replicas"],
        ports=["80:7000"],
        env=env,
        depends_on=[db],
    )

    # Provision ingress
    ingress = ctx.resource("network.ingress", "ingress",
        targetPort=80,
        targetResource="app",
        depends_on=[app],
    )

    return {
        "url": ingress.output("url"),
        "dbConnectionString": db.output("connectionString"),
    }
```

**Register the provider as a ResourceType:**

```yaml
# Written by: Platform Engineer
apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: application.web-stack
  annotations:
    dcm.io/description: "Complete web application stack (code-defined)"
spec:
  version: "1.0.0"
  lifecycle: stable
  provider:
    type: code
    language: go
    source:
      repository: "https://git.example.com/dcm-providers/web-stack.git"
      version: "v1.2.0"
```

**Developer uses it like any other resource type:**

```yaml
# Written by: Developer
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: my-web-app
spec:
  resources:
    - type: application.web-stack
      name: my-stack
      properties:
        image: "quay.io/my-team/frontend:v2.3"
        dbSize: "M"
        replicas: 3
        enableCache: true
```

**Or composed into larger applications:**

```yaml
# A composite of composites — each meta provider expands independently
spec:
  resources:
    - type: application.web-stack
      name: public-site
      properties:
        image: "quay.io/my-team/public:v1.0"

    - type: application.web-stack
      name: admin-panel
      properties:
        image: "quay.io/my-team/admin:v1.0"
        dbSize: "L"
        enableCache: true

    - type: network.load-balancer
      name: lb
      properties:
        backends:
          - "${public-site.url}"
          - "${admin-panel.url}"
      requirements:
        - public-site
        - admin-panel
```

### Persona Responsibilities

| Persona | Does |
|---------|------|
| **Platform Engineer** | Writes Meta Service Providers in code (Go, Python, TypeScript) that compose primitive resources programmatically |
| **Developer** | Uses meta resource types like any other resource type — no awareness of internal implementation |
| **Infra Operator** | Registers environments, manages capacity, binds recipes |

### Pros

- Full power of a programming language — loops, conditionals, error handling, data transformation, API calls
- Handles complex conditional logic naturally (if/else, feature flags) — no awkward CEL workarounds
- Testable — providers are regular code with unit tests, integration tests, linting
- Composable — providers can call other providers, import shared libraries
- IDE support — autocompletion, type checking, refactoring, debugging
- Familiar to developers — no new DSL to learn, just use Go/Python/TypeScript
- Can fetch external data at plan time (e.g., look up latest AMI, check team quotas)

### Cons

- Requires a code execution sandbox in the DCM engine (security, resource limits, isolation)
- Harder to audit than declarative YAML — must read code to understand what gets provisioned
- Non-deterministic risk — code could produce different graphs on different runs if it calls external APIs
- Higher barrier to entry for platform engineers who prefer declarative config
- Debugging requires understanding both the provider code and the DCM execution model
- Versioning and dependency management of provider code adds operational complexity

### Best Fit

Organizations with strong platform engineering teams who need maximum expressiveness — complex conditional logic, dynamic resource graphs, integration with external systems at plan time. Similar philosophy to Pulumi/CDK vs. Terraform/CloudFormation.

---

## Comparison Matrix

| Dimension | A: Developer-Authored | B: PE-Authored Templates | C: Meta Service Provider (Code) |
|-----------|----------------------|--------------------------|--------------------------|
| **Who composes?** | Developer | Platform Engineer | Platform Engineer |
| **Definition format** | YAML | YAML | Go / Python / TypeScript |
| **Developer experience** | Full flexibility, higher learning curve | Pick & parameterize, low learning curve | Use like any resource, low learning curve |
| **Golden path enforcement** | Weak (policy-based) | Strong (template-based) | Strong (built into code) |
| **New kind required?** | No (`Application`) | Yes (`ApplicationTemplate`) | No (extends `ResourceType`) |
| **Conditional logic** | CEL expressions | CEL in template | Native if/else, loops, functions |
| **Recursive composition** | No | No | Yes (providers call providers) |
| **Wiring visibility** | Developer sees full graph | Developer sees parameters only | Developer sees single resource |
| **Testability** | Schema validation only | Schema validation only | Unit tests, integration tests |
| **PE bottleneck risk** | Low | High | Medium |
| **Debugging** | Easy (flat graph) | Medium (template + params) | Hard (code + expansion) |
| **Flexibility** | High | Low-Medium | Very High |
| **Standardization** | Low | High | High |
| **Engine complexity** | Low | Low | High (code sandbox required) |

---

## Recommendation

These solutions are **not mutually exclusive**. A phased approach could combine them:

1. **Start with A** — establish the resource model, let early adopters compose freely
2. **Add C** — let platform engineers encode proven patterns as code-based meta service providers
3. **Optionally add B** — if the organization wants a curated catalog with parameter-only developer experience

The key question for the team: **Where does composition authority live — with developers, platform engineers, or both?**

---

## Open Questions

1. Should Solution B templates support escape hatches (allow developers to add extra resources beyond the template)?
2. In Solution C, what sandbox model should be used for executing provider code? (containers, WASM, gRPC plugin?)
3. How do we ensure determinism in code-based providers — should external API calls at plan time be allowed or restricted?
4. Can Solution A and C coexist — developers compose primitive + code-defined meta resources in the same Application?
5. How does each solution affect multi-environment deployment and placement?
6. What SDKs/languages should be supported for Solution C, and in what order?
