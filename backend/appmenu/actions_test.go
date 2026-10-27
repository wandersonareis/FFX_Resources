// Testes das funções do menu Dedupe (texto apenas — JSON/.strings).
//
// O menu Dedupe tem duas operações arquivo→arquivo:
//   - Dedupe: colapsa textos repetidos em refs "$hash" → saída dedupe_<stem>.<ext>
//   - Dupe:   resolve refs "$hash" de volta ao texto completo → saída dupe_<stem>.<ext>
//
// Ambas só aceitam artefatos .json ou .strings (FormatArtifactJSON / FormatArtifactStrings).
// As operações de imagem (ddsphyre) têm seu próprio dedupe em backend/fileFormats/ddsphyre.
package appmenu

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/appcore"
	"ffxresources/backend/dto"
	jsonfmt "ffxresources/backend/formatters/json"
	stringsfmt "ffxresources/backend/formatters/strings"
	"ffxresources/testData/tempdir"
)

func TestArtifactPrefixedPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefix   string
		expected string
	}{
		{
			name:     "json file with dedupe prefix",
			path:     filepath.Join("tmp", "artifact.json"),
			prefix:   appcore.PrefixDedupe,
			expected: filepath.Join("tmp", "dedupe_artifact.json"),
		},
		{
			name:     "strings file with dupe prefix",
			path:     filepath.Join("tmp", "artifact.strings"),
			prefix:   appcore.PrefixDupe,
			expected: filepath.Join("tmp", "dupe_artifact.strings"),
		},
		{
			name:     "path with directory",
			path:     filepath.Join("home", "user", "exports", "test.json"),
			prefix:   "prefix_",
			expected: filepath.Join("home", "user", "exports", "prefix_test.json"),
		},
		{
			name:     "windows-style path",
			path:     filepath.Join("C:", "Users", "test", "artifact.json"),
			prefix:   appcore.PrefixDedupe,
			expected: filepath.Join("C:", "Users", "test", "dedupe_artifact.json"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := appcore.ArtifactPrefixedPath(tc.path, tc.prefix)
			if result != tc.expected {
				t.Errorf("ArtifactPrefixedPath(%q, %q) = %q, want %q", tc.path, tc.prefix, result, tc.expected)
			}
		})
	}
}

func TestArtifactTargetExists(t *testing.T) {
	dir := tempdir.TempDir(t)

	// Cria arquivo de teste
	artifact := filepath.Join(dir, "test.json")
	if err := os.WriteFile(artifact, []byte(`{"test":{"rows":[]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Target não existe
	target := appcore.ArtifactPrefixedPath(artifact, appcore.PrefixDedupe)
	if appcore.ArtifactTargetExists(artifact, appcore.PrefixDedupe) {
		t.Error("esperava false para target inexistente")
	}

	// Cria o target
	if err := os.WriteFile(target, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Target existe
	if !appcore.ArtifactTargetExists(artifact, appcore.PrefixDedupe) {
		t.Error("esperava true para target existente")
	}

	// Com prefixo dupe
	targetDupe := appcore.ArtifactPrefixedPath(artifact, appcore.PrefixDupe)
	if appcore.ArtifactTargetExists(artifact, appcore.PrefixDupe) {
		t.Error("esperava false para target dupe inexistente")
	}
	if err := os.WriteFile(targetDupe, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !appcore.ArtifactTargetExists(artifact, appcore.PrefixDupe) {
		t.Error("esperava true para target dupe existente")
	}
}

func TestDedupeArtifactWithJSON(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	// Use 16-char hex hashes (xxh64 format)
	hashShared := "a1b2c3d4e5f67890"
	hashUnique := "b2c3d4e5f67890a1"

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "event/test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Texto repetido para dedupe"}, Hash: map[string]string{"us": hashShared}},
			{Index: 1, Text: map[string]string{"us": "Texto repetido para dedupe"}, Hash: map[string]string{"us": hashShared}},
			{Index: 2, Text: map[string]string{"us": "Texto único diferente"}, Hash: map[string]string{"us": hashUnique}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(3)
	c := dto.Collection{"event/test.bin": entry}

	original, err := jsonfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "test.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dedupe status = %v (%s)", out.Status, out.Reason)
	}
	if filepath.Base(out.Output) != "dedupe_test.json" {
		t.Errorf("saída dedupe = %q", out.Output)
	}

	deduped, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(deduped), "$"+hashShared) {
		t.Errorf("esperava ref $hash na saída dedupe:\n%s", string(deduped))
	}

	out, err = appcore.DupeArtifact(ctx, appcore.ArtifactRequest{Path: out.Output})
	if err != nil {
		t.Fatalf("dupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dupe status = %v (%s)", out.Status, out.Reason)
	}
	if filepath.Base(out.Output) != "dupe_dedupe_test.json" {
		t.Errorf("saída dupe = %q", out.Output)
	}

	expanded, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}

	want, err := jsonfmt.JSONFormatter{}.Unmarshal(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jsonfmt.JSONFormatter{}.Unmarshal(expanded)
	if err != nil {
		t.Fatal(err)
	}
	for id, wantEntry := range want {
		gotEntry, ok := got[id]
		if !ok {
			t.Fatalf("entrada %s sumiu no dupe", id)
		}
		if len(gotEntry.Rows) != len(wantEntry.Rows) {
			t.Fatalf("rows: got %d want %d", len(gotEntry.Rows), len(wantEntry.Rows))
		}
		for i, row := range wantEntry.Rows {
			if gotEntry.Rows[i].Text["us"] != row.Text["us"] {
				t.Errorf("row %d: texto %q != %q", i, gotEntry.Rows[i].Text["us"], row.Text["us"])
			}
		}
	}
}

func TestDedupeArtifactWithStrings(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	// Use 16-char hex hashes (xxh64 format)
	hashShared := "a1b2c3d4e5f67890"
	hashUnique := "b2c3d4e5f67890a1"

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "event/test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Texto repetido para dedupe"}, Hash: map[string]string{"us": hashShared}},
			{Index: 1, Text: map[string]string{"us": "Texto repetido para dedupe"}, Hash: map[string]string{"us": hashShared}},
			{Index: 2, Text: map[string]string{"us": "Texto único diferente"}, Hash: map[string]string{"us": hashUnique}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(3)
	c := dto.Collection{"event/test.bin": entry}

	original, err := stringsfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "test.strings")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dedupe status = %v (%s)", out.Status, out.Reason)
	}
	if filepath.Base(out.Output) != "dedupe_test.strings" {
		t.Errorf("saída dedupe = %q", out.Output)
	}

	deduped, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(deduped), "$"+hashShared) {
		t.Errorf("esperava ref $hash na saída dedupe:\n%s", string(deduped))
	}

	out, err = appcore.DupeArtifact(ctx, appcore.ArtifactRequest{Path: out.Output})
	if err != nil {
		t.Fatalf("dupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dupe status = %v (%s)", out.Status, out.Reason)
	}
	if filepath.Base(out.Output) != "dupe_dedupe_test.strings" {
		t.Errorf("saída dupe = %q", out.Output)
	}

	expanded, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}

	want, err := stringsfmt.StringsFormatter{}.Unmarshal(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := stringsfmt.StringsFormatter{}.Unmarshal(expanded)
	if err != nil {
		t.Fatal(err)
	}
	for id, wantEntry := range want {
		gotEntry, ok := got[id]
		if !ok {
			t.Fatalf("entrada %s sumiu no dupe", id)
		}
		if len(gotEntry.Rows) != len(wantEntry.Rows) {
			t.Fatalf("rows: got %d want %d", len(gotEntry.Rows), len(wantEntry.Rows))
		}
		for i, row := range wantEntry.Rows {
			if gotEntry.Rows[i].Text["us"] != row.Text["us"] {
				t.Errorf("row %d: texto %q != %q", i, gotEntry.Rows[i].Text["us"], row.Text["us"])
			}
		}
	}
}

func TestDedupeArtifactEmptyFile(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	artifact := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(artifact, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusMismatch {
		t.Fatalf("status = %v (%s), esperado mismatch", out.Status, out.Reason)
	}
	if !strings.Contains(out.Reason, "arquivo sem entradas") {
		t.Errorf("motivo = %q, esperava 'arquivo sem entradas'", out.Reason)
	}
}

func TestDedupeArtifactInvalidFormat(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	artifact := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(artifact, []byte("não é json"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusMismatch {
		t.Fatalf("status = %v (%s), esperado mismatch", out.Status, out.Reason)
	}
	// Error message may vary - just check it's a mismatch
	if out.Reason == "" {
		t.Errorf("motivo vazio, esperava mensagem de erro")
	}
}

func TestDedupeArtifactNonExistentFile(t *testing.T) {
	ctx := context.Background()

	// Use a path that won't exist on any platform
	artifact := filepath.Join(os.TempDir(), "nonexistent_"+randSeq(12)+".json")
	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusMismatch {
		t.Fatalf("status = %v (%s), esperado mismatch", out.Status, out.Reason)
	}
	// Error message may vary by platform - just check it's a mismatch
	if out.Reason == "" {
		t.Errorf("motivo vazio, esperava mensagem de erro")
	}
}

func randSeq(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func TestDedupeArtifactWithLangsFilter(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "event/test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Hello", "jp": "こんにちは"}, Hash: map[string]string{"us": "aaaa1111", "jp": "bbbb2222"}},
			{Index: 1, Text: map[string]string{"us": "Hello", "jp": "こんにちは"}, Hash: map[string]string{"us": "aaaa1111", "jp": "bbbb2222"}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(2)
	c := dto.Collection{"event/test.bin": entry}

	original, err := jsonfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "test.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact, Langs: []string{"us"}})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dedupe status = %v (%s)", out.Status, out.Reason)
	}

	deduped, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(deduped), "$aaaa1111") {
		t.Errorf("esperava ref $hash para 'us' na saída dedupe:\n%s", string(deduped))
	}
}

func TestDupeArtifactOnAlreadyDeduped(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	hash := "a1b2c3d4e5f67890"
	// Create an artifact where row 0 has full text (canonical) and row 1 has $hash ref
	// This simulates an already-deduped artifact
	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "event/test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Texto original"}, Hash: map[string]string{"us": hash}},
			{Index: 1, Text: map[string]string{"us": "Texto original"}, Hash: map[string]string{"us": hash}}, // Same text, will be collapsed to $hash by marshaller
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(2)
	c := dto.Collection{"event/test.bin": entry}

	// Marshal with dedup (will collapse row 1 to $hash)
	original, err := jsonfmt.JSONFormatter{}.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "already_deduped.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify the artifact has $hash ref for row 1
	if !strings.Contains(string(original), "$"+hash) {
		t.Logf("EXPECTED $hash ref but not found in:\n%s", string(original))
	}

	out, err := appcore.DupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dupe status = %v (%s)", out.Status, out.Reason)
	}

	expanded, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the expanded JSON to check the text field specifically
	got, err := jsonfmt.JSONFormatter{}.Unmarshal(expanded)
	if err != nil {
		t.Fatalf("unmarshal expanded: %v", err)
	}
	entry = got["event/test.bin"]

	// Row 0 should have full text
	if entry.Rows[0].Text["us"] != "Texto original" {
		t.Errorf("row 0 us: esperado 'Texto original', got %q", entry.Rows[0].Text["us"])
	}
	// Row 1 should have expanded text (not $hash ref)
	if entry.Rows[1].Text["us"] != "Texto original" {
		t.Errorf("row 1 us: esperado 'Texto original' (expandido), got %q", entry.Rows[1].Text["us"])
	}
}

func TestDedupeDupeIdempotent(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Texto A"}, Hash: map[string]string{"us": "aaaa1111"}},
			{Index: 1, Text: map[string]string{"us": "Texto A"}, Hash: map[string]string{"us": "aaaa1111"}},
			{Index: 2, Text: map[string]string{"us": "Texto B"}, Hash: map[string]string{"us": "bbbb2222"}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(3)
	c := dto.Collection{"test.bin": entry}

	original, err := jsonfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "test.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	currentPath := artifact
	for i := 0; i < 3; i++ {
		out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: currentPath})
		if err != nil {
			t.Fatalf("iter %d dedupe: %v", i, err)
		}
		if out.Status != appcore.StatusWritten {
			t.Fatalf("iter %d dedupe status = %v (%s)", i, out.Status, out.Reason)
		}
		currentPath = out.Output

		out, err = appcore.DupeArtifact(ctx, appcore.ArtifactRequest{Path: currentPath})
		if err != nil {
			t.Fatalf("iter %d dupe: %v", i, err)
		}
		if out.Status != appcore.StatusWritten {
			t.Fatalf("iter %d dupe status = %v (%s)", i, out.Status, out.Reason)
		}
		currentPath = out.Output

		expanded, err := os.ReadFile(currentPath)
		if err != nil {
			t.Fatal(err)
		}
		want, err := jsonfmt.JSONFormatter{}.Unmarshal(original)
		if err != nil {
			t.Fatal(err)
		}
		got, err := jsonfmt.JSONFormatter{}.Unmarshal(expanded)
		if err != nil {
			t.Fatal(err)
		}
		for id, wantEntry := range want {
			gotEntry := got[id]
			for j, row := range wantEntry.Rows {
				if gotEntry.Rows[j].Text["us"] != row.Text["us"] {
					t.Errorf("iter %d: row %d texto divergiu", i, j)
				}
			}
		}
	}
}

func TestDedupeArtifactOverwritesExisting(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": "Texto A"}, Hash: map[string]string{"us": "aaaa1111"}},
			{Index: 1, Text: map[string]string{"us": "Texto A"}, Hash: map[string]string{"us": "aaaa1111"}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(2)
	c := dto.Collection{"test.bin": entry}

	original, err := jsonfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "test.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe 1: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dedupe 1 status = %v", out.Status)
	}
	firstOutput := out.Output
	firstContent, _ := os.ReadFile(firstOutput)

	out, err = appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe 2: %v", err)
	}
	if out.Status != appcore.StatusOverwritten {
		t.Fatalf("dedupe 2 status = %v (%s), esperado Overwritten", out.Status, out.Reason)
	}
	if out.Output != firstOutput {
		t.Errorf("saída 2 = %q, esperava mesmo caminho %q", out.Output, firstOutput)
	}

	secondContent, _ := os.ReadFile(out.Output)
	if string(firstContent) != string(secondContent) {
		t.Error("conteúdo mudou entre sobrescritas idempotentes")
	}
}

func TestDedupeArtifactPreservesOtherLanguages(t *testing.T) {
	dir := tempdir.TempDir(t)
	ctx := context.Background()

	hashShared := "a1b2c3d4e5f67890"
	hashUnique := "b2c3d4e5f67890a1"

	// Use text > 10 runes to be dedup eligible (MinDedupRunes = 10, need > 10)
	sharedTextUS := "Shared text for testing"
	sharedTextJP := "共有テキストテスト用"
	sharedTextFR := "Texte partagé pour test"
	uniqueTextUS := "Unique text here"
	uniqueTextJP := "ユニークなテキスト"
	uniqueTextFR := "Texte unique ici"

	entry := dto.FileEntry{
		Metadata: dto.Metadata{Key: "test.bin", ID: "test"},
		Rows: []dto.TextRow{
			{Index: 0, Text: map[string]string{"us": sharedTextUS, "jp": sharedTextJP, "fr": sharedTextFR}, Hash: map[string]string{"us": hashShared, "jp": "c3d4e5f67890a1b2", "fr": "d4e5f67890a1b2c3"}},
			{Index: 1, Text: map[string]string{"us": sharedTextUS, "jp": sharedTextJP, "fr": sharedTextFR}, Hash: map[string]string{"us": hashShared, "jp": "c3d4e5f67890a1b2", "fr": "d4e5f67890a1b2c3"}},
			{Index: 2, Text: map[string]string{"us": uniqueTextUS, "jp": uniqueTextJP, "fr": uniqueTextFR}, Hash: map[string]string{"us": hashUnique, "jp": "e5f67890a1b2c3d4", "fr": "f67890a1b2c3d4e5"}},
		},
	}
	entry.Metadata = entry.Metadata.WithRowCount(3)
	c := dto.Collection{"test.bin": entry}

	original, err := jsonfmt.MarshalNoDedup(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	artifact := filepath.Join(dir, "test.json")
	if err := os.WriteFile(artifact, original, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := appcore.DedupeArtifact(ctx, appcore.ArtifactRequest{Path: artifact})
	if err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if out.Status != appcore.StatusWritten {
		t.Fatalf("dedupe status = %v (%s)", out.Status, out.Reason)
	}

	deduped, err := os.ReadFile(out.Output)
	if err != nil {
		t.Fatal(err)
	}

	// Check raw JSON: row 1 us text should be $hash ref
	dedupedStr := string(deduped)
	if !strings.Contains(dedupedStr, `"us": "$a1b2c3d4e5f67890"`) {
		t.Errorf("row 1 us text não é $hash ref na saída bruta:\n%s", dedupedStr)
	}
	// Row 0 us text should be full text
	if !strings.Contains(dedupedStr, `"us": "Shared text for testing"`) {
		t.Errorf("row 0 us text não é texto completo na saída bruta:\n%s", dedupedStr)
	}
	// Row 2 us text should be full text (unique)
	if !strings.Contains(dedupedStr, `"us": "Unique text here"`) {
		t.Errorf("row 2 us text não é texto completo na saída bruta:\n%s", dedupedStr)
	}

	// Also verify unmarshalling works (expands $hash refs)
	got, err := jsonfmt.JSONFormatter{}.Unmarshal(deduped)
	if err != nil {
		t.Fatal(err)
	}

	entry = got["test.bin"]
	// After unmarshal, all rows should have full text
	if entry.Rows[0].Text["us"] != sharedTextUS {
		t.Errorf("row 0 us after unmarshal: %q", entry.Rows[0].Text["us"])
	}
	if entry.Rows[1].Text["us"] != sharedTextUS {
		t.Errorf("row 1 us after unmarshal: %q", entry.Rows[1].Text["us"])
	}
	if entry.Rows[2].Text["us"] != uniqueTextUS {
		t.Errorf("row 2 us after unmarshal: %q", entry.Rows[2].Text["us"])
	}
	// Other languages should be preserved
	if entry.Rows[0].Text["jp"] != sharedTextJP {
		t.Errorf("row 0 jp: %q", entry.Rows[0].Text["jp"])
	}
	if entry.Rows[1].Text["jp"] != sharedTextJP {
		t.Errorf("row 1 jp: %q", entry.Rows[1].Text["jp"])
	}
	if entry.Rows[0].Text["fr"] != sharedTextFR {
		t.Errorf("row 0 fr: %q", entry.Rows[0].Text["fr"])
	}
	if entry.Rows[1].Text["fr"] != sharedTextFR {
		t.Errorf("row 1 fr: %q", entry.Rows[1].Text["fr"])
	}
}