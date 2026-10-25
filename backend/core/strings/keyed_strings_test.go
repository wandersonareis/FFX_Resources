package strings

import (
	"bytes"
	"testing"

	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/datastore"
	"ffxresources/backend/models"
)

// testCharset é exclusivo destes testes.
const testCharset = "testzz"

func seedTestCharset() {
	// Map all printable ASCII characters (0x20-0x7E) plus common control chars
	b2c := make(map[uint]rune)
	for i := 0x20; i <= 0x7E; i++ {
		b2c[uint(i)] = rune(i)
	}
	// Add some common control characters
	b2c[0x00] = '\x00'
	b2c[0x0A] = '\n'
	b2c[0x0D] = '\r'
	b2c[0x09] = '\t'

	c2b := map[rune]uint{}
	for k, v := range b2c {
		c2b[v] = k
	}
	// Seed both testCharset and "us" (default localization)
	ffxencoding.SetCharMap(common.GameVersionFFX, testCharset, b2c, c2b)
	ffxencoding.SetCharMap(common.GameVersionFFX2, testCharset, b2c, c2b)
	ffxencoding.SetCharMap(common.GameVersionFFX, "us", b2c, c2b)
	ffxencoding.SetCharMap(common.GameVersionFFX2, "us", b2c, c2b)
}

func TestPointerStatus_String(t *testing.T) {
	seedTestCharset()
	tests := []struct {
		status PointerStatus
		want   string
	}{
		{PointerCreated, "created"},
		{PointerOK, "ok"},
		{PointerEmpty, "empty"},
		{PointerMid, "mid"},
		{PointerOOB, "oob"},
	}
	for _, tc := range tests {
		if got := tc.status.String(); got != tc.want {
			t.Errorf("PointerStatus(%d).String() = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestPointerStatus_IsBroken(t *testing.T) {
	seedTestCharset()
	tests := []struct {
		status PointerStatus
		want   bool
	}{
		{PointerCreated, false},
		{PointerOK, false},
		{PointerEmpty, false},
		{PointerMid, true},
		{PointerOOB, true},
	}
	for _, tc := range tests {
		if got := tc.status.IsBroken(); got != tc.want {
			t.Errorf("PointerStatus(%d).IsBroken() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestPointerStatusAt(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00\x00")

	tests := []struct {
		name   string
		offset int
		want   PointerStatus
	}{
		{"start of 'hello'", 0, PointerOK},
		{"middle of 'hello'", 2, PointerMid},
		{"null terminator after 'hello'", 5, PointerEmpty},
		{"start of 'world'", 6, PointerOK},
		{"middle of 'world'", 8, PointerMid},
		{"null terminator after 'world'", 11, PointerEmpty},
		{"extra null", 12, PointerEmpty},
		{"out of bounds high", 13, PointerOOB},
		{"out of bounds negative", -1, PointerOOB},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := PointerStatusAt(table, tc.offset)
			if got != tc.want {
				t.Errorf("PointerStatusAt(table, %d) = %v, want %v", tc.offset, got, tc.want)
			}
		})
	}
}

func TestNewKeyedString(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")
	seg := models.Segment{Offset: 0, Key: 1}

	ks := NewKeyedString(testCharset, seg, table, common.GameVersionFFX)
	if ks == nil {
		t.Fatal("expected KeyedString, got nil")
	}

	concrete := ks.(*KeyedString)

	if concrete.GetString() != "hello" {
		t.Errorf("GetString() = %q, want %q", concrete.GetString(), "hello")
	}

	if concrete.PointerStatus() != PointerOK {
		t.Errorf("PointerStatus() = %v, want PointerOK", concrete.PointerStatus())
	}

	// Test nil segment
	nilSeg := models.Segment{Offset: 0, Key: 0}
	ks = NewKeyedString(testCharset, nilSeg, table, common.GameVersionFFX)
	if ks != nil {
		t.Error("expected nil for zero segment")
	}

	// Test PointerEmpty
	emptySeg := models.Segment{Offset: 5, Key: 2}
	ks = NewKeyedString(testCharset, emptySeg, table, common.GameVersionFFX)
	if ks == nil {
		t.Fatal("expected KeyedString for empty segment")
	}
	concrete = ks.(*KeyedString)
	if concrete.PointerStatus() != PointerEmpty {
		t.Errorf("PointerStatus() = %v, want PointerEmpty", concrete.PointerStatus())
	}
	if concrete.GetString() != "" {
		t.Errorf("GetString() = %q, want empty", concrete.GetString())
	}

	// Test PointerOOB
	oobSeg := models.Segment{Offset: 100, Key: 3}
	ks = NewKeyedString(testCharset, oobSeg, table, common.GameVersionFFX)
	if ks == nil {
		t.Fatal("expected KeyedString for OOB segment")
	}
	concrete = ks.(*KeyedString)
	if concrete.PointerStatus() != PointerOOB {
		t.Errorf("PointerStatus() = %v, want PointerOOB", concrete.PointerStatus())
	}
}

func TestKeyedString_SetGet(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")
	seg := models.Segment{Offset: 0, Key: 1}

	ks := NewKeyedString(testCharset, seg, table, common.GameVersionFFX)
	concrete := ks.(*KeyedString)

	// Test SetString
	concrete.SetString("goodbye", testCharset)
	if concrete.GetString() != "goodbye" {
		t.Errorf("after SetString: GetString() = %q, want %q", concrete.GetString(), "goodbye")
	}
	if !concrete.Edited() {
		t.Error("Edited() should be true after SetString")
	}

	// Test SetOffset/SetKey
	concrete.SetOffset(10)
	if concrete.GetOffset() != 10 {
		t.Errorf("GetOffset() = %d, want 10", concrete.GetOffset())
	}

	concrete.SetKey(5)
	if concrete.GetKey() != 5 {
		t.Errorf("GetKey() = %d, want 5", concrete.GetKey())
	}

	// Test GetCharset/SetCharset
	if concrete.GetCharset() != testCharset {
		t.Errorf("GetCharset() = %q, want %q", concrete.GetCharset(), testCharset)
	}
	concrete.SetCharset("shift_jis")
	if concrete.GetCharset() != "shift_jis" {
		t.Errorf("GetCharset() = %q, want %q", concrete.GetCharset(), "shift_jis")
	}
}

func TestKeyedString_GetHeaderBytes(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")
	seg := models.Segment{Offset: 0x1234, Key: 0x5678}

	ks := NewKeyedString(testCharset, seg, table, common.GameVersionFFX)
	concrete := ks.(*KeyedString)

	var buf bytes.Buffer
	concrete.GetHeaderBytes(&buf)

	if buf.Len() != 4 {
		t.Fatalf("header bytes length = %d, want 4", buf.Len())
	}

	// Verify bytes match segment - use ReadSegmentAt which returns (Segment, error)
	offset, err := models.ReadSegmentAt(buf.Bytes(), 0)
	if err != nil {
		t.Fatalf("ReadSegmentAt failed: %v", err)
	}
	if offset.Offset != 0x1234 || offset.Key != 0x5678 {
		t.Errorf("header bytes: got Offset=%X Key=%X, want Offset=1234 Key=5678",
			offset.Offset, offset.Key)
	}
}

func TestRebuildKeyedStrings_EmptyList(t *testing.T) {
	seedTestCharset()
	result := RebuildKeyedStrings(nil, testCharset, common.GameVersionFFX, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(result))
	}

	result = RebuildKeyedStrings([]datastore.IGlobalKeyedString{}, testCharset, common.GameVersionFFX, []byte{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(result))
	}
}

func TestRebuildKeyedStrings_SingleString(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")
	seg := models.Segment{Offset: 0, Key: 1}
	ks := NewKeyedString(testCharset, seg, table, common.GameVersionFFX)

	result := RebuildKeyedStrings([]datastore.IGlobalKeyedString{ks}, testCharset, common.GameVersionFFX, table)
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	if !bytes.Contains(result, []byte("hello\x00")) {
		t.Errorf("result doesn't contain expected string")
	}
}

func TestRebuildKeyedStrings_Dedup(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")
	seg1 := models.Segment{Offset: 0, Key: 1}
	seg2 := models.Segment{Offset: 6, Key: 2}

	ks1 := NewKeyedString(testCharset, seg1, table, common.GameVersionFFX)
	ks2 := NewKeyedString(testCharset, seg2, table, common.GameVersionFFX)

	// Edit both to same value - need concrete type
	ks1.(*KeyedString).SetString("same", testCharset)
	ks2.(*KeyedString).SetString("same", testCharset)

	result := RebuildKeyedStrings([]datastore.IGlobalKeyedString{ks1, ks2}, testCharset, common.GameVersionFFX, table)
	if !bytes.Contains(result, []byte("same\x00")) {
		t.Errorf("result doesn't contain deduped string")
	}
}

func TestUsableStringTableBase(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")
	seg := models.Segment{Offset: 0, Key: 1}
	ks := NewKeyedString(testCharset, seg, table, common.GameVersionFFX)

	// Valid base
	base := UsableStringTableBase([]datastore.IGlobalKeyedString{ks}, table)
	if base == nil {
		t.Fatal("expected usable base")
	}
	if !bytes.Equal(base, table) {
		t.Error("base should equal original table")
	}

	// Invalid base (wrong content)
	badTable := []byte("bad\x00table\x00")
	base = UsableStringTableBase([]datastore.IGlobalKeyedString{ks}, badTable)
	if base != nil {
		t.Error("expected nil for invalid base")
	}

	// Empty candidate
	base = UsableStringTableBase([]datastore.IGlobalKeyedString{ks}, nil)
	if base != nil {
		t.Error("expected nil for nil candidate")
	}

	// Empty list
	base = UsableStringTableBase(nil, table)
	if base == nil {
		t.Fatal("expected base for empty list")
	}
}

func TestRebuildKeyedStrings_MidOOB(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")

	// MID ref - points inside "hello"
	midSeg := models.Segment{Offset: 2, Key: 1}
	midKS := NewKeyedString(testCharset, midSeg, table, common.GameVersionFFX)
	midKS.(*KeyedString).SetString("edited", testCharset)

	// OOB ref
	oobSeg := models.Segment{Offset: 100, Key: 2}
	oobKS := NewKeyedString(testCharset, oobSeg, table, common.GameVersionFFX)
	oobKS.(*KeyedString).SetString("oob_text", testCharset)

	result := RebuildKeyedStrings([]datastore.IGlobalKeyedString{midKS, oobKS}, testCharset, common.GameVersionFFX, table)
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
}

func TestReportKeyedStrings(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00")

	okSeg := models.Segment{Offset: 0, Key: 1}
	okKS := NewKeyedString(testCharset, okSeg, table, common.GameVersionFFX)

	emptySeg := models.Segment{Offset: 5, Key: 2}
	emptyKS := NewKeyedString(testCharset, emptySeg, table, common.GameVersionFFX)

	midSeg := models.Segment{Offset: 2, Key: 3}
	midKS := NewKeyedString(testCharset, midSeg, table, common.GameVersionFFX)

	oobSeg := models.Segment{Offset: 100, Key: 4}
	oobKS := NewKeyedString(testCharset, oobSeg, table, common.GameVersionFFX)

	// For PointerCreated, we need a ref created in memory (not from table)
	createdKS := NewKeyedString(testCharset, models.Segment{Offset: 0, Key: 5}, []byte{0}, common.GameVersionFFX)
	createdKS.(*KeyedString).SetString("created", testCharset)
	// Manually set status to PointerCreated (refs created in memory)
	createdKS.(*KeyedString).Status = PointerCreated

	// ReportKeyedStrings takes interface slice, we can pass concrete types
	list := []datastore.IGlobalKeyedString{okKS, emptyKS, midKS, oobKS, createdKS}

	report := ReportKeyedStrings(list)

	if report.OK != 1 {
		t.Errorf("report.OK = %d, want 1", report.OK)
	}
	if report.Empty != 1 {
		t.Errorf("report.Empty = %d, want 1", report.Empty)
	}
	if report.Mid != 1 {
		t.Errorf("report.Mid = %d, want 1", report.Mid)
	}
	if report.OOB != 1 {
		t.Errorf("report.OOB = %d, want 1", report.OOB)
	}
	if report.Created != 1 {
		t.Errorf("report.Created = %d, want 1", report.Created)
	}
	if report.Broken() != 2 {
		t.Errorf("report.Broken() = %d, want 2", report.Broken())
	}
}

func TestIndexStringTable(t *testing.T) {
	seedTestCharset()
	table := []byte("hello\x00world\x00test\x00")
	index := indexStringTable(table)

	if len(index) != 3 {
		t.Errorf("index has %d entries, want 3", len(index))
	}

	if offset, ok := index["hello"]; !ok || offset != 0 {
		t.Errorf("index['hello'] = %v, want 0", offset)
	}
	if offset, ok := index["world"]; !ok || offset != 6 {
		t.Errorf("index['world'] = %v, want 6", offset)
	}
	if offset, ok := index["test"]; !ok || offset != 12 {
		t.Errorf("index['test'] = %v, want 12", offset)
	}
}

func TestRegisterOnce(t *testing.T) {
	seedTestCharset()
	index := make(map[string]models.Offset)
	registerOnce(index, []byte("hello"), 0)
	registerOnce(index, []byte("world"), 6)
	registerOnce(index, []byte("hello"), 100)

	if index["hello"] != 0 {
		t.Errorf("registerOnce overwrote existing entry: got %d, want 0", index["hello"])
	}
	if index["world"] != 6 {
		t.Errorf("index['world'] = %d, want 6", index["world"])
	}
}

func TestBlockStarts(t *testing.T) {
	seedTestCharset()
	tail := []byte("a\x00bc\x00d\x00")
	starts := blockStarts(tail)

	expected := []int{0, 2, 5}
	for i, s := range starts {
		if i >= len(expected) || s != expected[i] {
			t.Errorf("blockStarts[%d] = %d, want %d", i, s, expected[i])
		}
	}
}

func TestSafeTailPadding(t *testing.T) {
	seedTestCharset()
	base := []byte("base\x00")
	oobOffsets := []int{4}
	tail := []byte("tail\x00")

	padding := safeTailPadding(base, oobOffsets, tail)
	if padding < 0 {
		t.Errorf("padding = %d, want >= 0", padding)
	}

	// Test with empty tail
	padding = safeTailPadding(base, oobOffsets, []byte{})
	if padding < 0 {
		t.Errorf("padding with empty tail = %d, want >= 0", padding)
	}
}

func TestOriginalBlockLen(t *testing.T) {
	seedTestCharset()
	base := []byte("hello\x00world\x00")

	len := originalBlockLen(base, 0)
	if len != 6 {
		t.Errorf("originalBlockLen(0) = %d, want 6", len)
	}

	len = originalBlockLen(base, 6)
	if len != 6 {
		t.Errorf("originalBlockLen(6) = %d, want 6", len)
	}

	// Out of bounds
	len = originalBlockLen(base, 100)
	if len != 0 {
		t.Errorf("originalBlockLen(100) = %d, want 0", len)
	}
}