# DCM Multi-Environment Placement & Cross-Environment Networking Design Document

**Status:** Draft
**Date:** 2026-04-27
**Authors:** DCM Team

---

## 1. Overview

By default, all resources in an Application are placed in a single environment (see [placement.md](placement.md), Section 6.1). However, some applications require resources in different environments -- for example, a database in the EU for GDPR compliance and compute close to users in the US.

This document designs **per-resource placement** and the **cross-environment networking** model that ensures applications continue to work when their resources span multiple environments.

### 1.1 Design Principles

1. **Connectivity-aware.** The placement engine understands which environments can reach each other and rejects placements that would break networking.

2. **Fail-safe.** If connectivity between two environments cannot be proven, placement is rejected. The system never assumes environments can communicate.

3. **Overlay-agnostic.** The connectivity model supports any overlay technology (Submariner, Skupper, Cilium ClusterMesh) without coupling to a specific implementation.

4. **DAG-driven validation.** Only resources that actually reference each other (via CEL expressions or explicit requirements) need connectivity between their environments. Independent resources can be in disconnected environments.

---

## 2. Connectivity Model

Environments declare membership in **overlay networks**. The control plane builds a **connectivity graph** from these declarations.

### 2.1 Overlays

An environment declares membership in named overlay networks. Two environments sharing an overlay can reach each other at the application layer.

Overlays are declared in the Environment spec under `networking.overlays`:

```yaml
networking:
  overlays:
    - name: eu-mesh
      type: submariner
    - name: global-service-mesh
      type: skupper
```

#### Overlay Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | Yes | string | Name of the overlay network. Environments sharing the same name are connected. |
| `type` | Yes | string | Overlay technology: `submariner`, `skupper`, `cilium-clustermesh`, `istio-multicluster`. |
| `latency` | No | string | Measured or expected latency across this overlay (e.g., `5ms`, `80ms`). |
| `bandwidth` | No | string | Available bandwidth (e.g., `10Gbps`, `1Gbps`). |

#### How It Works

```
Environment A                    Environment B
networking:                      networking:
  overlays:                        overlays:
    - name: eu-mesh    ======>       - name: eu-mesh
      type: submariner               type: submariner

A and B share overlay "eu-mesh" => A <-> B are connected
```

### 2.2 Connectivity Graph

The control plane builds and maintains a connectivity graph from all overlay memberships. Two environments are **connected** if they share at least one overlay (same overlay `name`).

```
Connectivity Graph
==================

  prod-eu-k8s-01 ----[eu-mesh/submariner]---- prod-eu-k8s-02
                                                    |
                                              [eu-mesh/submariner]
                                                    |
  prod-us-k8s-01 ----[us-mesh/submariner]---- prod-us-k8s-02
```

The graph is queried during placement to validate that cross-environment resource references have connectivity.

---

## 3. Per-Resource Placement

### 3.1 Placement Hints

Individual resources in an Application can declare placement hints via a `labels` field. These hints are matched against PlacementPolicies (see [placement.md](placement.md)) to select a specific environment for that resource.

```yaml
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: global-app
  labels:
    team: platform
spec:
  resources:
    - name: db
      type: database.postgresql
      labels:
        compliance: gdpr
        region: eu
      properties:
        size: L

    - name: api
      type: compute.container
      labels:
        region: us
      properties:
        image: quay.io/app/api
        dbUrl: "${db.connectionString}"

    - name: cache
      type: cache.redis
      properties:
        memoryGB: 4
```

### 3.2 Placement Hint Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `placement.labels` | No | map&lt;string, string&gt; | Labels added to the resource's effective label set for policy matching. Merged with Application-level labels. |

### 3.3 Resolution Rules

Every resource is placed independently through the placement pipeline:

1. **Resources with `placement.labels`**: the resource's effective labels are the Application labels merged with the placement labels (placement labels take precedence). Policies are matched and evaluated against the merged label set.
2. **Resources without `placement`**: use the Application-level labels only. The placement pipeline still runs for the resource -- it is **not** automatically assigned to the same environment as other resources. The engine selects the best eligible environment based on matching policies and the resource's type.

In all cases, the target environment must support the resource's type in `capabilities.resourceTypes`. A resource is never placed in an environment that cannot provision it.

---

## 4. Connectivity Validation

When resources in an Application are placed in different environments, the placement engine validates that all cross-environment dependencies have connectivity.

### 4.1 Validation Algorithm

1. Build the Application's dependency DAG (from CEL references and explicit `requirements`).
2. For each edge in the DAG (resource A depends on resource B):
   - Resolve the environment for A and the environment for B.
   - If they are in the **same** environment: pass (no cross-env networking needed).
   - If they are in **different** environments: query the connectivity graph for a path between the two environments.
3. If any dependency edge lacks connectivity: **placement fails**.

```
DAG:  api -> db       (api references ${db.connectionString})
      api -> cache    (api references ${cache.host})

Placement (each resource placed independently):
  db    -> prod-eu-k8s-01  (placement labels: compliance=gdpr, region=eu)
  api   -> prod-us-k8s-01  (placement labels: region=us)
  cache -> prod-us-k8s-01  (no placement hint, but cache.redis supported here
                             and policies selected this env based on app labels)

Connectivity check:
  api(prod-us-k8s-01) -> db(prod-eu-k8s-01): need connectivity
    => shared overlay "global-mesh" exists => PASS
  api(prod-us-k8s-01) -> cache(prod-us-k8s-01): same environment => PASS

Result: placement succeeds
```

Note: `cache` was not inherited into `prod-us-k8s-01` -- it was independently evaluated through the placement pipeline. It landed in the same environment because policies and resource type compatibility led to that result. If `prod-us-k8s-01` did not support `cache.redis`, the engine would have selected a different eligible environment.

### 4.2 Error Reporting

When connectivity validation fails, the error identifies the specific resources and environments:

```
PlacementFailed: no connectivity between environments for dependent resources.

  Resource "api" (prod-us-k8s-01) depends on "db" (prod-asia-k8s-01)
    via: ${db.connectionString}
    environments prod-us-k8s-01 and prod-asia-k8s-01 share no overlay network.

  To resolve:
    - Add both environments to a shared overlay network
    - Or adjust placement hints so both resources land in the same environment
```

---