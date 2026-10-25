package chunkmap

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/core/converter"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/models"
)

type paddedTextChunk struct {
	Text models.TextRef
}

type oversizedTextChunk struct {
	Bytes [9]byte
}

func TestReadFileBytesResolvesPatternAgainstGameFilesRoot(t *testing.T) {
	root := tempdir.TempDir(t)
	previousRoot, previousDisableMods := common.GameFilesRoot, common.DisableMods
	common.GameFilesRoot = root
	common.DisableMods = false
	t.Cleanup(func() {
		common.GameFilesRoot = previousRoot
		common.DisableMods = previousDisableMods
	})

	pattern := filepath.Join("battle", "kernel", "command.bin")
	localized := filepath.Join(common.GetLocalizationRootForVersion(common.GameVersionFFX, common.DefaultLocalization), pattern)
	dataPath := filepath.Join(root, localized)
	modsPath := filepath.Join(root, common.ModsFolder, localized)
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(modsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modsPath, []byte("mods"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readFileBytes(common.GameVersionFFX, common.DefaultLocalization, pattern)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("mods")) {
		t.Fatalf("preferred read = %q, want mods overlay", got)
	}
	common.DisableMods = true
	got, err = readFileBytes(common.GameVersionFFX, common.DefaultLocalization, pattern)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("data")) {
		t.Fatalf("read with mods disabled = %q, want data", got)
	}
}

func TestLoadFileFromBytesPreservesTheRequestedLocalization(t *testing.T) {
	if err := ffxencoding.PrepareVersionCharsets(common.GameVersionFFX); err != nil {
		t.Fatal(err)
	}
	const language = "fr"
	encoded, err := converter.StringToBytes("A", common.LanguageCodeToCharset(language), common.GameVersionFFX)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 20+4+len(encoded)+1)
	binary.LittleEndian.PutUint16(data[10:12], 0)
	binary.LittleEndian.PutUint16(data[12:14], 4)
	binary.LittleEndian.PutUint16(data[14:16], 4)
	if err := models.WriteSegmentAt(data[20:24], 0, models.Segment{Offset: 0, Key: 1}); err != nil {
		t.Fatal(err)
	}
	copy(data[24:], encoded)

	file, err := LoadFileFromBytesForLanguage[paddedTextChunk](common.GameVersionFFX, "localized.bin", language, data)
	if err != nil {
		t.Fatal(err)
	}
	texts, err := file.ExportText(language)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || len(texts[0].Fields) != 1 || texts[0].Fields[0].Texts[language] != "A" {
		t.Fatalf("loaded text with wrong localization charset: %#v", texts)
	}
}

func TestLoadFilePreservesUnmappedChunkPadding(t *testing.T) {
	if err := ffxencoding.PrepareVersionCharsets(common.GameVersionFFX); err != nil {
		t.Fatal(err)
	}
	const individualLength = 8
	data := make([]byte, 20+individualLength+3)
	binary.LittleEndian.PutUint16(data[8:10], 0)  // min index
	binary.LittleEndian.PutUint16(data[10:12], 0) // max index
	binary.LittleEndian.PutUint16(data[12:14], individualLength)
	binary.LittleEndian.PutUint16(data[14:16], individualLength)
	if err := models.WriteSegmentAt(data[20:28], 0, models.Segment{Offset: 0, Key: 1}); err != nil {
		t.Fatal(err)
	}
	copy(data[24:28], []byte{0, 0, 0, 0})
	copy(data[28:], []byte{0})

	file, err := LoadFileFromBytes[paddedTextChunk](common.GameVersionFFX, "padded.bin", data)
	if err != nil {
		t.Fatalf("LoadFileFromBytes rejected a struct smaller than the physical chunk: %v", err)
	}
	if _, err := LoadFileFromBytes[oversizedTextChunk](common.GameVersionFFX, "oversized.bin", data); err == nil {
		t.Fatal("LoadFileFromBytes accepted a struct larger than individualLength")
	}
	if got, want := file.Objects[0].chunk.Tail, []byte{0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("chunk Tail = %v, want %v", got, want)
	}
	encodedChunk, err := file.Objects[0].chunk.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encodedChunk, data[20:28]) {
		t.Fatalf("chunk encode did not append Tail: got %v, want %v", encodedChunk, data[20:28])
	}
	if err := file.ImportText([]FileText{{Index: 0, Fields: []TextField{{Index: 0, Name: "Text.Standard", Texts: map[string]string{common.DefaultLocalization: "new text"}}}}}); err != nil {
		t.Fatal(err)
	}

	out, err := file.ToBytes()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out[24:28], []byte{0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("edited chunk did not preserve unmapped padding: got %v, want %v", got, want)
	}
	reloaded, err := LoadFileFromBytes[paddedTextChunk](common.GameVersionFFX, "padded.bin", out)
	if err != nil {
		t.Fatal(err)
	}
	texts, err := reloaded.ExportText(common.DefaultLocalization)
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 1 || texts[0].Fields[0].Texts[common.DefaultLocalization] != "new text" {
		t.Fatalf("edited text did not survive reload: %#v", texts)
	}
}
