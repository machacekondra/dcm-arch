# DCM ResourceType Design Document

**Status:** Draft
**Date:** 2026-04-27
**Authors:** DCM Team

---

## 1. Overview

The ResourceType is the Platform Engineer's primary abstraction artifact. It defines a contract between the three personas: it tells **Developers** what properties they can set and what outputs they can reference, it tells **Infrastructure Operators** what capability name to register in their Environments, and it tells the **Execution Engine** what to pass to provisioners and what to expect back.

A ResourceType defines the **WHAT** (interface) — not the **HOW** (implementation). The HOW is handled by **Recipes** — Terraform modules, Ansible playbooks, Helm charts, or other IaC templates — registered per-environment. Recipes receive the developer-provided properties directly (no mapping layer) and must return a standardized `result` object whose fields match the resource type's read-only properties.

### 1.1 Design Principles

1. **Interface, not implementation.** A ResourceType is a schema contract. It declares inputs and outputs without prescribing how provisioning happens. The same resource type can be backed by entirely different Recipes in different environments.

2. **Schema-first.** The resource type schema uses OpenAPI v3. Every property has a declared type, constraints, and documentation. This enables compile-time validation in the Application pipeline (see [appspec.md](appspec.md), Section 8).

3. **Unified schema, split by `readOnly`.** Inputs (developer-settable) and outputs (Recipe-provided) live in a single `properties` block. Outputs are marked `readOnly: true`. This gives developers one place to see everything a resource type offers.

4. **No mapping layer.** Recipes receive properties by name as-is. Recipe authors are responsible for accepting the same property names defined in the resource type schema. This keeps the system simple and pushes transformation logic into the Recipe itself.

5. **Standardized result contract.** Every Recipe must output a `result` object with `values` (non-sensitive outputs) and `secrets` (sensitive outputs). The Execution Engine validates the result against the resource type's `readOnly` properties.

6. **Context injection.** The Execution Engine auto-injects a `context` object into every Recipe invocation with resource, application, and environment metadata — enabling Recipes to generate unique, repeatable names.

7. **Versioned lifecycle.** Resource types follow semantic versioning with explicit deprecation support.

8. **Consistent structure.** Follows the same `apiVersion` / `kind` / `metadata` / `spec` YAML conventions as Application, Environment, and PlacementPolicy.

---

## 2. Top-Level Structure

```yaml
apiVersion: dcm.io/v1alpha1
kind: ResourceType
metadata:
  name: database.postgresql
  labels:
    category: database
    vendor: postgresql
  annotations:
    dcm.io/description: "A managed PostgreSQL database instance."
    dcm.io/documentation: "https://docs.example.com/resource-types/database-postgresql"
spec:
  version: "1.0.0"
  lifecycle: stable

  schema:
    type: object
    required: [size]
    properties:
      size:
        type: string
        description: "Instance size class."
        enum: ["XS", "S", "M", "L", "XL"]
        default: "S"
      version:
        type: string
        description: "PostgreSQL major version."
        enum: ["14", "15", "16", "17"]
        default: "16"
      storageGB:
        type: integer
        description: "Storage allocation in GiB."
        minimum: 10
        maximum: 10000
        default: 50
      multiAZ:
        type: boolean
        description: "Enable multi-AZ replication for high availability."
        default: false
      host:
        type: string
        description: "Database hostname or IP address."
        readOnly: true
      port:
        type: integer
        description: "Database port number."
        readOnly: true
      connectionString:
        type: string
        description: "Full connection string (postgres://host:port/dbname)."
        readOnly: true
      username:
        type: string
        description: "Admin username."
        readOnly: true
        x-dcm-sensitive: true
      password:
        type: string
        description: "Admin password."
        readOnly: true
        x-dcm-sensitive: true
```

### 2.1 Field Reference

| Field | Required | Description |
|-------|----------|-------------|
| `apiVersion` | Yes | API version. Currently `dcm.io/v1alpha1`. |
| `kind` | Yes | Always `ResourceType`. |
| `metadata.name` | Yes | Unique name, used in Application `type` fields and Environment `capabilities.resourceTypes`. Dot-separated naming: `<category>.<technology>` (e.g., `database.postgresql`, `compute.container`). |
| `metadata.labels` | No | Labels for catalog filtering and policy matching. |
| `metadata.annotations` | No | Annotations for documentation URLs, descriptions, and other non-filtering metadata. |
| `spec.version` | Yes | Semantic version of this resource type definition. |
| `spec.lifecycle` | Yes | Lifecycle stage: `draft`, `stable`, or `deprecated`. |
| `spec.deprecation` | No | Present only when `lifecycle: deprecated`. Contains migration guidance and deadline. |
| `spec.schema` | Yes | OpenAPI v3 schema defining both input properties and read-only outputs. |

---

## 3. Schema (OpenAPI v3)

The `spec.schema` is a standard [OpenAPI v3 Schema Object](https://spec.openapis.org/oas/v3.0.3#schema-object). It defines all properties in a single block. Developer-settable properties and Recipe-provided outputs are distinguished by the `readOnly` flag.

### 3.1 Input Properties

Properties without `readOnly` (or with `readOnly: false`) are **inputs** — developers set them in their Application's `properties` block. The Application controller validates developer-provided values against these schemas at compile time.

```yaml
size:
  type: string
  description: "Instance size class."
  enum: ["XS", "S", "M", "L", "XL"]
  default: "S"
```

### 3.2 Output Properties

Properties with `readOnly: true` are **outputs** — set by the Recipe after provisioning. Developers can reference them via CEL expressions (e.g., `${db.host}`) but cannot set them. The Application controller validates all CEL cross-resource references against these properties at compile time.

```yaml
host:
  type: string
  description: "Database hostname or IP address."
  readOnly: true
```

## 4. Recipes

While ResourceTypes define the interface, **Recipes** define the implementation. A Recipe is a Terraform module, Ansible playbook, Helm chart, or other IaC template that provisions the actual infrastructure and returns outputs matching the resource type's `readOnly` properties.

### 4.1 Why Recipes?

The same resource type (e.g., `database.postgresql`) may be provisioned by entirely different tools in different environments:

| Environment Type | Recipe Type | Implementation |
|-----------------|-------------|----------------|
| `aws` | Terraform | `aws_rds_instance` resource |
| `kubernetes` | Helm | CloudNativePG Helm chart |
| `bare-metal` | Ansible | Playbook installing PostgreSQL |
| `azure` | Terraform | `azurerm_postgresql_flexible_server` |

Recipes are registered per-environment (see Section 5), keeping the ResourceType definition clean and environment-agnostic.

### 4.2 Recipe Contract

Every Recipe must:

1. **Accept a `context` variable** — auto-injected by the Execution Engine (see Section 6).
2. **Accept input properties as variables** — using the same names as the resource type schema's non-`readOnly` properties.
3. **Output a `result` object** — with `values` (non-sensitive outputs) and `secrets` (sensitive outputs) matching the resource type's `readOnly` properties.

### 4.3 Result Object

The `result` output is the standardized contract between Recipes and the Execution Engine:

```
result:
  values:     # non-sensitive readOnly properties
    host: "db-prod-01.example.com"
    port: 5432
    connectionString: "postgres://db-prod-01.example.com:5432/mydb"
  secrets:    # readOnly properties marked x-dcm-sensitive
    username: "admin"
    password: "s3cur3p4ss"
```

| Field | Required | Description |
|-------|----------|-------------|
| `result.values` | Yes | Map of non-sensitive output property names to their values. Keys must match `readOnly` properties not marked `x-dcm-sensitive`. |
| `result.secrets` | Yes | Map of sensitive output property names to their values. Keys must match `readOnly` properties marked `x-dcm-sensitive`. |

The Execution Engine validates the `result` against the resource type schema:
- Every `readOnly` property must appear in either `values` or `secrets`.
- Properties marked `x-dcm-sensitive` must appear in `secrets`, not `values`.
- Missing outputs cause a provisioning error.

### 4.4 Terraform Recipe Example

**variables.tf:**
```hcl
variable "context" {
  description = "DCM-injected metadata about the resource, application, and environment."
  type        = any
}

variable "size" {
  description = "Instance size class."
  type        = string
  default     = "S"
}

variable "version" {
  description = "PostgreSQL major version."
  type        = string
  default     = "16"
}

variable "storageGB" {
  description = "Storage allocation in GiB."
  type        = number
  default     = 50
}

variable "multiAZ" {
  description = "Enable multi-AZ replication."
  type        = bool
  default     = false
}
```

**main.tf:**
```hcl
locals {
  instance_class = {
    XS = "db.t3.micro"
    S  = "db.t3.small"
    M  = "db.r6g.large"
    L  = "db.r6g.xlarge"
    XL = "db.r6g.2xlarge"
  }
}

resource "aws_db_instance" "main" {
  identifier          = "${var.context.resource.name}-${var.context.environment.name}"
  engine              = "postgres"
  engine_version      = var.version
  instance_class      = local.instance_class[var.size]
  allocated_storage   = var.storageGB
  multi_az            = var.multiAZ
  username            = "admin"
  password            = random_password.db.result
}

resource "random_password" "db" {
  length  = 32
  special = true
}

output "result" {
  value = {
    values = {
      host             = aws_db_instance.main.address
      port             = aws_db_instance.main.port
      connectionString = "postgres://${aws_db_instance.main.address}:${aws_db_instance.main.port}/postgres"
    }
    secrets = {
      username = aws_db_instance.main.username
      password = random_password.db.result
    }
  }
  sensitive = true
}
```

### 4.5 Ansible Recipe Example

**provision.yml:**
```yaml
- name: Provision PostgreSQL
  hosts: "{{ dcm_context.environment.name }}"
  vars:
    db_name: "{{ dcm_context.resource.name }}_db"
    pg_version: "{{ version | default('16') }}"
    shared_buffers:
      XS: "256MB"
      S: "512MB"
      M: "2GB"
      L: "8GB"
      XL: "16GB"
  tasks:
    - name: Install PostgreSQL
      ansible.builtin.package:
        name: "postgresql-{{ pg_version }}"
        state: present

    - name: Configure shared_buffers
      ansible.builtin.lineinfile:
        path: /etc/postgresql/{{ pg_version }}/main/postgresql.conf
        regexp: '^shared_buffers'
        line: "shared_buffers = {{ shared_buffers[size | default('S')] }}"

    - name: Set result
      ansible.builtin.set_fact:
        result:
          values:
            host: "{{ ansible_host }}"
            port: 5432
            connectionString: "postgres://{{ ansible_host }}:5432/{{ db_name }}"
          secrets:
            username: "postgres"
            password: "{{ generated_password }}"
```

### 4.6 Helm Recipe Example

**values.yaml** (injected by DCM):
```yaml
context:
  resource:
    name: my-db
  application:
    name: my-web-app
  environment:
    name: prod-eu-k8s-01
    type: kubernetes
size: "M"
version: "16"
storageGB: 100
```

The Helm chart returns the `result` via a ConfigMap/Secret that DCM reads after installation.

---

## 5. Recipe Registration

Recipes are registered on Environments, binding a resource type to a specific provisioner implementation for that environment. This is done via the Environment spec or through the control plane API.

### 5.1 Registration in Environment Spec

```yaml
apiVersion: dcm.io/v1alpha1
kind: Environment
metadata:
  name: prod-eu-k8s-01
spec:
  type: kubernetes
  # ... other fields ...

  capabilities:
    resourceTypes:
      - compute.container
      - compute.service
      - network.ingress
      - database.postgresql

  recipes:
    database.postgresql:
      default:
        type: helm
        source:
          repository: "https://charts.dcm.io"
          chart: "cnpg-postgresql"
          version: "2.1.0"
        parameters:
          storageClass: "ssd"
      cnpg-ha:
        type: helm
        source:
          repository: "https://charts.dcm.io"
          chart: "cnpg-postgresql-ha"
          version: "2.1.0"

    compute.container:
      default:
        type: helm
        source:
          repository: "https://charts.dcm.io"
          chart: "generic-deployment"
          version: "1.5.0"
```

### 5.2 Registration via Standalone Resource

For environments managed by separate teams, recipes can also be registered as standalone resources:

```yaml
apiVersion: dcm.io/v1alpha1
kind: Recipe
metadata:
  name: database-postgresql-terraform-aws
  labels:
    resourceType: database.postgresql
spec:
  resourceType: database.postgresql
  resourceTypeVersion: ">=1.0.0, <2.0.0"

  type: terraform
  source:
    registry: "registry.terraform.io"
    module: "dcm-modules/rds-postgresql/aws"
    version: "3.2.1"

  environmentMatch:
    types:
      - aws

  parameters:
    backup_retention_period: 7
    storage_encrypted: true
```

### 5.3 Recipe Field Reference

| Field | Required | Description |
|-------|----------|-------------|
| `type` | Yes | Recipe type: `terraform`, `ansible`, `helm`, `kubernetes-operator`, `pulumi`, `custom`. |
| `source` | Yes | Location of the IaC template. Fields vary by type. |
| `parameters` | No | Default parameter values set by the operator. These are merged with developer-provided properties (developer values take precedence). |

### 5.4 Recipe Source by Type

**Terraform:**
```yaml
type: terraform
source:
  registry: "registry.terraform.io"
  module: "dcm-modules/rds-postgresql/aws"
  version: "3.2.1"
```

**Ansible:**
```yaml
type: ansible
source:
  repository: "https://git.example.com/ansible/database-playbooks.git"
  playbook: "postgresql/provision.yml"
  version: "v2.1.0"
```

**Helm:**
```yaml
type: helm
source:
  repository: "https://charts.dcm.io"
  chart: "cnpg-postgresql"
  version: "2.1.0"
```

**Kubernetes Operator:**
```yaml
type: kubernetes-operator
source:
  apiGroup: "postgresql.cnpg.io"
  kind: "Cluster"
  version: "v1"
```

### 5.5 Recipe Selection

When an Application resource is provisioned:

1. The Execution Engine resolves the target environment via placement.
2. It looks up recipes for the resource type on that environment.
3. If the Application specifies a recipe name (`recipe: cnpg-ha`), that named recipe is used.
4. If no recipe name is specified, the `default` recipe is used.
5. If no recipe is registered for the resource type on the environment, provisioning fails.

Applications can optionally select a named recipe:

```yaml
spec:
  resources:
    - type: database.postgresql
      name: my-db
      recipe: cnpg-ha        # optional — uses named recipe instead of default
      properties:
        size: L
```

### 5.6 Parameter Precedence

Parameters are merged in this order (later wins):

1. Recipe `parameters` (operator defaults)
2. Application `properties` (developer values)

This allows operators to set environment-specific defaults (e.g., `storageClass`, `backup_retention_period`) while letting developers override resource-type-specific properties (e.g., `size`, `storageGB`).

---

## 6. Execution Flow

When the Execution Engine provisions a resource, it orchestrates the full flow from Application to Recipe:

```
1. Resource declares type: database.postgresql
2. Placement selects environment: prod-eu-k8s-01 (type: kubernetes)
3. Engine resolves recipe for (database.postgresql, prod-eu-k8s-01)
4. Engine builds invocation:
     a. Passes developer properties as variables (same names as schema)
     b. Merges operator parameters (from recipe registration)
5. Engine invokes Recipe (Terraform apply / Ansible playbook / Helm install)
6. Recipe returns result object with values + secrets
7. Engine validates result against resource type readOnly properties
8. Outputs become available for CEL resolution in dependent resources
```

```
Application               ResourceType                    Recipe
                        (database.postgresql)    (terraform-aws on prod-eu)
                        +------------------+     +------------------------+
properties:             |  schema:         |     |                        |
  size: "M"  ---------->|    size: string  |---->|  var.size = "M"        |
  storageGB: 100        |    storageGB: int|     |  var.storageGB = 100   |
                        |                  |     |  var.context = {...}   |
                        |    host:         |     |                        |
CEL: ${db.host}  <------|      readOnly ◄--|<----|  result.values.host    |
CEL: ${db.password} <---|    password:     |     |  result.secrets.pass.. |
                        |      readOnly    |     |                        |
                        |      sensitive   |     |                        |
                        +------------------+     +------------------------+
```
---

## 7. Integration with Other DCM Resources

### 7.1 Application ([appspec.md](appspec.md))

Developers reference resource types by `metadata.name` in their Application `spec.resources[].type` field. The Application controller validates:

- The resource type exists and is `stable` (or `deprecated` with warning).
- All `properties` match non-`readOnly` properties in the resource type schema (types, constraints, required fields).
- All CEL cross-resource references (e.g., `${db.host}`) match a `readOnly` property in the referenced resource type's schema.

### 7.2 Environment ([environment.md](environment.md))

Infrastructure Operators list supported resource type names in `spec.capabilities.resourceTypes` and register Recipes in `spec.recipes`. The placement pre-filter checks that the environment's resource types are a superset of the Application's required types.

### 7.3 PlacementPolicy ([placement.md](placement.md))

Placement policies can match on resource types via `spec.match.resourceTypes`. This allows different placement rules for different resource types (e.g., databases in EU-only environments, compute in any region).

### 7.4 Execution Engine ([architecture.md](architecture.md))

The Execution Engine consumes ResourceTypes and Recipes at runtime:

1. Resolve the ResourceType and version for each resource in the Application.
2. After placement, resolve the Recipe for the (resourceType, environment) pair.
3. Inject context and pass developer properties as variables to the Recipe.
4. Invoke the Recipe.
5. Validate the `result` object against the resource type's `readOnly` properties.
6. Make outputs available for CEL resolution in the dependency DAG.
