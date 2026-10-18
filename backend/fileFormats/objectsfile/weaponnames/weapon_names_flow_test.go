package weaponnames_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/fileFormats/objectsfile/schema/ffx"
	"ffxresources/backend/fileFormats/objectsfile/weaponnames"
	"ffxresources/backend/formatters"
	jsonfmt "ffxresources/backend/formatters/json"
	_ "ffxresources/backend/formatters/strings"
)

/*
Comportamento das rows enriquecidas de w_name.bin: a identidade do slot é o
model_id (via ffx.WeaponModelOwnerOf), nunca a posição no array — o export
rotula o dono pelo ID e o import localiza a posição atual do personagem pelos
model_ids do próprio registro. Inclui o round-trip EXTERNO: artefato
JSON/Strings gravado, relido pelo formatter e aplicado byte-exato.
*/

// modelIDOffset devolve o offset absoluto de model_ids[elem] no chunk
// (cabeçalho V1 = 20 bytes; individualLength = 0x48; model_ids @0x38).
func modelIDOffset(chunk, elem int) int {
	return 20 + chunk*72 + 56 + 2*elem
}

func setModelID(data []byte, chunk, elem int, id uint16) {
	binary.LittleEndian.PutUint16(data[modelIDOffset(chunk, elem):], id)
}

func swapModelIDs(data []byte, chunk, a, b int) {
	from, to := modelIDOffset(chunk, a), modelIDOffset(chunk, b)
	first, second := binary.LittleEndian.Uint16(data[from:]), binary.LittleEndian.Uint16(data[to:])
	binary.LittleEndian.PutUint16(data[from:], second)
	binary.LittleEndian.PutUint16(data[to:], first)
}

// mutatedFile carrega o binário de referência com uma mutação de bytes.
func mutatedFile(t *testing.T, mutate func([]byte)) *chunkmap.File[ffx.WeaponName] {
	t.Helper()
	data := bytes.Clone(readReference(t))
	mutate(data)
	return loadReference(t, data)
}

func rowsFor(t *testing.T, file *chunkmap.File[ffx.WeaponName]) []dto.TextRow {
	t.Helper()
	rows, err := weaponnames.Rows(file, map[string]*chunkmap.File[ffx.WeaponName]{lang: file})
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	return rows
}

func findRow(rows []dto.TextRow, chunk int, name string) (dto.TextRow, bool) {
	for _, row := range rows {
		if row.Index == chunk && row.Name == name {
			return row, true
		}
	}
	return dto.TextRow{}, false
}

func characterOf(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[:i]
	}
	return name
}

// TestWeaponNameRowsFollowModelIDNotPosition troca as duas primeiras
// posições de model_ids do chunk 1 (Tidus <-> Yuna) e exige que o export
// rotule pelo ID — o texto da posição 0 sai como Yuna — e que o import por
// NOME grave na posição onde o ID do personagem está agora.
func TestWeaponNameRowsFollowModelIDNotPosition(t *testing.T) {
	original := loadReference(t, readReference(t))
	originalRows := rowsFor(t, original)

	shuffled := mutatedFile(t, func(data []byte) { swapModelIDs(data, 1, 0, 1) })
	shuffledRows := rowsFor(t, shuffled)

	yunaRow, ok := findRow(shuffledRows, 1, "Yuna.Standard")
	if !ok {
		t.Fatal("row Yuna.Standard ausente no chunk 1 embaralhado")
	}
	tidusOriginal, ok := findRow(originalRows, 1, "Tidus.Standard")
	if !ok {
		t.Fatal("row Tidus.Standard ausente no original")
	}
	if yunaRow.Text[lang] != tidusOriginal.Text[lang] {
		t.Fatalf("texto do slot 0 não seguiu o model id: %q vs %q",
			yunaRow.Text[lang], tidusOriginal.Text[lang])
	}
	if yunaRow.Character != "Yuna" {
		t.Fatalf("character = %q, esperado \"Yuna\" (o rótulo segue o model id, não a posição)", yunaRow.Character)
	}
	if yunaRow.Kind != ffx.WeaponKindWeapon {
		t.Fatalf("kind = %q, esperado %q", yunaRow.Kind, ffx.WeaponKindWeapon)
	}

	// import por nome: editar Tidus.Standard grava na posição onde 0x4001
	// está AGORA (posição 1), sem tocar na posição 0.
	edited := append([]dto.TextRow(nil), shuffledRows...)
	for i := range edited {
		if edited[i].Index == 1 && edited[i].Name == "Tidus.Standard" {
			edited[i].Text = map[string]string{lang: "Testing_weapon"}
		}
	}
	if err := weaponnames.Apply(shuffled, edited); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	fields, err := chunkmap.SegmentFields[ffx.WeaponName]()
	if err != nil {
		t.Fatalf("SegmentFields: %v", err)
	}
	std, _, err := weaponnames.Slots()
	if err != nil {
		t.Fatalf("Slots: %v", err)
	}
	exported, err := shuffled.Get(1).ExportText(lang)
	if err != nil {
		t.Fatalf("ExportText: %v", err)
	}
	byName := make(map[string]string, len(exported))
	for _, field := range exported {
		byName[field.Name] = field.Texts[lang]
	}
	if got := byName[fields[std[1]].Name]; got != "Testing_weapon" {
		t.Fatalf("%s = %q, esperado o texto editado na posição do model id do Tidus", fields[std[1]].Name, got)
	}
	if got := byName[fields[std[0]].Name]; got != yunaRow.Text[lang] {
		t.Fatalf("%s = %q, a posição 0 não deveria ter sido alterada", fields[std[0]].Name, got)
	}
}

// TestWeaponNameRowsRejectsUnknownModelID: par de bytes fora da tabela é
// erro no export — nunca fallback posicional.
func TestWeaponNameRowsRejectsUnknownModelID(t *testing.T) {
	file := mutatedFile(t, func(data []byte) { setModelID(data, 1, 2, 0x4099) })
	_, err := weaponnames.Rows(file, map[string]*chunkmap.File[ffx.WeaponName]{lang: file})
	if err == nil {
		t.Fatal("esperado erro para model id sem dono no mapa")
	}
	if !strings.Contains(err.Error(), "99 40") {
		t.Fatalf("erro não cita o par de bytes: %v", err)
	}
}

// TestWeaponNameRejectsDuplicateCharacter: dois IDs do mesmo personagem no
// mesmo registro tornam a identidade ambígua — erro no export e no import.
func TestWeaponNameRejectsDuplicateCharacter(t *testing.T) {
	mutated := mutatedFile(t, func(data []byte) { setModelID(data, 1, 1, 0x4001) })
	if _, err := weaponnames.Rows(mutated, map[string]*chunkmap.File[ffx.WeaponName]{lang: mutated}); err == nil {
		t.Fatal("esperado erro de personagem repetido no export")
	} else if !strings.Contains(err.Error(), "Tidus") {
		t.Fatalf("erro não cita o personagem: %v", err)
	}

	original := loadReference(t, readReference(t))
	rows := rowsFor(t, original)
	if err := weaponnames.Apply(mutated, rows); err == nil {
		t.Fatal("esperado erro no apply contra o arquivo com model ids repetidos")
	}
}

// TestWeaponNameApplyFillsSimplifiedFromStandard: row Simplified ausente no
// JSON significa "leitura igual ao nome" — editando só o Standard, o import
// replica o texto na leitura (semântica do TextPair).
func TestWeaponNameApplyFillsSimplifiedFromStandard(t *testing.T) {
	file := loadReference(t, readReference(t))
	rows := rowsFor(t, file)

	sorted := append([]dto.TextRow(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Index != sorted[j].Index {
			return sorted[i].Index < sorted[j].Index
		}
		return sorted[i].Name < sorted[j].Name
	})
	var target dto.TextRow
	found := false
	for _, row := range sorted {
		if strings.HasSuffix(row.Name, ".Standard") && row.Text[lang] != "" {
			target, found = row, true
			break
		}
	}
	if !found {
		t.Fatal("nenhuma row Standard com texto no binário de referência")
	}

	edited := make([]dto.TextRow, 0, len(rows))
	for _, row := range rows {
		if row.Index == target.Index && row.Name == characterOf(target.Name)+".Simplified" {
			continue // sem row Simplified no JSON: o Standard replica na leitura
		}
		if row.Index == target.Index && row.Name == target.Name {
			row.Text = map[string]string{lang: "Edited_name"}
		}
		edited = append(edited, row)
	}
	if err := weaponnames.Apply(file, edited); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	character := characterOf(target.Name)
	pos := -1
	for i, id := range file.Get(target.Index).Value().ModelIDs {
		if owner, ok := ffx.WeaponModelOwnerOf(ffx.WeaponModelKeyOf(id)); ok && owner.Character == character {
			pos = i
			break
		}
	}
	if pos < 0 {
		t.Fatalf("personagem %s ausente dos model_ids do chunk %d", character, target.Index)
	}
	_, simp, err := weaponnames.Slots()
	if err != nil {
		t.Fatalf("Slots: %v", err)
	}
	fields, err := chunkmap.SegmentFields[ffx.WeaponName]()
	if err != nil {
		t.Fatalf("SegmentFields: %v", err)
	}
	want := fields[simp[pos]].Name
	exported, err := file.Get(target.Index).ExportText(lang)
	if err != nil {
		t.Fatalf("ExportText: %v", err)
	}
	for _, field := range exported {
		if field.Name == want {
			if field.Texts[lang] != "Edited_name" {
				t.Fatalf("%s = %q, esperado a leitura replicada do Standard", want, field.Texts[lang])
			}
			return
		}
	}
	t.Fatalf("%s ausente do export — a leitura não foi preenchida", want)
}

// TestWeaponNameApplyRejectsUnknownCharacter: row de personagem que não
// existe nos model_ids do registro é erro, não silêncio.
func TestWeaponNameApplyRejectsUnknownCharacter(t *testing.T) {
	file := loadReference(t, readReference(t))
	rows := []dto.TextRow{{
		Index: 0,
		Name:  "Shinra.Standard",
		Text:  map[string]string{lang: "x"},
	}}
	if err := weaponnames.Apply(file, rows); err == nil {
		t.Fatal("esperado erro para personagem ausente dos model_ids")
	}
}

// TestWeaponNameExternalRoundTrip grava o artefato JSON+Strings em disco,
// relê o JSON pelo formatter e aplica byte-exato num arquivo recém-carregado
// — o ciclo real de export/import por arquivo. Congela também o padrão de
// chave: entry "w_name" e metadata.key = FileLayoutKey.
func TestWeaponNameExternalRoundTrip(t *testing.T) {
	file := loadReference(t, readReference(t))
	rows := rowsFor(t, file)
	collection, err := weaponnames.Collection(common.GameVersionFFX, rows)
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}

	jsonPath := filepath.Join(t.TempDir(), "w_name_all_localizations.json")
	jsonF, ok := formatters.Get("json")
	if !ok {
		t.Fatal("formatter json não registrado")
	}
	jsonPath, err = formatters.NewWriter(jsonF, collection, jsonPath, []string{lang}).Write()
	if err != nil {
		t.Fatalf("escrever JSON: %v", err)
	}
	stringsF, ok := formatters.Get("strings")
	if !ok {
		t.Fatal("formatter strings não registrado")
	}
	stringsPath := formatters.SwapExt(stringsF, jsonPath)
	if _, err := formatters.NewWriter(stringsF, collection, stringsPath, []string{lang}).Write(); err != nil {
		t.Fatalf("escrever Strings: %v", err)
	}
	if _, err := os.Stat(stringsPath); err != nil {
		t.Fatalf("Strings não foi escrito: %v", err)
	}

	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("ler JSON exportado: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		`"character": "Tidus"`,
		`"kind": "weapon"`,
		`"kind": "shield"`,
		`"name": "Tidus.Simplified"`,
		`"ffx/battle/kernel/w_name.bin"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("JSON sem %s:\n%.400s", want, text)
		}
	}
	// O model_id é resolvido dos bytes do chunk — nunca sai no artefato.
	if strings.Contains(text, "modelId") {
		t.Fatalf("JSON não deveria conter modelId:\n%.400s", text)
	}

	back, err := jsonfmt.JSONFormatter{}.Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entry, ok := back["w_name"]
	if !ok || len(entry.Rows) == 0 {
		t.Fatalf("collection sem rows: %+v", back.SortedKeys())
	}

	original, err := file.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes: %v", err)
	}
	if err := weaponnames.Apply(file, entry.Rows); err != nil {
		t.Fatalf("aplicar JSON externo: %v", err)
	}
	out, err := file.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes: %v", err)
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("o JSON exportado não aplica byte-exato (%d vs %d bytes)", len(out), len(original))
	}
}

// TestWeaponNameMeasureAcceptsOwnerRows: a medição de capacidade do import
// aplica as rows com o MESMO Apply do fluxo real — rows rotuladas pelo dono
// ("Tidus.Standard") medem sem erro. O medidor GENÉRICO não conhece o
// naming de domínio: era ele quem bloqueava a importação do w_name com
// `chunkmap: campo "Tidus.Standard" nao existe no chunk 0`.
func TestWeaponNameMeasureAcceptsOwnerRows(t *testing.T) {
	file := loadReference(t, readReference(t))
	rows := rowsFor(t, file)

	used, err := file.MeasureStringTableWith(rows, lang, weaponnames.Apply)
	if err != nil {
		t.Fatalf("MeasureStringTableWith: %v", err)
	}
	if used <= 0 {
		t.Fatalf("string table medida = %d, queria > 0", used)
	}

	// Contraste: o applier genérico só entende caminhos de campo Go
	// ("Names[0].Standard") — é o que justifica o Apply injetado.
	if _, gerr := file.MeasureStringTable(rows, lang); gerr == nil {
		t.Fatal("medidor genérico aceitou rows de dono — deveria exigir o Apply do domínio")
	} else if !strings.Contains(gerr.Error(), "Tidus.Standard") {
		t.Fatalf("erro genérico deveria citar a row de dono: %v", gerr)
	}
}
