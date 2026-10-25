package models

import (
	"bytes"
	"testing"

	"ffxresources/backend/common"
)

func TestNewEventFileInfo(t *testing.T) {
	tests := []struct {
		name            string
		eventID         string
		version         common.GameVersion
		expectedPattern string
	}{
		{
			name:            "standard event ID",
			eventID:         "azit0000",
			version:         common.GameVersionFFX,
			expectedPattern: "event/obj_ps3/az/azit0000/azit0000.bin",
		},
		{
			name:            "event ID with different prefix",
			eventID:         "bvyt0000",
			version:         common.GameVersionFFX2,
			expectedPattern: "event/obj_ps3/bv/bvyt0000/bvyt0000.bin",
		},
		{
			name:            "short event ID (less than 2 chars)",
			eventID:         "a",
			version:         common.GameVersionFFX,
			expectedPattern: "", // populate() returns early for len < 2
		},
		{
			name:            "empty event ID",
			eventID:         "",
			version:         common.GameVersionFFX,
			expectedPattern: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := NewEventFileInfo(tc.eventID, tc.version)
			if info.EventID != tc.eventID {
				t.Errorf("EventID = %q, want %q", info.EventID, tc.eventID)
			}
			if info.Version != tc.version {
				t.Errorf("Version = %v, want %v", info.Version, tc.version)
			}
			if info.LocalizationPattern != tc.expectedPattern {
				t.Errorf("LocalizationPattern = %q, want %q", info.LocalizationPattern, tc.expectedPattern)
			}
		})
	}
}

func TestSegment_ReadWrite(t *testing.T) {
	// Test ReadSegmentAt/WriteSegmentAt
	seg := Segment{
		Offset: 0x1234,
		Key:    0x5678,
	}

	data := make([]byte, 4)
	err := WriteSegmentAt(data, 0, seg)
	if err != nil {
		t.Fatalf("WriteSegmentAt failed: %v", err)
	}

	readSeg, err := ReadSegmentAt(data, 0)
	if err != nil {
		t.Fatalf("ReadSegmentAt failed: %v", err)
	}

	if readSeg.Offset != seg.Offset || readSeg.Key != seg.Key {
		t.Errorf("roundtrip: got Offset=%X Key=%X, want Offset=%X Key=%X",
			readSeg.Offset, readSeg.Key, seg.Offset, seg.Key)
	}

	// Test bounds checking
	_, err = ReadSegmentAt(data, -1)
	if err == nil {
		t.Error("expected error for negative position")
	}

	_, err = ReadSegmentAt(data, 1) // not enough bytes
	if err == nil {
		t.Error("expected error for position with insufficient bytes")
	}

	err = WriteSegmentAt(data, -1, seg)
	if err == nil {
		t.Error("expected error for negative write position")
	}

	err = WriteSegmentAt(data, 1, seg)
	if err == nil {
		t.Error("expected error for write position with insufficient bytes")
	}
}

func TestSegment_IO(t *testing.T) {
	seg := Segment{
		Offset: 0xABCD,
		Key:    0xEF01,
	}

	// Test WriteSegment/ReadSegment with bytes.Buffer
	var buf bytes.Buffer
	err := WriteSegment(&buf, seg)
	if err != nil {
		t.Fatalf("WriteSegment failed: %v", err)
	}

	readSeg, err := ReadSegment(&buf)
	if err != nil {
		t.Fatalf("ReadSegment failed: %v", err)
	}

	if readSeg.Offset != seg.Offset || readSeg.Key != seg.Key {
		t.Errorf("roundtrip: got Offset=%X Key=%X, want Offset=%X Key=%X",
			readSeg.Offset, readSeg.Key, seg.Offset, seg.Key)
	}
}

func TestTextRef(t *testing.T) {
	seg := Segment{Offset: 0x100, Key: 0x200}
	ref := TextRef(seg)

	// Test ExportTextSegments
	values := ref.ExportTextSegments()
	if len(values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(values))
	}
	if values[0].Part != TextPartStandard {
		t.Errorf("expected Standard part, got %v", values[0].Part)
	}
	if values[0].Segment != seg {
		t.Errorf("expected segment %v, got %v", seg, values[0].Segment)
	}

	// Test ImportTextSegments
	var newRef TextRef
	err := newRef.ImportTextSegments(values)
	if err != nil {
		t.Fatalf("ImportTextSegments failed: %v", err)
	}
	if TextRef(seg) != newRef {
		t.Errorf("import: expected %v, got %v", ref, newRef)
	}

	// Test invalid import
	err = newRef.ImportTextSegments([]TextValue{
		{Part: TextPartStandard, Segment: seg},
		{Part: TextPartSimplified, Segment: seg}, // too many
	})
	if err == nil {
		t.Error("expected error for too many segments")
	}

	err = newRef.ImportTextSegments([]TextValue{
		{Part: TextPartSimplified, Segment: seg}, // wrong part
	})
	if err == nil {
		t.Error("expected error for wrong part")
	}
}

func TestSimplifiedRef(t *testing.T) {
	seg := Segment{Offset: 0x300, Key: 0x400}
	ref := SimplifiedRef(seg)

	// Test ExportTextSegments
	values := ref.ExportTextSegments()
	if len(values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(values))
	}
	if values[0].Part != TextPartSimplified {
		t.Errorf("expected Simplified part, got %v", values[0].Part)
	}
	if values[0].Segment != seg {
		t.Errorf("expected segment %v, got %v", seg, values[0].Segment)
	}

	// Test ImportTextSegments
	var newRef SimplifiedRef
	err := newRef.ImportTextSegments(values)
	if err != nil {
		t.Fatalf("ImportTextSegments failed: %v", err)
	}
	if SimplifiedRef(seg) != newRef {
		t.Errorf("import: expected %v, got %v", ref, newRef)
	}

	// Test invalid imports
	err = newRef.ImportTextSegments([]TextValue{
		{Part: TextPartStandard, Segment: seg}, // wrong part
	})
	if err == nil {
		t.Error("expected error for Standard part")
	}

	err = newRef.ImportTextSegments([]TextValue{
		{Part: TextPartSimplified, Segment: seg},
		{Part: TextPartStandard, Segment: seg}, // too many
	})
	if err == nil {
		t.Error("expected error for too many segments")
	}
}

func TestTextPair(t *testing.T) {
	stdSeg := Segment{Offset: 0x500, Key: 0x600}
	simpSeg := Segment{Offset: 0x700, Key: 0x800}
	pair := TextPair{
		Standard:   TextRef(stdSeg),
		Simplified: TextRef(simpSeg),
	}

	// Test ExportTextSegments
	values := pair.ExportTextSegments()
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
	if values[0].Part != TextPartStandard || values[0].Segment != stdSeg {
		t.Errorf("first value: expected Standard %v, got %v %v", stdSeg, values[0].Part, values[0].Segment)
	}
	if values[1].Part != TextPartSimplified || values[1].Segment != simpSeg {
		t.Errorf("second value: expected Simplified %v, got %v %v", simpSeg, values[1].Part, values[1].Segment)
	}
	if values[1].SameAs != TextPartStandard {
		t.Errorf("expected SameAs=Standard, got %v", values[1].SameAs)
	}

	// Test ImportTextSegments
	var newPair TextPair
	err := newPair.ImportTextSegments(values)
	if err != nil {
		t.Fatalf("ImportTextSegments failed: %v", err)
	}
	if TextRef(stdSeg) != newPair.Standard {
		t.Errorf("Standard: expected %v, got %v", stdSeg, newPair.Standard)
	}
	if TextRef(simpSeg) != newPair.Simplified {
		t.Errorf("Simplified: expected %v, got %v", simpSeg, newPair.Simplified)
	}

	// Test invalid imports
	err = newPair.ImportTextSegments([]TextValue{
		{Part: TextPartStandard, Segment: stdSeg}, // only one
	})
	if err == nil {
		t.Error("expected error for single segment")
	}

	err = newPair.ImportTextSegments([]TextValue{
		{Part: TextPartStandard, Segment: stdSeg},
		{Part: TextPartStandard, Segment: simpSeg}, // duplicate Standard
	})
	if err == nil {
		t.Error("expected error for duplicate Standard")
	}

	err = newPair.ImportTextSegments([]TextValue{
		{Part: TextPartSimplified, Segment: stdSeg},
		{Part: TextPartSimplified, Segment: simpSeg}, // duplicate Simplified
	})
	if err == nil {
		t.Error("expected error for duplicate Simplified")
	}
}

func TestNameDescriptionHeaderData(t *testing.T) {
	h := NameDescriptionHeaderData{
		NameSegment:                   Segment{Offset: 1, Key: 2},
		SimplifiedNameSegment:         Segment{Offset: 3, Key: 4},
		DescriptionSegment:            Segment{Offset: 5, Key: 6},
		SimplifiedDescriptionSegment: Segment{Offset: 7, Key: 8},
	}

	if h.NameSegment.Offset != 1 || h.NameSegment.Key != 2 {
		t.Errorf("NameSegment: %+v", h.NameSegment)
	}
	if h.SimplifiedNameSegment.Offset != 3 {
		t.Errorf("SimplifiedNameSegment: %+v", h.SimplifiedNameSegment)
	}
}

func TestNameOnlyHeaderData(t *testing.T) {
	h := NameOnlyHeaderData{
		NameSegment:           Segment{Offset: 10, Key: 20},
		FirstSeparatorSegment: Segment{Offset: 30, Key: 40},
	}

	if h.NameSegment.Offset != 10 {
		t.Errorf("NameSegment: %+v", h.NameSegment)
	}
}

func TestTextPartConstants(t *testing.T) {
	if TextPartStandard != "Standard" {
		t.Errorf("TextPartStandard = %q, want %q", TextPartStandard, "Standard")
	}
	if TextPartSimplified != "Simplified" {
		t.Errorf("TextPartSimplified = %q, want %q", TextPartSimplified, "Simplified")
	}
}