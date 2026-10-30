package builders

import (
	"fmt"
	"sort"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/formatters/hash"
	"ffxresources/backend/progress"
)

// BuildLockitDTO monta a Collection do lockit: uma entrada por versão (id =
// stem), com todos os idiomas disponíveis em cada row.
//
// As rows são agrupadas por tipo: game ocupa os índices 0..G-1 e utf8 ocupa
// G..G+U-1 (na ordem física de cada tipo). O campo Name carrega o tipo, como
// objectsfile usa name/description — o índice é, ao mesmo tempo, o separador
// entre os grupos e a posição dentro do grupo.
//
// ids vazio = todos os layouts da versão.
// r recebe o progresso da extração: um Step por arquivo lido (nil = sem
// progresso).
func BuildLockitDTO(version common.GameVersion, ids []string, r progress.Reporter) (dto.Collection, error) {
	files, err := lockitFiles(version, ids)
	if err != nil {
		return nil, err
	}
	return BuildLockitDTOFrom(files, r)
}

// BuildLockitDTOFrom monta a Collection a partir de LockitFile já carregados
// (qualquer fonte). É o caminho do ORIGINAL: lockit.LoadFrom(layout, SourceData).
// r recebe o progresso: um Step por arquivo montado.
func BuildLockitDTOFrom(files []*lockit.LockitFile, r progress.Reporter) (dto.Collection, error) {
	r = progress.OrNop(r)
	out := make(dto.Collection, len(files))
	for _, f := range files {
		r.Step(f.Layout().ID())
		rows := buildLockitRows(f)
		if len(rows) == 0 {
			continue
		}
		entry := dto.FileEntry{
			Metadata: dto.Metadata{Key: f.Layout().Key(), ID: f.Layout().ID()},
			Rows:     rows,
		}
		entry.Metadata = entry.Metadata.WithRowCount(len(rows))
		out[f.Layout().ID()] = entry
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no lockit entries with text data found")
	}
	if err := hash.ValidateNoCollision(out); err != nil {
		return nil, err
	}
	return out, nil
}

// buildLockitRows converte os registros físicos em rows do DTO, renumerando
// por tipo (game 0..G-1, utf8 G..G+U-1).
func buildLockitRows(f *lockit.LockitFile) []dto.TextRow {
	game, _ := f.IndexesByKind()
	g := len(game)
	rows := make([]dto.TextRow, 0, f.Len())
	gi, ui := 0, g
	for _, rec := range f.Records() {
		text := copyTextMap(rec.TextMap())
		var idx int
		if rec.Kind() == lockit.KindGame {
			idx = gi
			gi++
		} else {
			idx = ui
			ui++
		}
		rows = append(rows, dto.TextRow{
			Index: idx,
			Name:  rec.Kind().String(),
			Hash:  hash.Texts(text),
			Text:  text,
		})
	}
	dto.SortRows(rows)
	return rows
}

// LockitApplyScope decide DE ONDE vem o arquivo do lockit a aplicar: o
// store de data/ (mods-first) ou a sessão do .vbf (objeto decodificado do
// container). O escopo é o único ponto que conhece a fonte.
//
// Save opcional redireciona a gravação: o fluxo arquivo→arquivo do menu/CLI
// aponta o destino (imported/<basename>) em vez do padrão SaveLanguages
// (mods/). nil = gravação padrão.
type LockitApplyScope struct {
	Load func(id string) (*lockit.LockitFile, error)
	Save func(f *lockit.LockitFile) error
}

// StoreLockitScope monta o escopo do store de data/.
func StoreLockitScope(version common.GameVersion) LockitApplyScope {
	return LockitApplyScope{
		Load: func(id string) (*lockit.LockitFile, error) {
			l, ok := lockit.LayoutForID(version, id)
			if !ok {
				return nil, fmt.Errorf("unknown lockit id: %s", id)
			}
			return lockit.LoadFromStore(l)
		},
	}
}

// ApplyLockitDTO aplica as rows editadas de volta nos registros e persiste.
func ApplyLockitDTO(version common.GameVersion, c dto.Collection) error {
	return ApplyLockitDTOWithScope(version, c, StoreLockitScope(version))
}

// ApplyLockitDTOWithScope é o apply com a fonte do arquivo escolhida pelo
// chamador. A gravação é do próprio arquivo carregado (SaveLanguages →
// mods/), então escopo nenhum precisa de hook de save.
//
// O payload pode vir do DTO de DISPLAY (refs "$hash" das cópias não
// traduzidas): a ref é expandida antes de gravar — o applier escreve o texto
// da row direto, e "$hash" como texto corromperia o binário.
func ApplyLockitDTOWithScope(version common.GameVersion, c dto.Collection, scope LockitApplyScope) error {
	c = ResolveApplyRefs(c)
	for _, id := range c.SortedKeys() {
		entry := c[id]
		f, err := scope.Load(id)
		if err != nil {
			return err
		}
		if err := applyLockitEntry(f, entry); err != nil {
			return err
		}
		// Import escreve apenas a localização padrão (us), como nos demais
		// formatos; os outros idiomas são apenas referência de tradução.
		// Save do escopo (quando presente) é o destino do chamador — o
		// menu/CLI grava ao lado do binário apontado, não em mods/.
		if scope.Save != nil {
			if err := scope.Save(f); err != nil {
				return err
			}
			continue
		}
		if err := f.SaveLanguages(common.DefaultLocalization); err != nil {
			return err
		}
	}
	return nil
}

// applyLockitEntry mapeia cada row (Name + Index) de volta à posição física e
// atualiza o texto da localização padrão (us).
func applyLockitEntry(f *lockit.LockitFile, entry dto.FileEntry) error {
	game, utf8 := f.IndexesByKind()
	records := f.Records()
	for _, row := range entry.Rows {
		kind, ok := lockit.KindFromName(row.Name)
		if !ok {
			return fmt.Errorf("lockit %s: row com name inválido: %q", entry.Metadata.ID, row.Name)
		}
		var phys int
		if kind == lockit.KindGame {
			if row.Index < 0 || row.Index >= len(game) {
				return fmt.Errorf("lockit %s: índice game fora do range: %d", entry.Metadata.ID, row.Index)
			}
			phys = game[row.Index]
		} else {
			j := row.Index - len(game)
			if j < 0 || j >= len(utf8) {
				return fmt.Errorf("lockit %s: índice utf8 fora do range: %d", entry.Metadata.ID, row.Index)
			}
			phys = utf8[j]
		}
		rec := records[phys]
		if text, ok := row.Text[common.DefaultLocalization]; ok && f.HasLanguage(common.DefaultLocalization) {
			rec.SetText(common.DefaultLocalization, text)
		}
	}
	return nil
}

// lockitFiles resolve os arquivos do lockit para os ids pedidos (vazio = todos).
func lockitFiles(version common.GameVersion, ids []string) ([]*lockit.LockitFile, error) {
	if len(ids) == 0 {
		files, err := lockit.LoadAllFromStore(version)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no lockit files found for version %s", version)
		}
		return files, nil
	}
	var out []*lockit.LockitFile
	var missing []string
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		l, ok := lockit.LayoutForID(version, id)
		if !ok {
			missing = append(missing, id)
			continue
		}
		f, err := lockit.LoadFromStore(l)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("none of the %d requested lockit id(s) found: %v", len(ids), missing)
	}
	return out, nil
}

// copyTextMap copia o mapa de textos para não compartilhar estado com o
// arquivo em memória.
func copyTextMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
