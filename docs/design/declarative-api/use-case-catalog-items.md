# Use case: Catalog items (platform templates) — declarative API


**Personas:** 
- Platform engineer authors and publishes `CatalogItems` (curated templates stored in the DCM catalog database). 
- Dev user creates an `Application` that references a catalog item and supplies only allowed parameters.

This document explains the recommended declarative shape, tradeoffs, and a full example.


## Recommended declarative pattern


| Layer           | Owner             | What is stored                                                                                                                                    | Purpose                                                         |
| --------------- | ----------------- |---------------------------------------------------------------------------------------------------------------------------------------------------| --------------------------------------------------------------- |
| **CatalogItem** | Platform engineer | Stable id, version, `**spec.resources`** (same shape as a full **Application** graph: CEL, `requirements`, etc.), `spec.paramSchema`, policy tags | Single source of curated topology and guardrails                |
| **Application** | Dev user          | `metadata`, `spec.catalogItemRef` (or `spec.fromCatalog`) + `spec.params` only (optional small overrides if policy allows)                        | Intent: “instantiate *this* approved stack with *these* values” |


**CatalogItem vs Application** 
A **CatalogItem** uses the same `spec` layout as an `Application`: `paramSchema` defines the parameters 
tenants may supply, and `resources` holds the multi-resource graph. The engine resolves `fromCatalog` by copying 
that `resources` block into `Application.spec.resources` after `params` are applied, 
then continues with the normal pipeline (no separate nested graph object).

**Compile-time flow**

1. Resolve `fromCatalog` + `version` against the catalog DB (must exist and be published and not deprecated).
2. Load CatalogItem `spec.resources` (the frozen graph template).
3. Merge dev `Application.spec.params` into that graph (validate values against **CatalogItem
  `spec.paramSchema` only — devs do not declare arbitrary resource types in the strict model).
4. Run the same **CEL/DAG/plan** pipeline as for any full `Application` on the **materialized** 
  `spec.resources` (the engine may persist the **effective** graph for audit).

**Why not embed full `resources` in the dev’s YAML?** In the strict catalog model there is no need to
because hiding the graph reduces misuse and keeps review small.


## Advantages

- **Guardrails:** Only approved topologies, regions, etc. are shipped to production.
- **Faster validation:** Param schema is small; resource types and edges are **platform-owned**, so compile errors are rare compared to freeform graphs.
- **Operational consistency:** Naming, tagging, backup tier, and monitoring hooks can be **baked into** CatalogItem `spec.resources`.
- **Clear ownership:** Platform evolves **versions** of a CatalogItem; devs pin or float versions explicitly.


## Disadvantages

- **Flexibility bottleneck:** Every new architecture waits on a **new or updated** CatalogItem (unless patches / escape hatches are added).
- **Catalog lifecycle cost:** Versioning, deprecation, migration guides, compatibility testing across environments.
- **Possible mismatch:** Dev teams may push for “just one more resource” until the catalog explodes with variants (`payments-api-v47`).


## When to prefer this model

- Strong platform engineering and standardized stacks (e.g. “three-tier web”, “API + Postgres + bucket”).
- Compliance, cost caps, and predictable blast radius per tenant.
- Dev users who should **not** choose low-level resource graphs.


## Example: what the platform engineer defines (conceptual)

Stored in the **catalog database** (and often edited via a platform API or GitOps repo that syncs into DB). Not pasted by dev users.

```yaml
# CatalogItem (logical shape — exact API is product-specific)
apiVersion: dcm.io/v1alpha1
kind: CatalogItem
metadata:
  name: payments-api-stack
spec:
  version: "2.1.0"
  status: published
  description: Payments API with Postgres, object storage, VPC, DNS.
  paramSchema:
    appId:
      type: string
      required: true
    image:
      type: string
      required: true
    dnsName:
      type: string
      required: true

  # Mirrors Application.spec.resources (resource list + CEL + optional requirements)
  resources:
    - name: vpc
      type: network.virtual-network
      properties:
        name: "${params.appId}-payments-net"
        region: eu-central-1
        cidr: 10.40.0.0/16
    - name: storage
      type: storage.object-bucket
      properties:
        name: "${params.appId}-payments-artifacts"
        versioning: true
    - name: db
      type: database.postgresql
      properties:
        dbName: "${params.appId}-payments"
        tier: 1
        subnetIds: "${vpc.privateSubnetIds}"
    - name: api
      type: workloads.stateless-service
      properties:
        image: "${params.image}"
        subnetIds: "${vpc.publicSubnetIds}"
        env:
          DATABASE_URL: "${db.connectionString}"
          STORAGE_BUCKET: "${storage.name}"
        ports:
          - name: https
            port: 443
            targetPort: 8080
            public: true
    - name: api-dns
      type: dns.record-set
      properties:
        zone: example.com
        name: "${params.dnsName}"
        type: CNAME
        ttl: 300
        target: "${api.publicHostname}"
```


## Example: what the dev user submits (Application)

Only **reference + params**. The engine **materializes** **CatalogItem `spec.resources`** (plus bound `params`) into an effective **Application**-shaped graph before plan/apply.

```yaml
apiVersion: dcm.io/v1alpha1
kind: Application
metadata:
  name: acme-payments
  labels:
    dcm.io/environment: staging-eu
spec:
  fromCatalog:
    name: payments-api-stack
    version: "2.1.0"
  params:
    appId: acme
    image: quay.io/acme/payments-api:v1.9.0
    dnsName: payments-acme.staging
```

After resolution, the **effective** resource set matches the **payments** topology (VPC, storage, Postgres, stateless service, DNS), with cloud resource names incorporating `**appId`** and the other `**params`** values the dev supplied.


## Relation to the full-graph `payment-api.md` example

The **same DAG** as in `payment-api.md` applies **after** materialization from **CatalogItem `spec.resources`**. Dev users in this model **do not author** that graph file-by-file unless policy allows extensions.

See also: `use-case-freeform-application.md` (freeform path).