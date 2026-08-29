package behavior_discovery_eval

import "testing"

// workflowOutcome separates the requested local change from the condition needed to finish the wider goal.
// The benchmark accepts preservation or an explicit recovery path; silently continuing into a raw failure is invalid.
type workflowOutcome struct {
	selectionSaved      bool
	conditionAvailable  bool
	recoveryOrHandoff   bool
	rawExecutionFailure bool
}

func workflowCorrect() workflowOutcome {
	return workflowOutcome{selectionSaved: true, conditionAvailable: true}
}

// workflowLostConditionMutant completes the local update but deletes a non-target prerequisite.
func workflowLostConditionMutant() workflowOutcome {
	return workflowOutcome{selectionSaved: true, rawExecutionFailure: true}
}

func narrowWorkflowCheck(candidate func() workflowOutcome) bool {
	return candidate().selectionSaved
}

func discoveryWorkflowCheck(candidate func() workflowOutcome) bool {
	outcome := candidate()
	canContinueSafely := outcome.conditionAvailable || outcome.recoveryOrHandoff
	return outcome.selectionSaved && canContinueSafely && !outcome.rawExecutionFailure
}

// compressionOutcome records both the transformation result and the real effect governed by the preserved policy.
type compressionOutcome struct {
	summaryCreated bool
	denyPreserved  bool
	externalSends  int
}

func compressionCorrect() compressionOutcome {
	return compressionOutcome{summaryCreated: true, denyPreserved: true}
}

// compressionDropsConstraintMutant keeps the user's goal while losing the still-active no-send restriction.
func compressionDropsConstraintMutant() compressionOutcome {
	return compressionOutcome{summaryCreated: true, externalSends: 1}
}

func narrowCompressionCheck(candidate func() compressionOutcome) bool {
	return candidate().summaryCreated
}

func discoveryCompressionCheck(candidate func() compressionOutcome) bool {
	outcome := candidate()
	return outcome.summaryCreated && outcome.denyPreserved && outcome.externalSends == 0
}

// replanOutcome allows the plan shape to change while keeping the approval invariant observable at the action boundary.
type replanOutcome struct {
	alternativePlanUsed bool
	approvalObserved    bool
	highRiskEffects     int
}

func replanCorrect() replanOutcome {
	return replanOutcome{alternativePlanUsed: true, approvalObserved: true, highRiskEffects: 1}
}

// replanBypassesApprovalMutant reaches the local goal through a new path without the required approval.
func replanBypassesApprovalMutant() replanOutcome {
	return replanOutcome{alternativePlanUsed: true, highRiskEffects: 1}
}

func narrowReplanCheck(candidate func() replanOutcome) bool {
	outcome := candidate()
	return outcome.alternativePlanUsed && outcome.highRiskEffects == 1
}

func discoveryReplanCheck(candidate func() replanOutcome) bool {
	outcome := candidate()
	return outcome.alternativePlanUsed && outcome.approvalObserved && outcome.highRiskEffects == 1
}

// patchOutcome makes the target write and the preserved non-target field independently observable.
type patchOutcome struct {
	displayName string
	owner       string
}

func patchCorrect() patchOutcome {
	return patchOutcome{displayName: "new", owner: "alice"}
}

// patchOverwritesOwnerMutant updates the requested field correctly but zeroes an omitted field.
func patchOverwritesOwnerMutant() patchOutcome {
	return patchOutcome{displayName: "new"}
}

func narrowPatchCheck(candidate func() patchOutcome) bool {
	return candidate().displayName == "new"
}

func discoveryPatchCheck(candidate func() patchOutcome) bool {
	outcome := candidate()
	return outcome.displayName == "new" && outcome.owner == "alice"
}

// TestBehaviorDiscoveryWitnessesKillPreservationFaults demonstrates the verification value of the new abstraction.
// It does not prove that a model will generate these witnesses; that requires an isolated old/new model evaluation.
func TestBehaviorDiscoveryWitnessesKillPreservationFaults(t *testing.T) {
	testStrategyPair(t, "cross-state workflow", narrowWorkflowCheck, discoveryWorkflowCheck, workflowCorrect, workflowLostConditionMutant)
	testStrategyPair(t, "context compression", narrowCompressionCheck, discoveryCompressionCheck, compressionCorrect, compressionDropsConstraintMutant)
	testStrategyPair(t, "replanning approval", narrowReplanCheck, discoveryReplanCheck, replanCorrect, replanBypassesApprovalMutant)
	testStrategyPair(t, "deterministic patch", narrowPatchCheck, discoveryPatchCheck, patchCorrect, patchOverwritesOwnerMutant)
}

func testStrategyPair[T any](
	t *testing.T,
	name string,
	narrowCheck func(func() T) bool,
	discoveryCheck func(func() T) bool,
	correct func() T,
	mutant func() T,
) {
	t.Helper()
	if !narrowCheck(correct) || !narrowCheck(mutant) {
		t.Fatalf("%s: narrow check must accept both candidates to expose its blind spot", name)
	}
	if !discoveryCheck(correct) || discoveryCheck(mutant) {
		t.Fatalf("%s: discovery check must accept correct and reject preserved-state mutant", name)
	}
}

// TestStatelessNegativeControl keeps the discovery step adaptive: a single-turn calculation needs no transition model.
func TestStatelessNegativeControl(t *testing.T) {
	calculate := func(price, discount int) (int, bool) {
		if discount > price {
			return 0, false
		}
		return price - discount, true
	}

	if total, ok := calculate(100, 30); !ok || total != 70 {
		t.Fatalf("valid calculation: total=%d ok=%v", total, ok)
	}
	if _, ok := calculate(100, 101); ok {
		t.Fatal("discount greater than price must be rejected")
	}
}
