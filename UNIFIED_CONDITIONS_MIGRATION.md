# Unified Conditions System Migration Plan

## Overview

This document outlines the migration from the current duplicated condition management to a unified conditions system that eliminates code duplication and provides consistent condition handling across all controllers.

## Current State Analysis

### Problems Identified
1. **Code Duplication**: `isDomainReady`, `findTrafficPolicyByName`, and condition setter functions are duplicated across AgentEndpoint and CloudEndpoint controllers
2. **Inconsistent Patterns**: Different controllers use slightly different condition management approaches
3. **Manual Status Calls**: Controllers manually call `ReconcileStatus` multiple times, leading to redundant API calls
4. **Scattered Logic**: Domain and traffic policy logic is embedded in controller files rather than centralized

### Current Duplication
- `isDomainReady` function exists in both controllers (identical implementation)
- `findTrafficPolicyByName` duplicated with minimal differences
- `ensureDomainExists` logic duplicated with slight variations
- Condition setter functions (`setReadyCondition`, `setTrafficPolicyCondition`, etc.) duplicated in separate files

## Unified Architecture

### New Package Structure
```
internal/controller/conditions/
├── conditions.go      # Core types and interfaces
├── manager.go         # Batch condition update manager  
├── domain.go          # Domain-specific condition logic
├── traffic_policy.go  # Traffic policy condition logic
├── adapters.go        # Type adapters for existing resources
└── example_usage.go   # Usage examples and patterns
```

### Key Components

#### 1. ConditionTarget Interface
```go
type ConditionTarget interface {
    GetGeneration() int64
    GetConditions() []metav1.Condition
    SetConditions([]metav1.Condition)
}
```

#### 2. Condition Manager
- Batches condition updates to minimize API calls
- Provides fluent API for setting multiple conditions
- Handles automatic status reconciliation

#### 3. Specialized Managers
- **DomainManager**: Centralized domain creation and condition logic
- **TrafficPolicyManager**: Unified traffic policy validation and error handling

## Migration Strategy

### Phase 1: Foundation (Low Risk)
**Goal**: Establish unified system alongside existing code

1. **Create new conditions package** ✅
   - Implement core interfaces and managers
   - Add adapter types for existing resources
   - Create comprehensive tests

2. **Add integration tests**
   - Test condition manager with real Kubernetes objects
   - Validate domain and traffic policy managers
   - Ensure backward compatibility

3. **Update one controller as proof of concept**
   - Start with CloudEndpoint controller
   - Maintain old code paths as fallback
   - Compare behavior between old and new implementations

### Phase 2: CloudEndpoint Migration (Medium Risk)
**Goal**: Migrate CloudEndpoint controller to new system

1. **Update CloudEndpointReconciler**
   ```go
   // OLD: Manual condition management
   setReconcilingCondition(clep, "Reconciling CloudEndpoint")
   domain, err := r.ensureDomainExists(ctx, clep)
   if err != nil {
       return r.controller.ReconcileStatus(ctx, clep, err)
   }
   
   // NEW: Unified condition management
   adapter := conditions.NewCloudEndpointAdapter(clep)
   conditionManager := conditions.NewManager(r.Client, clep, adapter)
   conditionManager.SetReconciling("Reconciling CloudEndpoint")
   
   domainManager := conditions.NewDomainManager(r.Client, r.Recorder, r.DefaultDomainReclaimPolicy)
   domain, err := domainManager.EnsureDomainExists(ctx, clep.Spec.URL, clep.Namespace, conditionManager, clep.Name)
   if err != nil {
       return conditionManager.ApplyAndUpdate(ctx)
   }
   ```

2. **Remove duplicated functions**
   - Delete `isDomainReady` from cloudendpoint_controller.go
   - Delete `ensureDomainExists` from cloudendpoint_controller.go  
   - Delete `findTrafficPolicyByName` from cloudendpoint_controller.go
   - Remove cloudendpoint_conditions.go file

3. **Update imports and references**
   - Import new conditions package
   - Update any remaining references

### Phase 3: AgentEndpoint Migration (Medium Risk)
**Goal**: Migrate AgentEndpoint controller to new system

1. **Update AgentEndpointReconciler**
   - Follow similar pattern as CloudEndpoint
   - Handle AgentEndpoint-specific traffic policy structure
   - Maintain agent-specific domain logic

2. **Remove duplicated functions**
   - Delete duplicated functions from agent_endpoint_controller.go
   - Remove agent/conditions.go file

### Phase 4: Domain Controller Integration (Low Risk)
**Goal**: Integrate domain controller with unified system

1. **Update domain controller**
   - Use unified condition constants and patterns
   - Leverage shared domain condition logic
   - Maintain domain-specific business logic

2. **Consolidate domain conditions**
   - Move domain-specific conditions to unified system
   - Remove domain_conditions.go or integrate with new system

### Phase 5: Cleanup and Optimization (Low Risk)
**Goal**: Clean up legacy code and optimize

1. **Remove legacy files**
   - Delete unused condition files
   - Clean up imports
   - Update tests

2. **Performance optimization**
   - Audit condition update patterns
   - Minimize status API calls
   - Add metrics for condition changes

## Benefits

### Immediate Benefits
1. **Reduced Code Duplication**: ~200 lines of duplicated code eliminated
2. **Consistent Behavior**: All controllers use identical condition logic
3. **Easier Testing**: Centralized logic is easier to unit test
4. **Better Error Handling**: Standardized error condition patterns

### Long-term Benefits
1. **Easier Maintenance**: Single place to update condition logic
2. **Feature Velocity**: New controllers can leverage existing condition patterns
3. **Better Observability**: Consistent condition patterns across all resources
4. **Reduced API Calls**: Batched status updates reduce Kubernetes API load

## Risk Mitigation

### Backward Compatibility
- New system maintains exact same condition types and reasons
- Status structure remains unchanged
- Migration is invisible to end users

### Testing Strategy
- Comprehensive unit tests for new condition system
- Integration tests comparing old vs new behavior
- Gradual migration with ability to rollback

### Monitoring
- Add metrics for condition update patterns
- Monitor status update frequency
- Track reconciliation performance

## Success Criteria

1. **Code Reduction**: >150 lines of duplicated code eliminated
2. **Performance**: No regression in reconciliation performance
3. **Reliability**: No behavior changes from user perspective
4. **Maintainability**: New controllers can reuse 80%+ of condition logic

## Timeline

- **Week 1-2**: Phase 1 (Foundation)
- **Week 3**: Phase 2 (CloudEndpoint Migration)  
- **Week 4**: Phase 3 (AgentEndpoint Migration)
- **Week 5**: Phase 4 (Domain Controller Integration)
- **Week 6**: Phase 5 (Cleanup and Optimization)

## Rollback Plan

Each phase maintains the previous implementation until the new system is validated:
1. Keep old condition files until migration is complete
2. Use feature flags to switch between old/new implementations
3. Maintain old controller methods as fallbacks
4. Have comprehensive test coverage to detect regressions
