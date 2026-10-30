package services

import (
	"regexp"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/formatters/hash"
)

// refTextPattern reconhece uma ref de dedup ("$"+16 hex) já GRAVADA como
// texto — o sintoma do binário corrompido pelo apply (lm_accesary/command).
var refTextPattern = regexp.MustCompile(`^\$[0-9a-f]{16}$`)

// TestApplyObjectsResolvesDedupRefs: o Save de objects manda o DTO de DISPLAY
// (cópias não traduzidas colapsadas em "$hash") e o applier TEM de expandir a
// ref para o literal da def antes de gravar — sem isso o binário recebe
// "$<hash>" como texto e a view passa a exibir o hash cru (regressão do
// lm_accesary, 57 rows, e do command, 2 rows).
func TestApplyObjectsResolvesDedupRefs(t *testing.T) {
	seedFFXHelpRoot(t)
	svc := NewMetadataService(nil)

	// w_name tem rows suficientes e o schema chunkmap registrado.
	const id = "w_name"
	c, err := svc.GetCollection(KindObjects, common.GameVersionFFX, []string{id}, nil)
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	entry, ok := c[id]
	if !ok {
		t.Fatalf("%s ausente: %v", id, c.SortedKeys())
	}

	// Duas rows .Standard com texto: a primeira é a DEF, a segunda vira REF
	// dela (mesmo ponteiro do original — a forma do DTO de display). As rows
	// .Simplified ficam de fora: o builder as omite quando a leitura iguala o
	// nome, o que mascararia a conferência da row aplicada.
	var def, ref *dto.TextRow
	for i := range entry.Rows {
		r := &entry.Rows[i]
		if !strings.HasSuffix(r.Name, ".Standard") || r.Text[common.DefaultLocalization] == "" {
			continue
		}
		if def == nil {
			def = r
			continue
		}
		ref = r
		break
	}
	if def == nil || ref == nil {
		t.Skipf("%s sem duas rows .Standard com texto para montar a ref", id)
	}
	pointer := def.Hash[common.DefaultLocalization]
	if pointer == "" {
		t.Fatalf("def sem hash: %+v", *def)
	}
	ref.Hash[common.DefaultLocalization] = pointer
	ref.Text[common.DefaultLocalization] = hash.Prefix(pointer)

	payload := dto.Collection{id: {Metadata: entry.Metadata, Rows: entry.Rows}}
	if err := svc.ApplyTextCollection(KindObjects, common.GameVersionFFX, payload); err != nil {
		t.Fatalf("apply objects: %v", err)
	}

	// A partir daqui a leitura tem de vir do binário gravado em mods/.
	common.DisableMods = false
	got, err := svc.GetCollection(KindObjects, common.GameVersionFFX, []string{id}, nil)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(got[id].Rows) != len(entry.Rows) {
		t.Fatalf("rows após o apply: %d, queria %d (o apply não pode perder row)",
			len(got[id].Rows), len(entry.Rows))
	}
	for _, row := range got[id].Rows {
		text := row.Text[common.DefaultLocalization]
		if refTextPattern.MatchString(text) {
			t.Fatalf("apply gravou a ref como texto na row %d (%s): %q", row.Index, row.Name, text)
		}
	}
	// A row da ref recebeu o literal da def (a expansão aconteceu de fato).
	var refRow dto.TextRow
	for _, row := range got[id].Rows {
		if row.Index == ref.Index && row.Name == ref.Name {
			refRow = row
			break
		}
	}
	if refRow.Text[common.DefaultLocalization] != def.Text[common.DefaultLocalization] {
		t.Fatalf("ref deveria virar o literal da def (%q), veio %q",
			def.Text[common.DefaultLocalization], refRow.Text[common.DefaultLocalization])
	}
}
