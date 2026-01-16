# AI Agent Prompt: Implement Operator Drain Feature

## Context

You are working on the ngrok-operator, a Kubernetes operator that reconciles Ingress/Gateway API resources into ngrok cloud resources. The operator runs as multiple pods:
- **api-manager**: Main controller that creates CloudEndpoints, Domains, etc.
- **agent-manager**: Runs ngrok tunnels for AgentEndpoints
- **bindings-forwarder-manager**: Handles endpoint bindings (rarely used)

## Problem Being Solved

When users uninstall the helm chart, resources get stuck because:
1. The operator pod is deleted before it can remove finalizers
2. CRDs are deleted, kicking all CRs into termination
3. CRs have finalizers but no controller to remove them
4. This blocks namespace deletion and leaves orphaned resources

## Solution Overview

We're implementing a "drain" feature that:
1. Uses a helm pre-delete hook to trigger drain before uninstall
2. The KubernetesOperator CR serves as the drain trigger
3. All controllers check if draining and stop adding finalizers
4. The drainer removes finalizers from all managed resources
5. Supports two modes:
   - **Retain** (default): Just remove finalizers, preserve ngrok API resources
   - **Delete**: Remove finalizers AND delete CRs (controllers clean up ngrok API)

## Your Task

Execute the implementation plan in `plan.md` step by step. Work through each milestone in the recommended execution order:

1. **M1** (Helm & CRD) - Foundation for everything
2. **M6** (Drainer Improvements) - Fix correctness issues  
3. **M2** (Drain State Propagation) - Core functionality
4. **M3** (Driver/Store) - Stop new resources being created
5. **M4** (Agent Manager) - Extend to second pod
6. **M5** (Bindings Forwarder) - Extend to third pod
7. **M7** (Helm Cleanup) - Polish
8. **M8** (Testing) - Validate everything works

## Instructions

1. **Read the plan first**: Start by reading `plan.md` thoroughly to understand all milestones and tasks.

2. **Work incrementally**: Complete one task at a time, verify it compiles, then move to the next.

3. **Check in regularly**: After completing each major task or when you have questions, summarize what you did and ask for confirmation before proceeding.

4. **Run verification commands**: After making changes, run:
   - `make generate` - Regenerate DeepCopy methods
   - `make manifests` - Regenerate CRDs
   - `make build` - Verify compilation
   - `go test ./internal/drain/...` - Run drain package tests

5. **Ask clarifying questions** when:
   - You're unsure about a design decision
   - You find something in the existing code that contradicts the plan
   - You think there's a better approach
   - You need to understand existing code patterns

6. **Key files to reference**:
   - `internal/drain/drain.go` - Existing drainer implementation
   - `internal/drain/state.go` - Existing StateChecker
   - `internal/controller/base_controller.go` - BaseController pattern
   - `cmd/api-manager.go` - Main operator setup
   - `cmd/agent-manager.go` - Agent manager setup
   - `api/ngrok/v1alpha1/kubernetesoperator_types.go` - KubernetesOperator CRD
   - `helm/ngrok-operator/values.yaml` - Helm values

7. **Important patterns**:
   - Use `opts.releaseName` for KubernetesOperator CR name lookup
   - Use `opts.managerName` for controller labels (resource ownership)
   - Check `if !controller.IsDelete(obj)` before skipping reconciles during drain
   - The drain state should be a read-only interface that controllers query

## Current Branch State

This is a work-in-progress branch. Run `git diff origin/main` to see what's already implemented. Key things already done:
- KubernetesOperator CRD has DrainMode and drain status fields
- Basic Drainer struct exists
- StateChecker exists (but has bugs - see plan)
- Helm cleanup hook exists

## Example Check-in Format

After completing a task, report like this:

```
## Completed: Task X.Y - [Task Name]

### Changes Made:
- Modified `file1.go`: Added DrainState field to struct
- Modified `file2.go`: Added drain check in Reconcile()

### Verification:
- `make build` ✓
- `go test ./path/...` ✓

### Questions/Notes:
- [Any questions or things you noticed]

### Next Task:
- Task X.Z - [Next task name]

Shall I proceed?
```

## Start Here

Begin by reading `plan.md`, then start with **Milestone 1, Task 1.1: Add Helm value for drain policy**. Check in after completing it.
