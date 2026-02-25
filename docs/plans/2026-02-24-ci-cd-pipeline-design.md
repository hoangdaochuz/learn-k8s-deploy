# CI/CD Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a CI/CD pipeline using GitHub Actions (CI) and ArgoCD (CD) with GitOps for a multi-service Kubernetes application.

**Architecture:** GitHub Actions detects changed services via path filters, builds/pushes Docker images, then updates Kustomize image tags in-repo. ArgoCD watches the repo and auto-syncs staging (manual sync for production).

**Tech Stack:** GitHub Actions, ArgoCD, Kustomize, Docker Buildx, Docker Hub

---

## Context

- **Repo**: `hoangdaochuz/learn-k8s-deploy` on GitHub
- **Registry**: Docker Hub under `hkdarealest/` namespace
- **5 services**: api-golang, api-node, client-react, load-generator-python, db-migrator (postgresql)
- **Kustomize overlays**: base, staging, production at `deploy/k8s/kustomize/`
- **Current state**: Images hardcoded in Deployment YAML patches. No CI/CD exists.

---

### Task 1: Update Kustomize overlays to use `images` sections

Currently image tags are hardcoded in patch Deployment YAML files. Switch to Kustomize `images` sections so `kustomize edit set image` works from CI.

**Files:**
- Modify: `deploy/k8s/kustomize/staging/kustomization.yaml`
- Modify: `deploy/k8s/kustomize/production/kustomization.yaml`
- Modify: `deploy/k8s/kustomize/staging/api-golang/patches/Deployment.yaml` (remove image line)
- Modify: `deploy/k8s/kustomize/production/api-golang/patches/Deployment.yaml` (remove image line)

**Step 1: Add `images` section to staging overlay kustomization.yaml**

Replace `deploy/k8s/kustomize/staging/kustomization.yaml` with:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: demo-app-staging
resources:
  - api-golang
  - api-node
  - client-react
  - ../base/traefik-middleware
  - load-generator-python
images:
  - name: hkdarealest/devops-directive-docker-course-api-golang
    newTag: "0.2.0"
  - name: hkdarealest/devops-directive-docker-course-api-node
    newTag: "0.1.0"
  - name: hkdarealest/devops-directive-docker-course-client-react-nginx
    newTag: "0.1.0"
  - name: hkdarealest/devops-directive-kubernetes-course-load-generator-python
    newTag: "0.1.0"
```

**Step 2: Add `images` section to production overlay kustomization.yaml**

Replace `deploy/k8s/kustomize/production/kustomization.yaml` with:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: demo-app-production
resources:
  - api-golang
  - api-node
  - client-react
  - ../base/traefik-middleware
  - load-generator-python
images:
  - name: hkdarealest/devops-directive-docker-course-api-golang
    newTag: "0.3.0"
  - name: hkdarealest/devops-directive-docker-course-api-node
    newTag: "0.1.0"
  - name: hkdarealest/devops-directive-docker-course-client-react-nginx
    newTag: "0.1.0"
  - name: hkdarealest/devops-directive-kubernetes-course-load-generator-python
    newTag: "0.1.0"
```

**Step 3: Remove image override from staging api-golang patch**

Replace `deploy/k8s/kustomize/staging/api-golang/patches/Deployment.yaml` with (remove image line, keep replicas/resources):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-golang-deployment
  namespace: demo-app
spec:
  replicas: 2 #base-replicas
  template:
    spec:
      containers:
        - name: api-golang
          resources:
            requests:
              memory: "100Mi"
              cpu: "100m"
            limits:
              memory: "200Mi"
              cpu: "250m"
```

**Step 4: Remove image override from production api-golang patch**

Replace `deploy/k8s/kustomize/production/api-golang/patches/Deployment.yaml` with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-golang-deployment
  namespace: demo-app
spec:
  replicas: 3 #base-replicas
  template:
    spec:
      containers:
        - name: api-golang
          resources:
            requests:
              memory: "100Mi"
              cpu: "100m"
            limits:
              memory: "200Mi"
              cpu: "250m"
```

**Step 5: Verify kustomize builds correctly**

Run:
```bash
cd deploy/k8s && kustomize build ./kustomize/staging | grep "image:"
cd deploy/k8s && kustomize build ./kustomize/production | grep "image:"
```

Expected: Images should show the correct tags from the `images` section.

**Step 6: Commit**

```bash
git add deploy/k8s/kustomize/
git commit -m "refactor: use Kustomize images section for image tag management"
```

---

### Task 2: Create the reusable build-service workflow

**Files:**
- Create: `.github/workflows/build-service.yaml`

**Step 1: Create the workflow file**

```yaml
name: Build Service

on:
  workflow_call:
    inputs:
      service-name:
        required: true
        type: string
      dockerfile-path:
        required: true
        type: string
      image-name:
        required: true
        type: string
      context-path:
        required: true
        type: string
      push:
        required: true
        type: boolean
    secrets:
      DOCKERHUB_USERNAME:
        required: false
      DOCKERHUB_TOKEN:
        required: false

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to Docker Hub
        if: ${{ inputs.push }}
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      - name: Generate image tag
        id: tag
        run: echo "sha_short=$(echo ${{ github.sha }} | cut -c1-7)" >> $GITHUB_OUTPUT

      - name: Build and push
        uses: docker/build-push-action@v6
        with:
          context: ${{ inputs.context-path }}
          file: ${{ inputs.dockerfile-path }}
          push: ${{ inputs.push }}
          tags: |
            ${{ inputs.image-name }}:${{ steps.tag.outputs.sha_short }}
            ${{ inputs.image-name }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**Step 2: Commit**

```bash
git add .github/workflows/build-service.yaml
git commit -m "ci: add reusable build-service workflow"
```

---

### Task 3: Create the reusable update-kustomize workflow

**Files:**
- Create: `.github/workflows/update-kustomize.yaml`

**Step 1: Create the workflow file**

```yaml
name: Update Kustomize Image Tags

on:
  workflow_call:
    inputs:
      image-name:
        required: true
        type: string
      image-tag:
        required: true
        type: string

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          token: ${{ secrets.GITHUB_TOKEN }}

      - name: Setup Kustomize
        uses: imranismail/setup-kustomize@v2

      - name: Update staging image tag
        run: |
          cd deploy/k8s/kustomize/staging
          kustomize edit set image ${{ inputs.image-name }}:${{ inputs.image-tag }}

      - name: Update production image tag
        run: |
          cd deploy/k8s/kustomize/production
          kustomize edit set image ${{ inputs.image-name }}:${{ inputs.image-tag }}

      - name: Commit and push
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add deploy/k8s/kustomize/
          git diff --staged --quiet && echo "No changes to commit" && exit 0
          git commit -m "[skip ci] update image tag for ${{ inputs.image-name }} to ${{ inputs.image-tag }}"
          git push
```

**Step 2: Commit**

```bash
git add .github/workflows/update-kustomize.yaml
git commit -m "ci: add reusable update-kustomize workflow"
```

---

### Task 4: Create the main CI dispatcher workflow

**Files:**
- Create: `.github/workflows/ci.yaml`

**Step 1: Create the workflow file**

```yaml
name: CI Pipeline

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

permissions:
  contents: write

jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      api-golang: ${{ steps.filter.outputs.api-golang }}
      api-node: ${{ steps.filter.outputs.api-node }}
      client-react: ${{ steps.filter.outputs.client-react }}
      load-generator-python: ${{ steps.filter.outputs.load-generator-python }}
      db-migrator: ${{ steps.filter.outputs.db-migrator }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            api-golang:
              - 'api-golang/**'
            api-node:
              - 'api-node/**'
            client-react:
              - 'client-react/**'
            load-generator-python:
              - 'load-generator-python/**'
            db-migrator:
              - 'postgresql/**'

  # Build jobs - triggered for both PR and push
  build-api-golang:
    needs: detect-changes
    if: ${{ needs.detect-changes.outputs.api-golang == 'true' }}
    uses: ./.github/workflows/build-service.yaml
    with:
      service-name: api-golang
      dockerfile-path: api-golang/Dockerfile
      context-path: api-golang
      image-name: hkdarealest/devops-directive-docker-course-api-golang
      push: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    secrets:
      DOCKERHUB_USERNAME: ${{ secrets.DOCKERHUB_USERNAME }}
      DOCKERHUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}

  build-api-node:
    needs: detect-changes
    if: ${{ needs.detect-changes.outputs.api-node == 'true' }}
    uses: ./.github/workflows/build-service.yaml
    with:
      service-name: api-node
      dockerfile-path: api-node/Dockerfile
      context-path: api-node
      image-name: hkdarealest/devops-directive-docker-course-api-node
      push: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    secrets:
      DOCKERHUB_USERNAME: ${{ secrets.DOCKERHUB_USERNAME }}
      DOCKERHUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}

  build-client-react:
    needs: detect-changes
    if: ${{ needs.detect-changes.outputs.client-react == 'true' }}
    uses: ./.github/workflows/build-service.yaml
    with:
      service-name: client-react
      dockerfile-path: client-react/Dockerfile
      context-path: client-react
      image-name: hkdarealest/devops-directive-docker-course-client-react-nginx
      push: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    secrets:
      DOCKERHUB_USERNAME: ${{ secrets.DOCKERHUB_USERNAME }}
      DOCKERHUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}

  build-load-generator:
    needs: detect-changes
    if: ${{ needs.detect-changes.outputs.load-generator-python == 'true' }}
    uses: ./.github/workflows/build-service.yaml
    with:
      service-name: load-generator-python
      dockerfile-path: load-generator-python/Dockerfile
      context-path: load-generator-python
      image-name: hkdarealest/devops-directive-kubernetes-course-load-generator-python
      push: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    secrets:
      DOCKERHUB_USERNAME: ${{ secrets.DOCKERHUB_USERNAME }}
      DOCKERHUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}

  build-db-migrator:
    needs: detect-changes
    if: ${{ needs.detect-changes.outputs.db-migrator == 'true' }}
    uses: ./.github/workflows/build-service.yaml
    with:
      service-name: db-migrator
      dockerfile-path: postgresql/Dockerfile
      context-path: postgresql
      image-name: hkdarealest/devops-directive-kubernetes-course-db-migrator
      push: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    secrets:
      DOCKERHUB_USERNAME: ${{ secrets.DOCKERHUB_USERNAME }}
      DOCKERHUB_TOKEN: ${{ secrets.DOCKERHUB_TOKEN }}

  # Update Kustomize tags - only on push to main
  update-api-golang-tag:
    needs: build-api-golang
    if: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    uses: ./.github/workflows/update-kustomize.yaml
    with:
      image-name: hkdarealest/devops-directive-docker-course-api-golang
      image-tag: ${{ github.sha }}

  update-api-node-tag:
    needs: build-api-node
    if: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    uses: ./.github/workflows/update-kustomize.yaml
    with:
      image-name: hkdarealest/devops-directive-docker-course-api-node
      image-tag: ${{ github.sha }}

  update-client-react-tag:
    needs: build-client-react
    if: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    uses: ./.github/workflows/update-kustomize.yaml
    with:
      image-name: hkdarealest/devops-directive-docker-course-client-react-nginx
      image-tag: ${{ github.sha }}

  update-load-generator-tag:
    needs: build-load-generator
    if: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
    uses: ./.github/workflows/update-kustomize.yaml
    with:
      image-name: hkdarealest/devops-directive-kubernetes-course-load-generator-python
      image-tag: ${{ github.sha }}
```

**Step 2: Commit**

```bash
git add .github/workflows/ci.yaml
git commit -m "ci: add main CI dispatcher workflow with path-based change detection"
```

---

### Task 5: Create ArgoCD Application manifests

**Files:**
- Create: `deploy/k8s/argocd/namespace.yaml`
- Create: `deploy/k8s/argocd/argocd-project.yaml`
- Create: `deploy/k8s/argocd/argocd-app-staging.yaml`
- Create: `deploy/k8s/argocd/argocd-app-production.yaml`

**Step 1: Create ArgoCD namespace manifest**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: argocd
```

**Step 2: Create ArgoCD AppProject**

```yaml
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: demo-app
  namespace: argocd
spec:
  description: Demo application project
  sourceRepos:
    - "https://github.com/hoangdaochuz/learn-k8s-deploy.git"
  destinations:
    - namespace: demo-app-staging
      server: https://kubernetes.default.svc
    - namespace: demo-app-production
      server: https://kubernetes.default.svc
  clusterResourceWhitelist:
    - group: ""
      kind: Namespace
```

**Step 3: Create ArgoCD Application for staging**

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: demo-app-staging
  namespace: argocd
spec:
  project: demo-app
  source:
    repoURL: https://github.com/hoangdaochuz/learn-k8s-deploy.git
    targetRevision: main
    path: deploy/k8s/kustomize/staging
  destination:
    server: https://kubernetes.default.svc
    namespace: demo-app-staging
  syncPolicy:
    automated:
      selfHeal: true
      prune: true
    syncOptions:
      - CreateNamespace=true
```

**Step 4: Create ArgoCD Application for production**

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: demo-app-production
  namespace: argocd
spec:
  project: demo-app
  source:
    repoURL: https://github.com/hoangdaochuz/learn-k8s-deploy.git
    targetRevision: main
    path: deploy/k8s/kustomize/production
  destination:
    server: https://kubernetes.default.svc
    namespace: demo-app-production
  syncPolicy:
    syncOptions:
      - CreateNamespace=true
```

**Step 5: Commit**

```bash
git add deploy/k8s/argocd/
git commit -m "feat: add ArgoCD application manifests for staging and production"
```

---

### Task 6: Add ArgoCD tasks to Taskfile

**Files:**
- Modify: `deploy/k8s/Taskfile.yaml`
- Create: `deploy/k8s/argocd/Taskfile.yaml`

**Step 1: Create ArgoCD Taskfile**

```yaml
version: "3"

tasks:
  install:
    desc: Install ArgoCD in the cluster
    cmds:
      - kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
      - kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

  apply-project:
    desc: Apply ArgoCD AppProject
    cmds:
      - kubectl apply -f ./argocd-project.yaml

  apply-apps:
    desc: Apply ArgoCD Applications (staging + production)
    cmds:
      - kubectl apply -f ./argocd-app-staging.yaml
      - kubectl apply -f ./argocd-app-production.yaml

  setup:
    desc: Full ArgoCD setup (install + project + apps)
    cmds:
      - task: install
      - echo "Waiting for ArgoCD to be ready..."
      - kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=120s
      - task: apply-project
      - task: apply-apps

  get-admin-password:
    desc: Get ArgoCD admin initial password
    cmds:
      - kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d && echo

  port-forward:
    desc: Port-forward ArgoCD UI to localhost:9080
    cmds:
      - kubectl port-forward svc/argocd-server -n argocd 9080:443
```

**Step 2: Add ArgoCD include to main Taskfile**

Add to `deploy/k8s/Taskfile.yaml` includes section:

```yaml
  argocd:
    taskfile: ./argocd/Taskfile.yaml
    dir: ./argocd
```

**Step 3: Commit**

```bash
git add deploy/k8s/argocd/Taskfile.yaml deploy/k8s/Taskfile.yaml
git commit -m "feat: add ArgoCD Taskfile for install and management"
```

---

### Task 7: Validate the full pipeline configuration

**Step 1: Verify Kustomize builds**

```bash
cd deploy/k8s && kustomize build ./kustomize/staging
cd deploy/k8s && kustomize build ./kustomize/production
```

Expected: Valid YAML output with correct image tags.

**Step 2: Validate GitHub Actions workflows**

```bash
# Check YAML syntax
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yaml'))"
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/build-service.yaml'))"
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/update-kustomize.yaml'))"
```

Expected: No errors.

**Step 3: Validate ArgoCD manifests**

```bash
python3 -c "import yaml; yaml.safe_load(open('deploy/k8s/argocd/argocd-app-staging.yaml'))"
python3 -c "import yaml; yaml.safe_load(open('deploy/k8s/argocd/argocd-app-production.yaml'))"
python3 -c "import yaml; yaml.safe_load(open('deploy/k8s/argocd/argocd-project.yaml'))"
```

Expected: No errors.
