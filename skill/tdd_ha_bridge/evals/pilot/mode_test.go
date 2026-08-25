package pilot

import (
	"encoding/json"
	"errors"
	"testing"
)

const defaultMode = 7

var errNegativeMode = errors.New("mode must be non-negative")

type modeParser func([]byte) (int, error)

type modeCase struct {
	name    string
	payload string
	want    int
	wantErr bool
}

// parseModeCorrect preserves field presence so explicit zero and missing remain different business inputs.
func parseModeCorrect(payload []byte) (int, error) {
	var raw struct {
		Mode *int `json:"mode"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return 0, err
	}
	if raw.Mode == nil {
		return defaultMode, nil
	}
	if *raw.Mode < 0 {
		return 0, errNegativeMode
	}
	return *raw.Mode, nil
}

// parseModeZeroAsMissingMutant models the common Go bug where a scalar zero value is treated as field absence.
func parseModeZeroAsMissingMutant(payload []byte) (int, error) {
	var raw struct {
		Mode int `json:"mode"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return 0, err
	}
	if raw.Mode == 0 {
		return defaultMode, nil
	}
	if raw.Mode < 0 {
		return 0, errNegativeMode
	}
	return raw.Mode, nil
}

// parseModeMissingAsZeroMutant preserves explicit zero but silently maps an absent field to zero.
func parseModeMissingAsZeroMutant(payload []byte) (int, error) {
	var raw struct {
		Mode int `json:"mode"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return 0, err
	}
	if raw.Mode < 0 {
		return 0, errNegativeMode
	}
	return raw.Mode, nil
}

// parseModeAlwaysDefaultMutant simulates an incident-specific patch that overwrites every valid value.
func parseModeAlwaysDefaultMutant(payload []byte) (int, error) {
	var raw map[string]int
	if err := json.Unmarshal(payload, &raw); err != nil {
		return 0, err
	}
	if mode, ok := raw["mode"]; ok && mode < 0 {
		return 0, errNegativeMode
	}
	return defaultMode, nil
}

// parseModeAcceptNegativeMutant implements the presence contract but omits its validation boundary.
func parseModeAcceptNegativeMutant(payload []byte) (int, error) {
	var raw struct {
		Mode *int `json:"mode"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return 0, err
	}
	if raw.Mode == nil {
		return defaultMode, nil
	}
	return *raw.Mode, nil
}

// modeSuitePasses executes an observable contract against one candidate parser without inspecting its implementation.
func modeSuitePasses(parser modeParser, cases []modeCase) bool {
	for _, tc := range cases {
		got, err := parser([]byte(tc.payload))
		if (err != nil) != tc.wantErr || err == nil && got != tc.want {
			return false
		}
	}
	return true
}

// TestP1ModeMutationKill compares a simple happy-path baseline with the skill-derived generalized-neighbor suite.
func TestP1ModeMutationKill(t *testing.T) {
	baseline := []modeCase{{name: "normal", payload: `{"mode":3}`, want: 3}}
	skill := []modeCase{
		{name: "explicit-zero", payload: `{"mode":0}`, want: 0},
		{name: "missing", payload: `{}`, want: defaultMode},
		{name: "normal", payload: `{"mode":3}`, want: 3},
		{name: "negative", payload: `{"mode":-1}`, wantErr: true},
	}
	mutants := []modeParser{
		parseModeZeroAsMissingMutant,
		parseModeMissingAsZeroMutant,
		parseModeAlwaysDefaultMutant,
		parseModeAcceptNegativeMutant,
	}

	assertCorrectCandidate(t, modeSuitePasses(parseModeCorrect, skill), "P1")
	baselineKills := countModeKills(mutants, baseline)
	skillKills := countModeKills(mutants, skill)
	if baselineKills != 1 || skillKills != len(mutants) {
		t.Fatalf("P1 unexpected kill matrix: baseline=%d skill=%d mutants=%d", baselineKills, skillKills, len(mutants))
	}
	t.Logf("P1 kill matrix: baseline=%d/%d skill=%d/%d", baselineKills, len(mutants), skillKills, len(mutants))
}

// countModeKills counts candidates rejected by a suite; rejection means the suite exposes the seeded contract fault.
func countModeKills(mutants []modeParser, cases []modeCase) int {
	kills := 0
	for _, mutant := range mutants {
		if !modeSuitePasses(mutant, cases) {
			kills++
		}
	}
	return kills
}

// assertCorrectCandidate protects the benchmark from rewarding suites that also reject the correct implementation.
func assertCorrectCandidate(t *testing.T, passed bool, caseID string) {
	t.Helper()
	if !passed {
		t.Fatalf("%s rejected the correct implementation", caseID)
	}
}
