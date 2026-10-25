package services

import (
	"testing"

	"ffxresources/backend/common"
)

func TestFilterEventIDsForVersion(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		version  common.GameVersion
		expected []string
	}{
		{
			name:     "EternalCalm: shows only sc events",
			ids:      []string{"sc1000", "sc2000", "azit0000", "bvyt0000", "lmyt0000"},
			version:  common.GameVersionEternalCalm,
			expected: []string{"sc1000", "sc2000"},
		},
		{
			name:     "FFX: hides sc events",
			ids:      []string{"sc1000", "sc2000", "azit0000", "bvyt0000", "lmyt0000"},
			version:  common.GameVersionFFX,
			expected: []string{"azit0000", "bvyt0000", "lmyt0000"},
		},
		{
			name:     "LastMiss: shows only lm events",
			ids:      []string{"lm1000", "lm2000", "azit0000", "bvyt0000"},
			version:  common.GameVersionLastMiss,
			expected: []string{"lm1000", "lm2000"},
		},
		{
			name:     "FFX2: hides lm events",
			ids:      []string{"lm1000", "lm2000", "azit0000", "bvyt0000"},
			version:  common.GameVersionFFX2,
			expected: []string{"azit0000", "bvyt0000"},
		},
		{
			name:     "other versions: no filtering",
			ids:      []string{"azit0000", "bvyt0000"},
			version:  common.GameVersionFFX, // valid version, no special filtering rules
			expected: []string{"azit0000", "bvyt0000"},
		},
		{
			name:     "empty input",
			ids:      []string{},
			version:  common.GameVersionFFX,
			expected: []string{},
		},
		{
			name:     "ids shorter than 2 chars are filtered",
			ids:      []string{"a", "b", "sc1000", "azit0000"},
			version:  common.GameVersionFFX,
			expected: []string{"azit0000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := filterEventIDsForVersion(tc.ids, tc.version)
			if len(result) != len(tc.expected) {
				t.Errorf("got %d entries, want %d", len(result), len(tc.expected))
			}
			for i, id := range result {
				if i < len(tc.expected) && id != tc.expected[i] {
					t.Errorf("result[%d] = %q, want %q", i, id, tc.expected[i])
				}
			}
		})
	}
}

func TestEventtableKindUsable(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		version  common.GameVersion
		expected bool
	}{
		{"battletext in FFX", KindBattleText, common.GameVersionFFX, true},
		{"battletext in FFX2", KindBattleText, common.GameVersionFFX2, true},
		{"battletext in EternalCalm", KindBattleText, common.GameVersionEternalCalm, false},
		{"battletext in LastMiss", KindBattleText, common.GameVersionLastMiss, false},
		{"cloud in FFX", KindCloud, common.GameVersionFFX, true},
		{"cloud in FFX2", KindCloud, common.GameVersionFFX2, true},
		{"cloud in EternalCalm", KindCloud, common.GameVersionEternalCalm, false},
		{"cloud in LastMiss", KindCloud, common.GameVersionLastMiss, false},
		{"tutorial in FFX2", KindTutorial, common.GameVersionFFX2, true},
		{"tutorial in FFX", KindTutorial, common.GameVersionFFX, false},
		{"tutorial in EternalCalm", KindTutorial, common.GameVersionEternalCalm, false},
		{"tutorial in LastMiss", KindTutorial, common.GameVersionLastMiss, false},
		{"menumain in FFX", KindMenuMain, common.GameVersionFFX, true},
		{"menumain in FFX2", KindMenuMain, common.GameVersionFFX2, false},
		{"menumain in EternalCalm", KindMenuMain, common.GameVersionEternalCalm, false},
		{"menumain in LastMiss", KindMenuMain, common.GameVersionLastMiss, false},
		{"unknown kind", "unknown", common.GameVersionFFX, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := eventtableKindUsable(tc.kind, tc.version)
			if result != tc.expected {
				t.Errorf("eventtableKindUsable(%q, %v) = %v, want %v", tc.kind, tc.version, result, tc.expected)
			}
		})
	}
}

func TestDedupViewKinds(t *testing.T) {
	// Test that dedupViewKinds contains expected kinds
	expected := map[string]bool{
		KindHelp:       true,
		KindEvents:     true,
		KindMacro:      true,
		KindBattleText: true,
		KindCloud:      true,
		KindTutorial:   true,
		KindMenuMain:   true,
	}

	for kind := range expected {
		if !dedupViewKinds[kind] {
			t.Errorf("dedupViewKinds[%q] should be true", kind)
		}
	}

	// Kinds that should NOT be in dedupViewKinds
	notExpected := []string{
		KindObjects,
		KindLockit,
		KindImages,
	}

	for _, kind := range notExpected {
		if dedupViewKinds[kind] {
			t.Errorf("dedupViewKinds[%q] should be false", kind)
		}
	}
}

func TestConstants(t *testing.T) {
	if LastMissionShortened != "lm" {
		t.Errorf("LastMissionShortened = %q, want %q", LastMissionShortened, "lm")
	}
	if EternalCalmShortened != "sc" {
		t.Errorf("EternalCalmShortened = %q, want %q", EternalCalmShortened, "sc")
	}
}

func TestKindStepLabels(t *testing.T) {
	// kindStepLabels is used for export labels
	// Verify it has entries for known kinds
	expected := map[string]string{
		KindHelp:       "Painel de ajuda",
		KindMacro:      "Dicionário",
		KindLockit:     "Loc Kit",
	}

	for kind, expectedLabel := range expected {
		if kindStepLabels[kind] != expectedLabel {
			t.Errorf("kindStepLabels[%q] = %q, want %q", kind, kindStepLabels[kind], expectedLabel)
		}
	}
}