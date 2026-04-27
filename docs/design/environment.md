# DCM Environment Design Document

**Status:** Draft
**Date:** 2026-04-27
**Authors:** DCM Team

---

## 1. Overview

The Environment is the Infrastructure Operator's primary artifact. It represents a target infrastructure -- a Kubernetes cluster, a VMware vCenter, an AWS account/region, or any other platform -- where the Execution Engine provisions resources. Environments are registered by Infrastructure Operators and consumed by the Placement Engine at runtime.

### 1.1 Design Principles

1. **Declare what is available, not what to deploy.** Environments describe capacity, capabilities, and constraints. They do not contain workload definitions.

2. **Static plus dynamic.** Static capabilities (supported resource types, features) are declared at registration time. Dynamic capacity (CPU, memory, GPU) is pushed by agents at runtime.

3. **Sovereignty-aware.** Every environment carries jurisdiction, compliance, and data classification metadata so that the placement engine can enforce regulatory constraints.

4. **Cost-aware.** Every environment declares cost rates so the placement engine can optimize for cost alongside capacity and compliance.

5. **CEL-queryable.** All environment properties are exposed as a structured CEL object so placement policies can use arbitrary expressions.

---

## 2. Top-Level Structure

```yaml
apiVersion: dcm.io/v1alpha1
kind: Environment
metadata:
  name: prod-eu-k8s-01
  labels:
    tier: production
    team: platform
spec:
  type: kubernetes
  description: "Production Kubernetes cluster in Frankfurt"

  connection:
    endpoint: "https://k8s-prod-eu.example.com:6443"
    credentialRef: "vault:secret/dcm/envs/prod-eu-k8s-01"

  capabilities:
    resourceTypes:
      - compute.container
      - compute.service
      - network.ingress
    features:
      - gpu
      - ssd-storage

  capacity:
    cpu:
      total: 2000
      unit: cores
    memory:
      total: 8192
      unit: GiB
    storage:
      total: 100000
      unit: GiB
    gpu:
      total: 64
      unit: devices
    custom: {}

  sovereignty:
    country: DE
    region: eu-central-1
    zone: eu-central-1a
    jurisdiction: EU
    compliance:
      - GDPR
      - SOC2
    dataClassification: confidential

  networking:
    features:
      - public-ip
      - ipv6
      - load-balancer
    zones:
      - name: dmz
        subnets:
          - name: dmz-primary
            cidr: "10.0.1.0/24"
            gateway: "10.0.1.1"
            availableIPs: 200
            vlan: 100
      - name: internal
        subnets:
          - name: internal-primary
            cidr: "10.0.10.0/22"
            gateway: "10.0.10.1"
            availableIPs: 900
            vlan: 200

  cost:
    currency: EUR
    rates:
      cpu:
        unit: core/hour
        value: 0.035
      memory:
        unit: GiB/hour
        value: 0.004
      storage:
        unit: GiB/month
        value: 0.08
      gpu:
        unit: device/hour
        value: 2.50
    custom: {}
```

### 2.1 Field Reference

| Field | Required | Description |
|-------|----------|-------------|
| `apiVersion` | Yes | API version. Currently `dcm.io/v1alpha1`. |
| `kind` | Yes | Always `Environment`. |
| `metadata.name` | Yes | Unique name for this environment. |
| `metadata.labels` | No | Key-value pairs for filtering and selection (e.g., `tier`, `team`). |
| `spec.type` | Yes | Environment type: `kubernetes`, `openshift`, `vmware`, `aws`, `azure`, `gcp`, `bare-metal`. |
| `spec.description` | No | Human-readable description. |
| `spec.connection` | Yes | Connection details for the environment. |
| `spec.connection.endpoint` | Yes | API endpoint URL. |
| `spec.connection.credentialRef` | Yes | Reference to stored credentials (e.g., Vault path). |
| `spec.capabilities` | Yes | Capabilities declared by operator. |
| `spec.capacity` | No | Declared total capacity (updated dynamically by agent). |
| `spec.sovereignty` | Yes | Jurisdiction and compliance metadata. |
| `spec.cost` | No | Cost rates for resource consumption in this environment. |
| `spec.networking` | No | Networking capabilities and topology. |

---

## 3. Environment Types and Static Capabilities

### 3.1 Resource Types

The `capabilities.resourceTypes` list declares which resource types this environment can provision. When the Execution Engine evaluates placement, an environment is only a candidate if its `resourceTypes` is a superset of the resource types required by the Application.

```yaml
capabilities:
  resourceTypes:
    - compute.container
    - compute.service
    - network.ingress
    - database.postgresql
```

### 3.3 Features

The `capabilities.features` list is freeform. Operators tag environments with features that placement policies can reference.

```yaml
capabilities:
  features:
    - gpu
    - arm64
    - ssd-storage
    - high-memory
```

---

## 4. Dynamic Capacity Reporting

### 4.1 Agent Architecture

A lightweight agent (`dcm-agent`) runs in each environment and periodically pushes capacity metrics to the control plane. Push-based design is used because environments may be behind firewalls or NAT, making pull-based scraping impractical.

```
+-------------------+          POST /capacity          +------------------+
|   Environment     |  -------------------------------->  Control Plane   |
|                   |                                   |                  |
|   dcm-agent       |   interval: spec.agent            |   Environment    |
|   (push metrics)  |   .reportInterval                 |   Status Store   |
+-------------------+                                   +------------------+
```

### 4.2 Capacity Data Model

The agent reports current resource state. The control plane stores this in the `status.capacity` block (see [Section 9](#9-status-and-health-reporting)).

Each resource dimension follows the same structure:

| Field | Type | Description |
|-------|------|-------------|
| `total` | int | Total capacity in the environment. |
| `allocated` | int | Currently allocated/reserved capacity. |
| `available` | int | Remaining capacity (`total - allocated`). |
| `unit` | string | Unit of measurement (e.g., `cores`, `GiB`, `devices`). |

Standard dimensions: `cpu`, `memory`, `storage`, `gpu`.

```yaml
capacity:
  cpu:
    total: 2000
    unit: cores
  memory:
    total: 8192
    unit: GiB
  storage:
    total: 100000
    unit: GiB
  gpu:
    total: 64
    unit: devices
```

### 4.3 Freshness and Staleness

The control plane compares `status.lastReported` against the `spec.agent.staleness` threshold. If the time since the last report exceeds the threshold, `status.staleness` is set to `stale`.

| Staleness | Condition |
|-----------|-----------|
| `fresh` | `now - lastReported < spec.agent.staleness` |
| `stale` | `now - lastReported >= spec.agent.staleness` |
| `unknown` | No report has ever been received. |

Placement policies should check staleness to avoid scheduling to environments with outdated data:

```cel
env.status.staleness == "fresh"
```

---

## 5. Sovereignty Model

Every environment must declare its sovereignty metadata. The placement engine uses this to enforce regulatory constraints such as data residency, compliance requirements, and classification levels.

### 5.1 Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `country` | Yes | string (ISO 3166-1 alpha-2) | Country where the environment physically resides. |
| `region` | Yes | string | Cloud region or datacenter region identifier. |
| `zone` | No | string | Availability zone within the region. |
| `jurisdiction` | Yes | string | Legal jurisdiction: `EU`, `US`, `CN`, `UK`, `JP`, etc. |
| `compliance` | No | list of strings | Certifications: `GDPR`, `FedRAMP`, `SOC2`, `HIPAA`, `PCI-DSS`, `ISO27001`. |
| `dataClassification` | Yes | string enum | Maximum data sensitivity level: `public`, `internal`, `confidential`, `restricted`. |

### 5.2 CEL Usage Examples

```cel
# Must be in Germany
env.sovereignty.country == "DE"

# Must be HIPAA certified
env.sovereignty.compliance.exists(c, c == "HIPAA")

# EU jurisdiction, approved for confidential data
env.sovereignty.jurisdiction == "EU"
  && env.sovereignty.dataClassification in ["confidential", "restricted"]

# FedRAMP + US only
env.sovereignty.country == "US"
  && env.sovereignty.compliance.exists(c, c == "FedRAMP")
```

---

## 6. Networking Model

### 6.1 Abstract Capabilities

The `networking.features` list declares what networking features the environment supports at a high level.

| Feature | Description |
|---------|-------------|
| `public-ip` | Can assign public/external IP addresses. |
| `ipv6` | Supports IPv6 addressing. |
| `load-balancer` | Can provision L4/L7 load balancers. |
| `service-mesh` | Has a service mesh (e.g., Istio, Linkerd) available. |
| `network-policy` | Supports network policy enforcement. |
| `dns` | Can manage DNS records. |

### 6.2 Network Zones

Network zones group subnets by security boundary. Common zones: `dmz`, `internal`, `management`.

```yaml
networking:
  zones:
    - name: dmz
      subnets:
        - name: dmz-primary
          cidr: "10.0.1.0/24"
          gateway: "10.0.1.1"
          availableIPs: 200
          vlan: 100
    - name: internal
      subnets:
        - name: app-subnet
          cidr: "10.0.10.0/22"
          gateway: "10.0.10.1"
          availableIPs: 900
          vlan: 200
        - name: db-subnet
          cidr: "10.0.20.0/24"
          gateway: "10.0.20.1"
          availableIPs: 250
          vlan: 201
```

### 6.3 Subnet Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | Yes | string | Unique subnet name within the environment. |
| `cidr` | Yes | string | CIDR notation (e.g., `10.0.1.0/24`). |
| `gateway` | No | string | Default gateway IP address. |
| `availableIPs` | No | int | Number of available IPs (dynamic, updated by agent). |
| `vlan` | No | int | VLAN ID. |

### 6.4 CEL Usage Examples

```cel
# Environment must support IPv6
env.networking.features.exists(f, f == "ipv6")

# Must have a DMZ zone with available IPs
env.networking.zones.exists(z,
  z.name == "dmz" && z.subnets.exists(s, s.availableIPs > 10))

# Must support public IPs and load balancers
env.networking.features.exists(f, f == "public-ip")
  && env.networking.features.exists(f, f == "load-balancer")
```

---

## 7. Cost Model

Every environment can declare cost rates so that the placement engine can factor cost into scheduling decisions. This enables Platform Engineers to write policies that balance cost against capacity, compliance, and proximity.

### 7.1 Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `currency` | Yes (if `cost` is set) | string (ISO 4217) | Currency code: `USD`, `EUR`, `GBP`, `JPY`, etc. |
| `rates.cpu` | No | CostRate | Cost per CPU core per time unit. |
| `rates.memory` | No | CostRate | Cost per GiB of memory per time unit. |
| `rates.storage` | No | CostRate | Cost per GiB of storage per time unit. |
| `rates.gpu` | No | CostRate | Cost per GPU device per time unit. |
| `custom` | No | map&lt;string, CostRate&gt; | Custom cost dimensions (e.g., `fpga`, `network-egress`). |

Each `CostRate` has:

| Field | Type | Description |
|-------|------|-------------|
| `unit` | string | Billing unit (e.g., `core/hour`, `GiB/month`, `device/hour`). |
| `value` | float | Cost per unit in the declared currency. |

### 7.2 Example

```yaml
cost:
  currency: USD
  rates:
    cpu:
      unit: core/hour
      value: 0.048
    memory:
      unit: GiB/hour
      value: 0.006
    storage:
      unit: GiB/month
      value: 0.10
    gpu:
      unit: device/hour
      value: 3.06
  custom:
    network-egress:
      unit: GiB
      value: 0.09
```

### 7.3 CEL Usage Examples

```cel
# Prefer cheapest CPU
env.cost.rates.cpu.value

# Only environments under $0.05/core/hour
env.cost.rates.cpu.value < 0.05

# Exclude environments with GPU cost above threshold
env.cost.rates.gpu.value <= 3.00

# Composite cost score (lower is better -- use negative in prefer)
-(env.cost.rates.cpu.value * 100 + env.cost.rates.memory.value * 256)
```

---

## 8. Glossary

| Term | Definition |
|------|------------|
| **Environment** | A target infrastructure registered by an Infrastructure Operator where resources can be provisioned. |
| **dcm-agent** | A lightweight agent running in each environment that pushes capacity metrics to the control plane. |
| **Placement Policy** | A CEL-based rule written by Platform Engineers that determines which environments are eligible for a given workload. |
| **Sovereignty** | Metadata describing an environment's physical location, legal jurisdiction, and compliance certifications. |
| **Staleness** | An indicator of whether an environment's dynamic capacity data is current or outdated. |
| **Capacity** | The compute, memory, storage, and custom resources available in an environment, reported dynamically by the agent. |
| **Cost Rate** | A per-unit price for a resource dimension (e.g., CPU core/hour), used by placement policies to optimize for cost. |
| **Network Zone** | A logical grouping of subnets by security boundary (e.g., DMZ, internal, management). |
