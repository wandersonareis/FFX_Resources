package services

// EDIÇÃO A PARTIR DO .vbf PARA help, macro, objects e lockit.
//
// O padrão é o mesmo do motor de events: o clique decodifica o arquivo, o
// objeto fica na sessão, o applier MUTA ele e o save grava em mods/. Sem
// container nos testes — o estado vem de data/ (SourceData), o mesmo tipo
// de objeto que o decode do .vbf entrega; o caminho de gravação é idêntico.
//
// O invariante coberto aqui é a promessa do app: gravar a partir do .vbf
// não toca em NADA fora de mods/ (o .vbf é fonte somente leitura e data/ é
// o original extraído) e os mods releve o texto editado.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ffxresources/backend/builders"
	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/dto"
	"ffxresources/backend/fileFormats/helpfile"
	"ffxresources/backend/fileFormats/lockit"
	"ffxresources/backend/fileFormats/objectsfile"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/interactions"
)

// seedVbfEditRoot copia os fixtures FFX para um root temporário (o mesmo
// seed de help_kind_test.go — a árvore inteira vem junto) e limpa a sessão
// de .vbf antes/depois de cada teste. Devolve o root temporário.
func seedVbfEditRoot(t *testing.T) string {
	t.Helper()
	// O store de objects resolve o game dir do config na PRIMEIRA chamada de
	// NewInteractionService e aponta GameFilesRoot para lá. Aqueço antes de
	// fixar o root temporário, senão o seed seria sobrescrito no meio do
	// apply (o load de objects acontece durante o teste).
	_ = interactions.NewInteractionService()
	seedFFXHelpRoot(t)
	if err := ffxencoding.PrepareVersionCharsets(common.GameVersionFFX); err != nil {
		t.Skipf("charsets: %v", err)
	}
	vbfSessionResetAll()
	t.Cleanup(vbfSessionResetAll)
	return common.GameFilesRoot
}

// snapshotOutsideMods mapeia rel path → sha256 de TODOS os arquivos do root
// fora de mods/: o retrato do "original intocado".
func snapshotOutsideMods(t *testing.T, root string) map[string]string {
	t.Helper()
	mods := filepath.Join(root, common.ModsFolder)
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == mods {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot de %s: %v", root, err)
	}
	return out
}

// requireNadaForaDeModsMudou falha se algum arquivo FORA de mods/ tiver
// sido criado, alterado ou removido pelo apply — é o invariante que
// bloqueia qualquer escrita no .vbf/data/.
func requireNadaForaDeModsMudou(t *testing.T, root string, antes map[string]string) {
	t.Helper()
	depois := snapshotOutsideMods(t, root)
	for rel, hash := range antes {
		got, ok := depois[rel]
		switch {
		case !ok:
			t.Errorf("arquivo fora de mods/ foi removido: %s", rel)
		case got != hash:
			t.Errorf("arquivo fora de mods/ foi alterado: %s", rel)
		}
	}
	for rel := range depois {
		if _, ok := antes[rel]; !ok {
			t.Errorf("arquivo criado fora de mods/: %s", rel)
		}
	}
}

// editPayload devolve o payload de apply com todos os campos preservados e
// UM texto trocado: mesma row (Index/Name/Hash — validação e refs veem a
// row original), Text[us] novo. Todas as rows entram no payload (macro e
// lockit reescrevem o arquivo inteiro a partir delas).
func editPayload(id string, entry dto.FileEntry, novo string) dto.Collection {
	if len(entry.Rows) == 0 {
		return dto.Collection{}
	}
	rows := make([]dto.TextRow, len(entry.Rows))
	copy(rows, entry.Rows)
	text := make(map[string]string, len(rows[0].Text))
	for k, v := range rows[0].Text {
		text[k] = v
	}
	text[common.DefaultLocalization] = novo
	rows[0].Text = text
	return dto.Collection{id: dto.FileEntry{Metadata: entry.Metadata, Rows: rows}}
}

// rowTextPorChave devolve Text[us] da row com a chave (Index:Name) dada —
// a releitura pode reordenar, então a procura é por chave.
func rowTextPorChave(t *testing.T, e dto.FileEntry, rowKey string) string {
	t.Helper()
	for _, r := range e.Rows {
		if dto.RowKey(r) == rowKey {
			return r.Text[common.DefaultLocalization]
		}
	}
	t.Fatalf("row %q ausente na releitura de %s", rowKey, e.Metadata.ID)
	return ""
}

// TestVbfSessionHelpSalvaSomenteEmMods: painel aberto na sessão → escopo só
// com ele → apply → mods/ com o texto novo, resto da árvore byte a byte.
func TestVbfSessionHelpSalvaSomenteEmMods(t *testing.T) {
	root := seedVbfEditRoot(t)
	antes := snapshotOutsideMods(t, root)

	panel := helpfile.ReadHelpPanelFrom(common.GameVersionFFX, "now_help", common.SourceData)
	if panel == nil {
		t.Fatal("painel now_help não decodificou de data/")
	}
	entry, ok := builders.BuildHelpEntryDTOFrom("now_help", common.GameVersionFFX, panel)
	if !ok || len(entry.Rows) == 0 {
		t.Fatalf("DTO do painel não montou (ok=%v rows=%d)", ok, len(entry.Rows))
	}
	vbfSessionUpsertEstado("fake.vbf",
		vbfTarget{Kind: KindHelp, ID: "now_help", Version: common.GameVersionFFX},
		entry, &vbfEstado{painel: panel})

	scope, ok := vbfSessionHelpScope("fake.vbf")
	if !ok {
		t.Fatal("escopo de help não montou")
	}
	if len(scope.Ids) != 1 || scope.Ids[0] != "now_help" {
		t.Fatalf("escopo deveria conter só o painel aberto, obtido %v", scope.Ids)
	}

	const novo = "Texto editado pelo fluxo do .vbf"
	payload := editPayload("now_help", entry, novo)
	rowKey := dto.RowKey(payload["now_help"].Rows[0])
	if err := builders.ApplyHelpDTOWithScope(common.GameVersionFFX, payload,
		[]string{"now_help"}, scope); err != nil {
		t.Fatalf("apply help: %v", err)
	}

	requireNadaForaDeModsMudou(t, root, antes)

	readBack := helpfile.ReadHelpPanelFrom(common.GameVersionFFX, "now_help", common.SourceMods)
	if readBack == nil {
		t.Fatal("painel não releve de mods/")
	}
	rb, ok := builders.BuildHelpEntryDTOFrom("now_help", common.GameVersionFFX, readBack)
	if !ok {
		t.Fatal("DTO da releitura de mods/ não montou")
	}
	if got := rowTextPorChave(t, rb, rowKey); got != novo {
		t.Fatalf("mods/ com texto %q, esperado %q", got, novo)
	}
}

// TestVbfSessionLockitSalvaSomenteEmMods: arquivo de lockit aberto na
// sessão → apply → lockit só em mods/, data/ intacto.
func TestVbfSessionLockitSalvaSomenteEmMods(t *testing.T) {
	root := seedVbfEditRoot(t)
	antes := snapshotOutsideMods(t, root)

	layouts := lockit.LayoutsForVersion(common.GameVersionFFX)
	if len(layouts) == 0 {
		t.Fatal("sem layouts de lockit em FFX")
	}
	var (
		f *lockit.LockitFile
		l lockit.Layout
	)
	for _, cand := range layouts {
		loaded, err := lockit.LoadFrom(cand, common.SourceData)
		if err != nil || loaded == nil {
			continue
		}
		f, l = loaded, cand
		break
	}
	if f == nil {
		t.Fatal("nenhum lockit decodificou de data/")
	}
	id := l.ID()
	c, err := builders.BuildLockitDTOFrom([]*lockit.LockitFile{f}, nil)
	if err != nil {
		t.Fatalf("DTO lockit: %v", err)
	}
	entry, ok := c[id]
	if !ok || len(entry.Rows) == 0 {
		t.Fatalf("lockit %s sem rows (ok=%v rows=%d)", id, ok, len(entry.Rows))
	}
	vbfSessionUpsertEstado("fake.vbf",
		vbfTarget{Kind: KindLockit, ID: id, Version: common.GameVersionFFX},
		entry, &vbfEstado{lockitFile: f})

	scope, ok := vbfSessionLockitScope("fake.vbf")
	if !ok {
		t.Fatal("escopo de lockit não montou")
	}

	const novo = "Linha de lockit editada pelo .vbf"
	payload := editPayload(id, entry, novo)
	rowKey := dto.RowKey(payload[id].Rows[0])
	if err := builders.ApplyLockitDTOWithScope(common.GameVersionFFX, payload, scope); err != nil {
		t.Fatalf("apply lockit: %v", err)
	}

	requireNadaForaDeModsMudou(t, root, antes)

	rb, err := lockit.LoadFrom(l, common.SourceMods)
	if err != nil {
		t.Fatalf("releitura do lockit em mods/: %v", err)
	}
	rc, err := builders.BuildLockitDTOFrom([]*lockit.LockitFile{rb}, nil)
	if err != nil {
		t.Fatalf("DTO da releitura: %v", err)
	}
	if got := rowTextPorChave(t, rc[id], rowKey); got != novo {
		t.Fatalf("mods/ com texto %q, esperado %q", got, novo)
	}
}

// TestVbfSessionObjectsSalvaSomenteEmMods: objects é chunkmap puro — o
// apply lê mods-first e grava em mods/ (o original de data/ nunca é
// tocado); sem estado de sessão para objects.
func TestVbfSessionObjectsSalvaSomenteEmMods(t *testing.T) {
	root := seedVbfEditRoot(t)
	antes := snapshotOutsideMods(t, root)

	const id = "command"
	key, ok := objectKeyForID(common.GameVersionFFX, id)
	if !ok {
		t.Fatalf("layout de %s não encontrado", id)
	}
	layout := objectsfile.FileLayouts[key]

	// Coleção chunkmap: mesmo fluxo do editor normal.
	loader, ok := chunkmap.SchemaRegistry[key]
	if !ok {
		t.Fatal("schema chunkmap não registrado")
	}
	c, err := loader(common.GameVersionFFX, layout.PatternPath())
	if err != nil {
		t.Fatalf("DTO objects: %v", err)
	}
	entry, ok := c[id]
	if !ok || len(entry.Rows) == 0 {
		t.Fatalf("objects %s sem rows (ok=%v rows=%d)", id, ok, len(entry.Rows))
	}

	const novo = "Texto de objects editado pelo .vbf"
	payload := editPayload(id, entry, novo)
	rowKey := dto.RowKey(payload[id].Rows[0])
	if err := applyObjectsCollection(common.GameVersionFFX, payload); err != nil {
		t.Fatalf("apply objects: %v", err)
	}

	requireNadaForaDeModsMudou(t, root, antes)

	// Releitura de mods/: o apply gravou lá. (DisableMods do seed desliga a
	// preferência do SourcePreferred — o SourceMods lê o alvo gravado.)
	loadFrom, ok := chunkmap.SchemaSourceLoaders[key]
	if !ok {
		t.Fatal("schema chunkmap source loader não registrado")
	}
	rb, err := loadFrom(common.GameVersionFFX, layout.PatternPath(), common.SourceMods)
	if err != nil {
		t.Fatalf("DTO da releitura: %v", err)
	}
	if got := rowTextPorChave(t, rb[id], rowKey); got != novo {
		t.Fatalf("mods/ com texto %q, esperado %q", got, novo)
	}
}

// TestVbfSessionMacroSalvaSomenteEmMods: o escopo de macro é o dicionário
// INTEIRO (o artefato é um por localização) — os chunks não abertos não
// podem sumir no rebuild, e o que sai vai só para mods/.
func TestVbfSessionMacroSalvaSomenteEmMods(t *testing.T) {
	root := seedVbfEditRoot(t)
	antes := snapshotOutsideMods(t, root)

	full, err := builders.BuildMacroDTOFromSource(common.GameVersionFFX, common.SourceData, nil)
	if err != nil {
		t.Fatalf("dicionário macro de data/: %v", err)
	}
	if len(full) == 0 {
		t.Fatal("dicionário macro vazio")
	}
	// Primeiro chunk com texto: chunks posicionais vazios existem na
	// posição do dicionário mas não têm row editável.
	var chunk string
	for _, key := range full.SortedKeys() {
		if len(full[key].Rows) > 0 {
			chunk = key
			break
		}
	}
	if chunk == "" {
		t.Fatal("nenhum chunk de macro tem rows")
	}
	entry := full[chunk]
	vbfSessionUpsertEstado("fake.vbf",
		vbfTarget{Kind: KindMacro, ID: chunk, Version: common.GameVersionFFX},
		entry, &vbfEstado{macro: full})

	sessFull, ok := vbfSessionMacro("fake.vbf")
	if !ok {
		t.Fatal("dicionário da sessão não montou")
	}
	if len(sessFull) != len(full) {
		t.Fatalf("escopo deveria ser o dicionário inteiro: %d ≠ %d", len(sessFull), len(full))
	}

	const novo = "Frase de macro editada pelo .vbf"
	payload := editPayload(chunk, entry, novo)
	rowKey := dto.RowKey(payload[chunk].Rows[0])
	if err := applyMacroCollection(common.GameVersionFFX, sessFull, payload, sessFull); err != nil {
		t.Fatalf("apply macro: %v", err)
	}

	requireNadaForaDeModsMudou(t, root, antes)

	rb, err := builders.BuildMacroDTOFromSource(common.GameVersionFFX, common.SourceMods, nil)
	if err != nil {
		t.Fatalf("releitura do dicionário em mods/: %v", err)
	}
	if got := rowTextPorChave(t, rb[chunk], rowKey); got != novo {
		t.Fatalf("mods/ com texto %q, esperado %q", got, novo)
	}
	// Nenhum chunk some no rebuild: o dicionário relido tem os mesmos.
	if len(rb) != len(full) {
		t.Fatalf("releitura com %d chunks, esperado %d", len(rb), len(full))
	}
}

// TestApplyVbfTextCollectionNaoTocaNoContainer: a porta de entrada do save
// .vbf recusa ANTES de qualquer coisa quando o arquivo não é um container
// válido — e o container (o arquivo inteiro, bytes e mtime) fica intacto.
// Pela ponta, o guard de escrita reforça: nenhum escritor aceita alvo .vbf.
func TestApplyVbfTextCollectionNaoTocaNoContainer(t *testing.T) {
	root := seedVbfEditRoot(t)
	vbfPath := filepath.Join(root, "FFX_Data.vbf")
	if err := os.WriteFile(vbfPath, []byte("conteudo-de-container-invalido"), 0o644); err != nil {
		t.Fatalf("criar container: %v", err)
	}
	stAntes, err := os.Stat(vbfPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	antes := snapshotOutsideMods(t, root)

	svc := &MetadataService{}
	if err := svc.ApplyVbfTextCollection(vbfPath, KindHelp, common.GameVersionFFX,
		dto.Collection{"now_help": dto.FileEntry{}}); err == nil {
		t.Fatal("apply contra container inválido deveria ser recusado")
	}

	requireNadaForaDeModsMudou(t, root, antes)
	stDepois, err := os.Stat(vbfPath)
	if err != nil {
		t.Fatalf("stat depois: %v", err)
	}
	if !stAntes.ModTime().Equal(stDepois.ModTime()) {
		t.Errorf("mtime do .vbf mudou: %s → %s", stAntes.ModTime(), stDepois.ModTime())
	}

	// Pela ponta: nenhum escritor aceita alvo .vbf (guard central).
	if err := common.WriteBytesToFile(vbfPath, []byte("x")); !errors.Is(err, common.ErrVbfImmutable) {
		t.Fatalf("WriteBytesToFile deveria recusar .vbf, obtido: %v", err)
	}
}

// TestApplyVbfCollectionInSessionSemSessaoErro: sem nada aberto no clique,
// NENHUM kind com estado de sessão passa — o apply nunca roda contra o
// store de data/ nem contra o container. Objects é exceção: chunkmap puro,
// mods-first, sem estado de sessão.
func TestApplyVbfCollectionInSessionSemSessaoErro(t *testing.T) {
	vbfSessionResetAll()
	t.Cleanup(vbfSessionResetAll)
	for _, kind := range []string{
		KindEvents, KindBattleText, KindHelp, KindMacro, KindLockit,
	} {
		if err := applyVbfCollectionInSession("fake.vbf", kind, common.GameVersionFFX, dto.Collection{}); err == nil {
			t.Errorf("kind %s: esperava erro sem estado na sessão", kind)
		}
	}
}
