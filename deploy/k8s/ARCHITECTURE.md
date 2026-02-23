# K8s Architecture

## Overview

A **3-tier web application** deployed across 3 Kubernetes namespaces on host `learn-k8s-kayn.work.gd`.

- **Frontend:** React app served via Nginx
- **APIs:** Go (port 8000) and Node.js (port 3000), both query PostgreSQL
- **Database:** PostgreSQL 15.3 StatefulSet with persistent storage
- **Ingress:** Traefik with path-based routing and prefix stripping
- **Load Testing:** Python generator hitting the Go API every 1s

## Architecture Diagram

```mermaid
graph TB
    %% External
    User["👤 User / Browser"]

    subgraph K8s Cluster

        subgraph ns-traefik["Namespace: traefik"]
            Traefik["Traefik Ingress Controller<br/>(Helm chart v20.8.0)<br/>EntryPoint: web"]
            Middleware["Middleware: stripprefix<br/>strips /api/golang, /api/node"]
        end

        subgraph ns-demo["Namespace: demo-app"]

            subgraph frontend["Frontend Tier"]
                ReactDeploy["Deployment: client-react<br/>1 replica | nginx:8080<br/>CPU: 250m-500m | Mem: 64-128Mi"]
                ReactSvc["Service: client-react-service<br/>ClusterIP :8080"]
                ReactCM["ConfigMap: nginx-config<br/>nginx.conf with /ping + SPA routing"]
                ReactIngress["IngressRoute<br/>Host(learn-k8s-kayn.work.gd)<br/>→ / (root path)"]
            end

            subgraph api-tier["API Tier"]
                GolangDeploy["Deployment: api-golang<br/>1 replica | :8000<br/>CPU: 250m-500m | Mem: 100-200Mi<br/>readinessProbe: /ping"]
                GolangSvc["Service: api-golang-service<br/>ClusterIP :8000"]
                GolangSecret["Secret: api-golang-secret<br/>DATABASE_URL"]
                GolangIngress["IngressRoute<br/>PathPrefix(/api/golang)<br/>+ stripprefix middleware"]

                NodeDeploy["Deployment: api-node<br/>1 replica | :3000<br/>CPU: 500m | Mem: 128-256Mi<br/>readinessProbe: /ping"]
                NodeSvc["Service: api-node-service<br/>ClusterIP :3000"]
                NodeSecret["Secret: api-node-secret<br/>DATABASE_URL"]
                NodeIngress["IngressRoute<br/>PathPrefix(/api/node)<br/>+ stripprefix middleware"]
            end

            subgraph loadgen["Load Generator"]
                LoadGenDeploy["Deployment: load-generator-python<br/>1 replica | DELAY_MS=1000<br/>CPU: 250m-500m | Mem: 64-128Mi"]
            end

            subgraph migration["DB Migration"]
                MigratorJob["Job: db-migrator<br/>runs SQL migrations"]
                MigratorSecret["Secret: db-migrator-secret<br/>DATABASE_URL"]
            end

        end

        subgraph ns-postgres["Namespace: postgres"]
            PgSS["StatefulSet: postgres-postgresql<br/>1 replica | postgres:15.3-alpine<br/>port: 5432"]
            PgSvc["Headless Service<br/>postgres-postgresql :5432"]
            PgPVC["PVC: 1Gi<br/>ReadWriteOnce"]
        end

    end

    %% Traffic flow
    User -->|"HTTP :80"| Traefik
    Traefik -->|"/"| ReactIngress
    Traefik -->|"/api/golang"| GolangIngress
    Traefik -->|"/api/node"| NodeIngress

    ReactIngress --> ReactSvc --> ReactDeploy
    ReactCM -.->|mount nginx conf| ReactDeploy

    GolangIngress -->|stripprefix| Middleware
    Middleware --> GolangSvc --> GolangDeploy
    NodeIngress -->|stripprefix| Middleware
    Middleware --> NodeSvc --> NodeDeploy

    GolangSecret -.->|envFrom| GolangDeploy
    NodeSecret -.->|envFrom| NodeDeploy

    %% DB connections
    GolangDeploy -->|"SELECT NOW()"| PgSvc
    NodeDeploy -->|"SELECT NOW()"| PgSvc
    PgSvc --> PgSS
    PgPVC -.->|persistent storage| PgSS

    %% Load generator
    LoadGenDeploy -->|"HTTP every 1s"| GolangSvc

    %% Migration
    MigratorSecret -.->|envFrom| MigratorJob
    MigratorJob -->|"migrate up"| PgSvc

    %% Styles
    classDef ns fill:#1a1a2e,stroke:#16213e,color:#e0e0e0
    classDef deploy fill:#0f3460,stroke:#533483,color:#e0e0e0
    classDef svc fill:#533483,stroke:#e94560,color:#e0e0e0
    classDef ingress fill:#e94560,stroke:#fff,color:#fff
    classDef secret fill:#2d4059,stroke:#ea5455,color:#e0e0e0
    classDef storage fill:#3c6382,stroke:#82ccdd,color:#e0e0e0
```

## Component Summary

| Component | Namespace | Kind | Port | Image |
|---|---|---|---|---|
| Traefik | `traefik` | Helm release | 80 | traefik/traefik v20.8.0 |
| client-react | `demo-app` | Deployment (1r) | 8080 | client-react-nginx:0.1.0 |
| api-golang | `demo-app` | Deployment (1r) | 8000 | api-golang:0.1.0 |
| api-node | `demo-app` | Deployment (1r) | 3000 | api-node:0.1.0 |
| load-generator | `demo-app` | Deployment (1r) | - | load-generator-python:0.1.0 |
| db-migrator | `demo-app` | Job | - | db-migrator:0.1.0 |
| PostgreSQL | `postgres` | StatefulSet (1r) | 5432 | postgres:15.3-alpine |

## Routing Rules

| Path | Target Service | Middleware |
|---|---|---|
| `/` | client-react-service:8080 | none |
| `/api/golang` | api-golang-service:8000 | stripprefix |
| `/api/node` | api-node-service:3000 | stripprefix |

## Cross-Namespace Communication

APIs and the db-migrator in `demo-app` connect to PostgreSQL in `postgres` namespace via FQDN:

```
postgres-postgresql-0.postgres-postgresql.postgres.svc.cluster.local:5432
```
