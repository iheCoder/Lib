package pilot

import "testing"

type accessInput struct {
	admin     bool
	owner     bool
	pending   bool
	flag      bool
	suspended bool
}

type accessCase struct {
	name  string
	input accessInput
	want  bool
}

type accessPolicy func(accessInput) bool

// allowCorrect is the normalized public decision contract used as the benchmark oracle.
func allowCorrect(in accessInput) bool {
	return in.admin || in.owner && in.pending && (!in.flag || !in.suspended)
}

// allowIgnoreSuspendedMutant keeps the old owner rule even after the feature flag enables suspension checks.
func allowIgnoreSuspendedMutant(in accessInput) bool {
	return in.admin || in.owner && in.pending
}

// allowWidenedOrMutant misplaces parentheses and promotes a non-suspended account into an allow condition.
func allowWidenedOrMutant(in accessInput) bool {
	return in.admin || in.owner && in.pending || !in.suspended
}

// allowFlagGatesAdminMutant incorrectly applies the feature flag branch to the independent Admin override.
func allowFlagGatesAdminMutant(in accessInput) bool {
	if !in.flag {
		return in.owner && in.pending
	}
	return in.admin || in.owner && in.pending && !in.suspended
}

// allowSuspendedDeniesAdminMutant turns suspension into a global deny instead of an owner-path condition.
func allowSuspendedDeniesAdminMutant(in accessInput) bool {
	if in.suspended {
		return false
	}
	return in.admin || in.owner && in.pending
}

// accessSuitePasses checks only the public allow/deny result so the suite remains implementation-independent.
func accessSuitePasses(policy accessPolicy, cases []accessCase) bool {
	for _, tc := range cases {
		if policy(tc.input) != tc.want {
			return false
		}
	}
	return true
}

// TestP2DecisionSurfaceMutationKill verifies that independent-condition rows outperform happy-path sampling.
func TestP2DecisionSurfaceMutationKill(t *testing.T) {
	baseline := []accessCase{
		{name: "admin", input: accessInput{admin: true, flag: true}, want: true},
		{name: "owner", input: accessInput{owner: true, pending: true, flag: true}, want: true},
		{name: "deny", input: accessInput{pending: true, flag: true, suspended: true}, want: false},
	}
	skill := []accessCase{
		{name: "admin-override", input: accessInput{admin: true, flag: false, suspended: true}, want: true},
		{name: "owner-enabled", input: accessInput{owner: true, pending: true, flag: true}, want: true},
		{name: "suspended-changes", input: accessInput{owner: true, pending: true, flag: true, suspended: true}, want: false},
		{name: "flag-preserves-old", input: accessInput{owner: true, pending: true, flag: false, suspended: true}, want: true},
		{name: "owner-required", input: accessInput{pending: true, flag: true}, want: false},
		{name: "pending-required", input: accessInput{owner: true, flag: true}, want: false},
	}
	mutants := []accessPolicy{
		allowIgnoreSuspendedMutant,
		allowWidenedOrMutant,
		allowFlagGatesAdminMutant,
		allowSuspendedDeniesAdminMutant,
	}

	assertCorrectCandidate(t, accessSuitePasses(allowCorrect, skill), "P2")
	baselineKills := countAccessKills(mutants, baseline)
	skillKills := countAccessKills(mutants, skill)
	if baselineKills != 0 || skillKills != len(mutants) {
		t.Fatalf("P2 unexpected kill matrix: baseline=%d skill=%d mutants=%d", baselineKills, skillKills, len(mutants))
	}
	t.Logf("P2 kill matrix: baseline=%d/%d skill=%d/%d", baselineKills, len(mutants), skillKills, len(mutants))
}

// countAccessKills counts seeded policies whose externally observable decision violates at least one case.
func countAccessKills(mutants []accessPolicy, cases []accessCase) int {
	kills := 0
	for _, mutant := range mutants {
		if !accessSuitePasses(mutant, cases) {
			kills++
		}
	}
	return kills
}
