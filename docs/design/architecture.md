# DCM Architecture Overview

**Status:** Draft
**Date:** 2026-04-24
**Authors:** DCM Team

---

## High-Level Architecture

The diagram below shows how the three personas interact with the DCM Control Plane and how an Application flows from authoring to execution.

- **Developers** author Applications and commit them to Git, or create them directly via the self-service portal (WebApp).
- **Platform Engineers** interact with the Control Plane to define resource types and policies, and register Resource Providers that handle actual provisioning.
- **Infrastructure Operators** manage the Environments (datacenters, clusters, cloud regions) that the system provisions into.

The **Control Plane** is the core of DCM. The Git Controller watches repositories for Application changes and persists them to the Application DB. The Application Controller picks up new or updated Applications, builds a dependency DAG, and hands it to the Execution Engine. The Execution Engine fetches available Environments, resolves placement, and executes provisioning through the registered Resource Providers.

```mermaid
graph TD
    DEV((Developer))
    PE((Platfrom Engineer))
    INFRA((Infra Engineer))

    subgraph Application
        FE[WebApp]
    end

    subgraph Git
        repo[repo]
    end

    ENV[Environments]
    RP[Resource Providers]

    %% Main components (the Control Plane box)
    subgraph CP[Control Plane]
        GCTRL[Git Controller]
        AP_DB[Application DB]
        AP_CTRL[Application controller]
        DAG[DAG]
        ENGINE[Execution Engine]
    end

    %% Define connections
    DEV -->|Creates| Application
    DEV -->|Commit| Git
    GCTRL -->|Watches| Git
    FE --> |Creates| AP_DB
    PE -->|Interacts with| CP
    PE -->|Creates| RP
    INFRA -->|Manages| ENV

    %% Internal Control Plane and downward flows
    GCTRL --> |Creates| AP_DB
    AP_CTRL -->  |Watches| AP_DB
    AP_CTRL --> |Creates| DAG
    ENGINE --> |Consume| DAG
    ENGINE --> |Fetches| ENV
    ENGINE --> |Executes| RP
```

---

## Entity Relationship Diagram

The diagram below shows all DCM resource types (entities), how they reference each other, and their role in the provisioning pipeline.

```mermaid
erDiagram
    Application ||--o{ ResourceDecl : "spec.resources"
    ResourceDecl }o--|| ResourceType : "type references"
    ResourceDecl }o--o| Recipe : "recipe selects"
    ResourceDecl }o--o{ ResourceDecl : "requirements / CEL refs"

    ResourceType ||--o{ PropertySchema : "spec.schema.properties"

    Environment ||--o{ RecipeBinding : "spec.recipes"
    RecipeBinding }o--|| ResourceType : "bound to"
    RecipeBinding }o--o| Recipe : "implements"

    PlacementPolicy }o--o{ Application : "spec.match selects"
    PlacementPolicy }o--o{ Environment : "rule/prefer evaluates"

    Deployment }o--|| Application : "spec.application"
    Deployment ||--o{ DeploymentResource : "spec.resources"
    DeploymentResource }o--|| Environment : "placed in"

    Application {
        string name PK
        label labels
    }
    ResourceDecl {
        string name
        string type FK
        map properties
        list requirements
        string recipe
    }
    ResourceType {
        string name PK
        string version
        string lifecycle
        object schema
    }
    PropertySchema {
        string name
        string type
        bool readOnly
        bool sensitive
    }
    Environment {
        string name PK
        string type
        object connection
        object capabilities
        object sovereignty
        object capacity
        object networking
        object cost
        map recipes
    }
    RecipeBinding {
        string resourceType FK
        string recipeName
        string driverType
        map source
        map parameters
    }
    Recipe {
        string name PK
        string resourceType FK
        string type
        map source
        object environmentMatch
    }
    PlacementPolicy {
        string name PK
        object match
        string rule
        string prefer
        float weight
        int priority
    }
    Deployment {
        string name PK
        string application FK
        string phase
        string startedAt
        string finishedAt
        map assignments
    }
    DeploymentResource {
        string name
        string phase
        string environment FK
        map outputs
    }
```

---

## Provisioning Pipeline

This diagram shows the end-to-end flow when an Application is deployed — from submission through validation, DAG building, placement, and execution.

```mermaid
flowchart TB
    subgraph Input["Developer Input"]
        APP[Application YAML]
    end

    subgraph Validation["Validation Phase"]
        STRUCT[Structural Validation]
        SCHEMA[Schema Validation]
        RT_LOOKUP[Lookup ResourceTypes]
    end

    subgraph DAGPhase["DAG Phase"]
        CEL_PARSE[Parse CEL References]
        DAG_BUILD[Build Dependency Graph]
        TOPO[Topological Sort]
    end

    subgraph PlacementPhase["Placement Phase"]
        LOAD_ENV[Load Environments]
        LOAD_POL[Load Policies]
        PREFILTER[Pre-filter by ResourceType]
        MATCH[Match Policies to Resources]
        RULES[Evaluate CEL Rules]
        SCORE[Score CEL Preferences]
        SELECT[Select Best Environment]
        CONNECT[Validate Connectivity]
    end

    subgraph ExecutionPhase["Execution Phase"]
        RESOLVE[Resolve Recipe per Resource+Env]
        MERGE[Merge Parameters]
        CTX[Inject DCM Context]
        DRIVER[Invoke Driver]
        RESULT[Validate Result]
        OUTPUTS[Store Outputs]
    end

    subgraph Drivers["Recipe Drivers"]
        MOCK[Mock Driver]
        ANSIBLE[Ansible Driver]
        HELM[Helm Driver]
        TF[Terraform Driver]
    end

    subgraph Record["Record"]
        DEPLOY[Create Deployment Record]
        STATUS[Update Status]
    end

    APP --> STRUCT --> SCHEMA
    SCHEMA --> RT_LOOKUP
    RT_LOOKUP -.->|ResourceType| SCHEMA

    STRUCT --> CEL_PARSE --> DAG_BUILD --> TOPO

    TOPO --> LOAD_ENV & LOAD_POL
    LOAD_ENV --> PREFILTER --> MATCH
    LOAD_POL --> MATCH
    MATCH --> RULES --> SCORE --> SELECT --> CONNECT

    CONNECT --> RESOLVE --> MERGE --> CTX --> DRIVER
    DRIVER --> MOCK & ANSIBLE & HELM & TF
    DRIVER --> RESULT --> OUTPUTS
    OUTPUTS -.->|"CEL ${ref.field}"| MERGE

    RESULT --> DEPLOY --> STATUS

    style Input fill:#1a1d27,stroke:#6366f1,color:#e4e6eb
    style Validation fill:#1a1d27,stroke:#f59e0b,color:#e4e6eb
    style DAGPhase fill:#1a1d27,stroke:#3b82f6,color:#e4e6eb
    style PlacementPhase fill:#1a1d27,stroke:#22c55e,color:#e4e6eb
    style ExecutionPhase fill:#1a1d27,stroke:#ef4444,color:#e4e6eb
    style Drivers fill:#1a1d27,stroke:#9ca3af,color:#e4e6eb
    style Record fill:#1a1d27,stroke:#8b5cf6,color:#e4e6eb
```

---

## Data Flow Between Entities

This diagram shows how data flows between the core resource types during a deployment.

```mermaid
graph LR
    subgraph PlatformEngineer["Platform Engineer"]
        RT[ResourceType]
        PP[PlacementPolicy]
        RCP[Recipe]
    end

    subgraph InfraOperator["Infra Operator"]
        ENV[Environment]
    end

    subgraph Developer["Developer"]
        APP[Application]
    end

    subgraph ControlPlane["Control Plane"]
        VAL{Validate}
        DAG{Build DAG}
        PLACE{Place}
        EXEC{Execute}
    end

    subgraph Output["Output"]
        DEP[Deployment]
    end

    RT -->|"schema validates"| VAL
    RT -->|"outputs define CEL refs"| DAG
    APP -->|"resources[]"| VAL
    APP -->|"resources[]"| DAG

    PP -->|"match + rule + prefer"| PLACE
    ENV -->|"capabilities + sovereignty"| PLACE
    DAG -->|"levels[]"| PLACE

    ENV -->|"recipes[type]"| EXEC
    RCP -->|"driver + source"| EXEC
    PLACE -->|"assignments"| EXEC

    EXEC -->|"phase + outputs"| DEP
    DEP -->|"tracks"| APP

    style PlatformEngineer fill:#1e293b,stroke:#6366f1,color:#e4e6eb
    style InfraOperator fill:#1e293b,stroke:#22c55e,color:#e4e6eb
    style Developer fill:#1e293b,stroke:#3b82f6,color:#e4e6eb
    style ControlPlane fill:#1e293b,stroke:#f59e0b,color:#e4e6eb
    style Output fill:#1e293b,stroke:#8b5cf6,color:#e4e6eb
```
