package objectsfile

import (
	"testing"

	"ffxresources/backend/common"
)

func TestFileLayout_PatternPath(t *testing.T) {
	layout := FileLayout{
		Version:    common.GameVersionFFX,
		DirPattern: "battle/kernel",
		FileName:   "command.bin",
	}

	expected := "battle/kernel/command.bin"
	if layout.PatternPath() != expected {
		t.Errorf("PatternPath() = %q, want %q", layout.PatternPath(), expected)
	}
}

func TestFileLayout_FilePath(t *testing.T) {
	tests := []struct {
		name     string
		layout   FileLayout
		expected string
	}{
		{
			name: "FFX file",
			layout: FileLayout{
				Version:    common.GameVersionFFX,
				DirPattern: "battle/kernel",
				FileName:   "command.bin",
			},
			expected: "ffx/battle/kernel/command.bin",
		},
		{
			name: "FFX-2 file",
			layout: FileLayout{
				Version:    common.GameVersionFFX2,
				DirPattern: "battle/kernel",
				FileName:   "item.bin",
			},
			expected: "ffx2/battle/kernel/item.bin",
		},
		{
			name: "LastMission file",
			layout: FileLayout{
				Version:    common.GameVersionLastMiss,
				DirPattern: "lastmiss/kernel",
				FileName:   "lm_command.bin",
			},
			// VersionPathName returns "ffx2" for LastMission
			expected: "ffx2/lastmiss/kernel/lm_command.bin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.layout.FilePath()
			if result != tc.expected {
				t.Errorf("FilePath() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestFileLayoutKey(t *testing.T) {
	tests := []struct {
		name           string
		version        common.GameVersion
		patternPath    string
		expected       string
	}{
		{
			name:           "no prefix adds prefix",
			version:        common.GameVersionFFX,
			patternPath:    "battle/kernel/command.bin",
			expected:       "ffx/battle/kernel/command.bin",
		},
		{
			name:           "already has prefix keeps it",
			version:        common.GameVersionFFX,
			patternPath:    "ffx/battle/kernel/command.bin",
			expected:       "ffx/battle/kernel/command.bin",
		},
		{
			name:           "FFX-2 version",
			version:        common.GameVersionFFX2,
			patternPath:    "battle/kernel/item.bin",
			expected:       "ffx2/battle/kernel/item.bin",
		},
		{
			name:           "LastMission version",
			version:        common.GameVersionLastMiss,
			patternPath:    "lastmiss/kernel/lm_command.bin",
			expected:       "lastmiss/kernel/lm_command.bin",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FileLayoutKey(tc.version, tc.patternPath)
			if result != tc.expected {
				t.Errorf("FileLayoutKey(%v, %q) = %q, want %q", tc.version, tc.patternPath, result, tc.expected)
			}
		})
	}
}

func TestFileLayoutFor(t *testing.T) {
	tests := []struct {
		name     string
		version  common.GameVersion
		path     string
		expected FileLayout
		exists   bool
	}{
		{
			name:     "existing FFX layout",
			version:  common.GameVersionFFX,
			path:     "battle/kernel/command.bin",
			expected: FileLayout{
				Version:    common.GameVersionFFX,
				DirPattern: "battle/kernel",
				FileName:   "command.bin",
			},
			exists: true,
		},
		{
			name:     "existing FFX layout with prefix",
			version:  common.GameVersionFFX,
			path:     "ffx/battle/kernel/command.bin",
			expected: FileLayout{
				Version:    common.GameVersionFFX,
				DirPattern: "battle/kernel",
				FileName:   "command.bin",
			},
			exists: true,
		},
		{
			name:     "existing FFX-2 layout",
			version:  common.GameVersionFFX2,
			path:     "battle/kernel/item.bin",
			expected: FileLayout{
				Version:    common.GameVersionFFX2,
				DirPattern: "battle/kernel",
				FileName:   "item.bin",
			},
			exists: true,
		},
		{
			name:     "existing LastMission layout",
			version:  common.GameVersionLastMiss,
			path:     "lastmiss/kernel/lm_command.bin",
			expected: FileLayout{
				Version:    common.GameVersionLastMiss,
				DirPattern: "lastmiss/kernel",
				FileName:   "lm_command.bin",
			},
			exists: true,
		},
		{
			name:   "non-existent layout",
			version: common.GameVersionFFX,
			path:    "battle/kernel/nonexistent.bin",
			exists:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			layout, ok := FileLayoutFor(tc.version, tc.path)
			if ok != tc.exists {
				t.Errorf("FileLayoutFor(%v, %q) ok = %v, want %v", tc.version, tc.path, ok, tc.exists)
			}
			if tc.exists {
				if layout.Version != tc.expected.Version {
					t.Errorf("Version = %v, want %v", layout.Version, tc.expected.Version)
				}
				if layout.DirPattern != tc.expected.DirPattern {
					t.Errorf("DirPattern = %q, want %q", layout.DirPattern, tc.expected.DirPattern)
				}
				if layout.FileName != tc.expected.FileName {
					t.Errorf("FileName = %q, want %q", layout.FileName, tc.expected.FileName)
				}
			}
		})
	}
}

func TestFileLayouts_MapNotEmpty(t *testing.T) {
	if len(FileLayouts) == 0 {
		t.Fatal("FileLayouts map is empty")
	}

	// Verify some expected keys exist
	expectedKeys := []string{
		"ffx/battle/kernel/command.bin",
		"ffx/battle/kernel/item.bin",
		"ffx2/battle/kernel/command.bin",
		"ffx2/battle/kernel/item.bin",
		"lastmiss/kernel/lm_command.bin",
	}

	for _, key := range expectedKeys {
		if _, ok := FileLayouts[key]; !ok {
			t.Errorf("expected key %q not found in FileLayouts", key)
		}
	}
}

func TestFileLayouts_VersionConsistency(t *testing.T) {
	// Verify that all layouts have consistent version/dirPattern/fileName
	for key, layout := range FileLayouts {
		expectedKey := FileLayoutKey(layout.Version, layout.PatternPath())
		if key != expectedKey {
			t.Errorf("key mismatch: map key = %q, computed key = %q", key, expectedKey)
		}

		if layout.FileName == "" {
			t.Errorf("empty FileName for key %q", key)
		}
		if layout.DirPattern == "" {
			t.Errorf("empty DirPattern for key %q", key)
		}
	}
}

func TestFileLayouts_Count(t *testing.T) {
	// Just verify we have a reasonable number of layouts
	if len(FileLayouts) < 50 {
		t.Errorf("expected at least 50 layouts, got %d", len(FileLayouts))
	}
}