package datastore

import (
	"bytes"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/core/components"
	"ffxresources/backend/models"
)

// Mock implementations of the datastore interfaces for testing

type mockGlobalKeyedString struct {
	offset  models.Offset
	key     models.Key
	header  *bytes.Buffer
	charset string
	str     string
}

func newMockGlobalKeyedString() *mockGlobalKeyedString {
	return &mockGlobalKeyedString{
		header: new(bytes.Buffer),
	}
}

func (m *mockGlobalKeyedString) GetOffset() models.Offset                 { return m.offset }
func (m *mockGlobalKeyedString) SetOffset(offset models.Offset)          { m.offset = offset }
func (m *mockGlobalKeyedString) SetKey(key models.Key)                   { m.key = key }
func (m *mockGlobalKeyedString) GetKey() models.Key                      { return m.key }
func (m *mockGlobalKeyedString) SetHeaderBytes(buf *bytes.Buffer)        { m.header = buf }
func (m *mockGlobalKeyedString) GetHeaderBytes(buf *bytes.Buffer)        { buf.Write(m.header.Bytes()) }
func (m *mockGlobalKeyedString) SetCharset(charset string)               { m.charset = charset }
func (m *mockGlobalKeyedString) GetCharset() string                      { return m.charset }
func (m *mockGlobalKeyedString) GetString() string                       { return m.str }
func (m *mockGlobalKeyedString) SetString(s, charset string)             { m.str, m.charset = s, charset }
func (m *mockGlobalKeyedString) IsEmpty() bool                           { return m.str == "" }
func (m *mockGlobalKeyedString) String() string                          { return m.str }

type mockGlobalLocalizedKeyedStringObject struct {
	localizations map[string]*mockGlobalKeyedString
	defaultLang   string
	name          string
}

func newMockGlobalLocalizedKeyedStringObject(name string) *mockGlobalLocalizedKeyedStringObject {
	return &mockGlobalLocalizedKeyedStringObject{
		name:          name,
		localizations: make(map[string]*mockGlobalKeyedString),
		defaultLang:   "us",
	}
}

func (m *mockGlobalLocalizedKeyedStringObject) ReadAndSetLocalizedContent(languageCode string, data []byte, offset models.Offset, key models.Key, version common.GameVersion) {
	ks := newMockGlobalKeyedString()
	ks.SetOffset(offset)
	ks.SetKey(key)
	ks.SetString(string(data), "utf-8")
	m.localizations[languageCode] = ks
	if m.defaultLang == "" {
		m.defaultLang = languageCode
	}
}

func (m *mockGlobalLocalizedKeyedStringObject) SetLocalizedContent(languageCode string, content IGlobalKeyedString) {
	if mc, ok := content.(*mockGlobalKeyedString); ok {
		m.localizations[languageCode] = mc
	}
}

func (m *mockGlobalLocalizedKeyedStringObject) GetLocalizedContent(languageCode string) IGlobalKeyedString {
	return m.localizations[languageCode]
}

func (m *mockGlobalLocalizedKeyedStringObject) GetLocalizedString(languageCode string) string {
	if ks, ok := m.localizations[languageCode]; ok {
		return ks.GetString()
	}
	return ""
}

func (m *mockGlobalLocalizedKeyedStringObject) GetDefaultContent() IGlobalKeyedString {
	return m.localizations[m.defaultLang]
}

func (m *mockGlobalLocalizedKeyedStringObject) GetDefaultString() string {
	if ks, ok := m.localizations[m.defaultLang]; ok {
		return ks.GetString()
	}
	return ""
}

func (m *mockGlobalLocalizedKeyedStringObject) CopyInto(other IGlobalLocalizedKeyedStringObject) {
	if mo, ok := other.(*mockGlobalLocalizedKeyedStringObject); ok {
		for lang, ks := range m.localizations {
			mo.localizations[lang] = ks
		}
		mo.defaultLang = m.defaultLang
	}
}

func (m *mockGlobalLocalizedKeyedStringObject) String() string {
	return m.name
}

type mockGlobalMacroString struct {
	str string
}

func (m *mockGlobalMacroString) GetString() string { return m.str }
func (m *mockGlobalMacroString) IsEmpty() bool     { return m.str == "" }
func (m *mockGlobalMacroString) String() string    { return m.str }

type mockGlobalLocalizedMacroStringObject struct {
	localizations map[string]*mockGlobalMacroString
}

func newMockGlobalLocalizedMacroStringObject() *mockGlobalLocalizedMacroStringObject {
	return &mockGlobalLocalizedMacroStringObject{
		localizations: make(map[string]*mockGlobalMacroString),
	}
}

func (m *mockGlobalLocalizedMacroStringObject) SetLocalizedContent(localization string, content IGlobalMacroString) {
	if mc, ok := content.(*mockGlobalMacroString); ok {
		m.localizations[localization] = mc
	}
}

func (m *mockGlobalLocalizedMacroStringObject) GetLocalizedContent(localization string) (IGlobalMacroString, bool) {
	val, ok := m.localizations[localization]
	return val, ok
}

func (m *mockGlobalLocalizedMacroStringObject) GetLocalizedString(localization string) string {
	if val, ok := m.localizations[localization]; ok {
		return val.GetString()
	}
	return ""
}

func (m *mockGlobalLocalizedMacroStringObject) GetDefaultContent() (IGlobalMacroString, bool) {
	for _, val := range m.localizations {
		return val, true
	}
	return nil, false
}

func (m *mockGlobalLocalizedMacroStringObject) CopyInto(other IGlobalLocalizedMacroStringObject) {
	if mo, ok := other.(*mockGlobalLocalizedMacroStringObject); ok {
		for lang, ms := range m.localizations {
			mo.localizations[lang] = ms
		}
	}
}

func (m *mockGlobalLocalizedMacroStringObject) String() string {
	return "mockMacro"
}

type mockEventObject struct {
	id string
}

func newMockEventObject(id string) *mockEventObject {
	return &mockEventObject{id: id}
}

func (m *mockEventObject) GetID() string { return m.id }

type mockGlobalLocalizedTextObject struct {
	keyedStrings map[string]*mockGlobalLocalizedKeyedStringObject
	name         string
}

func newMockGlobalLocalizedTextObject(name string) *mockGlobalLocalizedTextObject {
	return &mockGlobalLocalizedTextObject{
		name:         name,
		keyedStrings: make(map[string]*mockGlobalLocalizedKeyedStringObject),
	}
}

func (m *mockGlobalLocalizedTextObject) GetName(languageCode string) string          { return m.name }
func (m *mockGlobalLocalizedTextObject) GetKeyedString(title string) IGlobalLocalizedKeyedStringObject {
	if ks, ok := m.keyedStrings[title]; ok {
		return ks
	}
	return nil
}
func (m *mockGlobalLocalizedTextObject) GetLocalizedKeyedStrings(languageCode string) []IGlobalKeyedString {
	var result []IGlobalKeyedString
	for _, ks := range m.keyedStrings {
		if c := ks.GetLocalizedContent(languageCode); c != nil {
			result = append(result, c)
		}
	}
	return result
}
func (m *mockGlobalLocalizedTextObject) SetLocalizations(other IGlobalLocalizationSetter) {}
func (m *mockGlobalLocalizedTextObject) GetTextObject() IGlobalLocalizedTextObject        { return m }
func (m *mockGlobalLocalizedTextObject) GetHeaderLength() int                             { return 0 }
func (m *mockGlobalLocalizedTextObject) ToBytes(languageCode string) ([]byte, error)      { return []byte{}, nil }
func (m *mockGlobalLocalizedTextObject) ToString(languageCode string) string              { return m.name }
func (m *mockGlobalLocalizedTextObject) String() string                                   { return m.name }

func TestGlobalDataStore_Macros(t *testing.T) {
	ds := newGlobalDataStore()

	// Test GetMacro on empty store
	macro, ok := ds.GetMacro(common.GameVersionFFX, 1)
	if ok {
		t.Errorf("expected no macro, got %v", macro)
	}

	// Test SetMacro
	mockMacro := newMockGlobalLocalizedMacroStringObject()
	mockMacro.SetLocalizedContent("us", &mockGlobalMacroString{str: "test macro"})
	ds.SetMacro(common.GameVersionFFX, 1, mockMacro)

	// Test GetMacro after set
	macro, ok = ds.GetMacro(common.GameVersionFFX, 1)
	if !ok {
		t.Fatal("expected macro to exist")
	}
	if macro.String() != "mockMacro" {
		t.Errorf("expected mockMacro, got %s", macro.String())
	}

	// Test TryAddMacro - should succeed for new ID
	mockMacro2 := newMockGlobalLocalizedMacroStringObject()
	mockMacro2.SetLocalizedContent("us", &mockGlobalMacroString{str: "test macro 2"})
	added := ds.TryAddMacro(common.GameVersionFFX, 2, mockMacro2)
	if !added {
		t.Error("TryAddMacro should succeed for new ID")
	}

	// Test TryAddMacro - should fail for existing ID
	mockMacro3 := newMockGlobalLocalizedMacroStringObject()
	mockMacro3.SetLocalizedContent("us", &mockGlobalMacroString{str: "test macro 3"})
	added = ds.TryAddMacro(common.GameVersionFFX, 1, mockMacro3)
	if added {
		t.Error("TryAddMacro should fail for existing ID")
	}

	// Test GetMacros
	macros := ds.GetMacros(common.GameVersionFFX)
	if macros.Count() != 2 {
		t.Errorf("expected 2 macros, got %d", macros.Count())
	}

	// Test ClearMacros
	ds.ClearMacros(common.GameVersionFFX)
	macros = ds.GetMacros(common.GameVersionFFX)
	if macros.Count() != 0 {
		t.Errorf("expected 0 macros after clear, got %d", macros.Count())
	}
}

func TestGlobalDataStore_Events(t *testing.T) {
	ds := newGlobalDataStore()

	// Test GetEvent on empty store
	event := ds.GetEvent(common.GameVersionFFX, "test")
	if event != nil {
		t.Errorf("expected no event, got %v", event)
	}

	// Test SetEvent
	mockEvent := newMockEventObject("test_event")
	ds.SetEvent(common.GameVersionFFX, "test_event", mockEvent)

	// Test GetEvent after set
	event = ds.GetEvent(common.GameVersionFFX, "test_event")
	if event == nil {
		t.Fatal("expected event to exist")
	}
	if event.GetID() != "test_event" {
		t.Errorf("expected test_event, got %s", event.GetID())
	}

	// Test GetAllEventIDs
	ids := ds.GetAllEventIDs(common.GameVersionFFX)
	if len(ids) != 1 || ids[0] != "test_event" {
		t.Errorf("expected [test_event], got %v", ids)
	}

	// Test ClearEvents
	ds.ClearEvents(common.GameVersionFFX)
	event = ds.GetEvent(common.GameVersionFFX, "test_event")
	if event != nil {
		t.Error("expected nil event after clear")
	}
}

func TestGlobalDataStore_Objects(t *testing.T) {
	ds := newGlobalDataStore()

	// Test GetObjectStore on empty store
	store := ds.GetObjectStore(common.GameVersionFFX, "test_key")
	if store != nil && store.Len() > 0 {
		t.Errorf("expected empty store, got %d items", store.Len())
	}

	// Test SetObjectStore
	mockObj := newMockGlobalLocalizedTextObject("test object")
	list := components.NewEmptyList[IGlobalLocalizedTextObject]()
	list.Add(mockObj)
	ds.SetObjectStore(common.GameVersionFFX, "test_key", list)

	// Test GetObjectStore after set
	store = ds.GetObjectStore(common.GameVersionFFX, "test_key")
	if store == nil || store.Len() != 1 {
		t.Errorf("expected 1 object, got %d", store.Len())
	}

	// Test GetAllObjectKeys
	keys := ds.GetAllObjectKeys(common.GameVersionFFX)
	if len(keys) != 1 || keys[0] != "test_key" {
		t.Errorf("expected [test_key], got %v", keys)
	}

	// Test ClearObjects
	ds.ClearObjects(common.GameVersionFFX)
	store = ds.GetObjectStore(common.GameVersionFFX, "test_key")
	if store != nil && store.Len() > 0 {
		t.Error("expected empty store after clear")
	}
}

func TestGlobalDataStore_NormalizeGameVersion(t *testing.T) {
	ds := newGlobalDataStore()

	// Test version normalization
	tests := []struct {
		input    common.GameVersion
		expected common.GameVersion
	}{
		{common.GameVersionFFX, common.GameVersionFFX},
		{common.GameVersionEternalCalm, common.GameVersionFFX},
		{common.GameVersionFFX2, common.GameVersionFFX2},
		{common.GameVersionLastMiss, common.GameVersionFFX2},
	}

	for _, tc := range tests {
		// We can't directly test the private function, but we can test
		// that macros/events/objects are shared correctly

		// Set a macro for the input version
		mockMacro := newMockGlobalLocalizedMacroStringObject()
		mockMacro.SetLocalizedContent("us", &mockGlobalMacroString{str: "test"})
		ds.SetMacro(tc.input, 1, mockMacro)

		// Check it's accessible from the expected version
		macro, ok := ds.GetMacro(tc.expected, 1)
		if !ok {
			t.Errorf("normalize: expected macro at version %v to be accessible at %v", tc.input, tc.expected)
		}
		if macro == nil {
			t.Errorf("macro is nil for version %v", tc.expected)
		}

		// Clear for next test
		ds.ClearAllMacros()
	}
}

func TestGlobalDataStore_Concurrency(t *testing.T) {
	ds := newGlobalDataStore()

	// Test concurrent reads and writes
	done := make(chan bool, 100)

	// Writers
	for i := 0; i < 10; i++ {
		go func(idx int) {
			mockMacro := newMockGlobalLocalizedMacroStringObject()
			mockMacro.SetLocalizedContent("us", &mockGlobalMacroString{str: "macro"})
			ds.SetMacro(common.GameVersionFFX, idx, mockMacro)
			done <- true
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func() {
			ds.GetMacros(common.GameVersionFFX)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify data integrity
	macros := ds.GetMacros(common.GameVersionFFX)
	if macros.Count() < 10 {
		t.Errorf("expected at least 10 macros after concurrent test, got %d", macros.Count())
	}
}

func TestInstance_Singleton(t *testing.T) {
	// Test that Instance is a singleton
	mockMacro := newMockGlobalLocalizedMacroStringObject()
	mockMacro.SetLocalizedContent("us", &mockGlobalMacroString{str: "singleton test"})

	SetMacro(common.GameVersionFFX, 999, mockMacro)

	macro, ok := GetMacro(common.GameVersionFFX, 999)
	if !ok {
		t.Fatal("expected macro from Instance")
	}
	if macro.String() != "mockMacro" {
		t.Errorf("expected mockMacro from singleton")
	}

	// Clean up
	Instance.ClearMacros(common.GameVersionFFX)
}

func TestGlobalDataStore_ClearAll(t *testing.T) {
	ds := newGlobalDataStore()

	// Add data to all stores
	mockMacro := newMockGlobalLocalizedMacroStringObject()
	mockMacro.SetLocalizedContent("us", &mockGlobalMacroString{str: "test"})
	ds.SetMacro(common.GameVersionFFX, 1, mockMacro)

	mockEvent := newMockEventObject("event1")
	ds.SetEvent(common.GameVersionFFX, "event1", mockEvent)

	mockObj := newMockGlobalLocalizedTextObject("obj1")
	list := components.NewEmptyList[IGlobalLocalizedTextObject]()
	list.Add(mockObj)
	ds.SetObjectStore(common.GameVersionFFX, "key1", list)

	// Clear all
	ds.ClearAllMacros()
	ds.ClearAllEvents()
	ds.ClearAllObjects()

	// Verify everything is cleared
	if ds.GetMacros(common.GameVersionFFX).Count() != 0 {
		t.Error("macros not cleared")
	}
	if len(ds.GetAllEventIDs(common.GameVersionFFX)) != 0 {
		t.Error("events not cleared")
	}
	if len(ds.GetAllObjectKeys(common.GameVersionFFX)) != 0 {
		t.Error("objects not cleared")
	}
}

func TestGlobalDataStore_IsEmpty(t *testing.T) {
	ds := newGlobalDataStore()

	// Empty store
	if ds.IsEmpty(common.GameVersionFFX) {
		t.Error("empty store should return false for IsEmpty (has data = true)")
	}

	// Add macro
	mockMacro := newMockGlobalLocalizedMacroStringObject()
	mockMacro.SetLocalizedContent("us", &mockGlobalMacroString{str: "test"})
	ds.SetMacro(common.GameVersionFFX, 1, mockMacro)

	// Note: IsEmpty returns true when there IS data (name is confusing)
	if !ds.IsEmpty(common.GameVersionFFX) {
		t.Error("store with data should return true for IsEmpty")
	}

	ds.ClearMacros(common.GameVersionFFX)
	if ds.IsEmpty(common.GameVersionFFX) {
		t.Error("cleared store should return false for IsEmpty")
	}
}