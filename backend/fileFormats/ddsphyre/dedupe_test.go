package ddsphyre

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
)

// withDupTree monta, num root temporário, três texturas da versão FFX:
//
//	dup/a  → amostra original;
//	dup/b  → MESMA imagem com o namespace alterado (outro nome embutido) —
//	         o container difere, o payload é igual: é a cópia de DVD;
//	other/c → payload diferente (imagem outra).
//
// É o recorte da árvore real: 445 arquivos em hiku22, 66 imagens distintas.
func withDupTree(t *testing.T) (root string, a, b, c string, sample []byte) {
	t.Helper()
	_, sample = samplePhyre(t)

	root = tempdir.TempDir(t)
	prev := common.GameFilesRoot
	common.GameFilesRoot = root
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		InvalidateIndexes()
	})

	a = "gamedata/ps3data/dup/a"
	b = "gamedata/ps3data/dup/b"
	c = "gamedata/ps3data/other/c"

	other := append([]byte(nil), sample...)
	off, err := dataOffsetOf(other)
	if err != nil {
		t.Fatalf("dataOffsetOf: %v", err)
	}
	if off >= uint64(len(other)) {
		t.Fatalf("dataOffset %d além do arquivo (%d)", off, len(other))
	}
	// Outra imagem: payload trocado (o header/container continua o mesmo).
	other[int(off)] ^= 0xFF

	writePhyre(t, root, a, sample)
	writePhyre(t, root, b, mutateContainerName(t, append([]byte(nil), sample...)))
	writePhyre(t, root, c, other)
	return root, a, b, c, sample
}

// writePhyre grava um .dds.phyre em <root>/ffx_data/<id>.dds.phyre.
func writePhyre(t *testing.T, root, id string, raw []byte) {
	t.Helper()
	dest := filepath.Join(root, "ffx_data", filepath.FromSlash(id)+Suffix)
	if err := common.EnsurePathExists(dest); err != nil {
		t.Fatalf("criando árvore: %v", err)
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatalf("gravando %s: %v", dest, err)
	}
}

// mutateContainerName troca o caixa de uma letra dentro de uma string do
// namespace (o nome/caminho embutido no container), mantendo o payload
// intacto — é exatamente o que difere entre duas cópias idênticas.
func mutateContainerName(t *testing.T, raw []byte) []byte {
	t.Helper()
	off, err := dataOffsetOf(raw)
	if err != nil {
		t.Fatalf("dataOffsetOf: %v", err)
	}
	ns := raw[dx11HeaderSize:off]
	for i := 0; i < len(ns); i++ {
		ch := ns[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			ns[i] = ch - 0x20
			return raw
		case ch >= 'A' && ch <= 'Z':
			ns[i] = ch + 0x20
			return raw
		}
	}
	t.Fatal("nenhuma letra no namespace da amostra")
	return raw
}

func TestPayloadHashIsTheImageNotTheContainer(t *testing.T) {
	_, raw := samplePhyre(t)
	off, err := dataOffsetOf(raw)
	if err != nil {
		t.Fatalf("dataOffsetOf: %v", err)
	}
	tex, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tex.DataOffset != off {
		t.Errorf("dataOffsetOf=%d, Parse=%d", off, tex.DataOffset)
	}

	hash, size, err := PayloadHash(raw)
	if err != nil {
		t.Fatalf("PayloadHash: %v", err)
	}
	if size != int64(len(raw))-int64(off) {
		t.Errorf("payload size = %d, esperado %d", size, len(raw)-int(off))
	}

	// Container alterado (nome no namespace) → mesmo hash.
	renamed := mutateContainerName(t, append([]byte(nil), raw...))
	hashRenamed, _, err := PayloadHash(renamed)
	if err != nil {
		t.Fatalf("PayloadHash(renamed): %v", err)
	}
	if hashRenamed != hash {
		t.Error("trocar o nome no namespace mudou o hash da imagem")
	}

	// Payload alterado → hash diferente.
	edited := append([]byte(nil), raw...)
	edited[int(off)] ^= 0xFF
	hashEdited, _, err := PayloadHash(edited)
	if err != nil {
		t.Fatalf("PayloadHash(edited): %v", err)
	}
	if hashEdited == hash {
		t.Error("payload editado não mudou o hash")
	}
}

func TestPayloadHashRejectsGarbage(t *testing.T) {
	if _, _, err := PayloadHash([]byte("não é phyre")); err == nil {
		t.Error("esperava erro para arquivo que não é .dds.phyre")
	}
	_, raw := samplePhyre(t)
	if _, _, err := PayloadHash(raw[:dx11HeaderSize+1]); err == nil {
		t.Error("esperava erro para header truncado")
	}
}

func TestIndexGroupsCopiesByPayload(t *testing.T) {
	_, a, b, c, sample := withDupTree(t)

	ix, err := BuildIndex(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	copies, payload := ix.Copies(a)
	if len(copies) != 1 || copies[0].ID != b {
		t.Fatalf("cópias de a = %+v, esperado só %s", copies, b)
	}
	if copies[0].InMods || !copies[0].Identical {
		t.Errorf("cópia pristine deveria ser idêntica e sem mods/: %+v", copies[0])
	}
	off, _ := dataOffsetOf(sample)
	if want := int64(len(sample)) - int64(off); payload != want {
		t.Errorf("payload = %d, esperado %d", payload, want)
	}

	if group := ix.OriginalGroup(a); len(group) != 2 || group[0] != a || group[1] != b {
		t.Errorf("grupo original de a = %v, esperado [%s %s]", group, a, b)
	}
	if copies, _ := ix.Copies(c); len(copies) != 0 {
		t.Errorf("imagem única não deveria ter cópias: %+v", copies)
	}
	if group := ix.OriginalGroup(c); len(group) != 1 {
		t.Errorf("grupo de c = %v, esperado só ele", group)
	}
	if group := ix.OriginalGroup("gamedata/ps3data/inexistente"); group != nil {
		t.Errorf("id fora do índice devolveu %v", group)
	}
}

func TestIndexTracksModsFirst(t *testing.T) {
	root, a, b, _, sample := withDupTree(t)

	// Importa uma imagem NOVA em cima da cópia b: o original continua igual
	// (grupo de propagação intacto), mas o efetivo divergiu.
	modsRaw := append([]byte(nil), sample...)
	off, err := dataOffsetOf(modsRaw)
	if err != nil {
		t.Fatalf("dataOffsetOf: %v", err)
	}
	modsRaw[int(off)] ^= 0xFF
	modsPath := filepath.Join(root, common.ModsFolder, RelPath(common.GameVersionFFX, b))
	if err := common.EnsurePathExists(modsPath); err != nil {
		t.Fatalf("criando mods/: %v", err)
	}
	if err := os.WriteFile(modsPath, modsRaw, 0o644); err != nil {
		t.Fatalf("gravando mods/: %v", err)
	}

	ix, err := BuildIndex(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	copies, _ := ix.Copies(a)
	if len(copies) != 1 || copies[0].ID != b {
		t.Fatalf("cópias de a = %+v, esperado só %s", copies, b)
	}
	if !copies[0].InMods {
		t.Error("cópia importada deveria ter InMods=true")
	}
	if copies[0].Identical {
		t.Error("cópia com payload novo não pode ser marcada idêntica")
	}
	if group := ix.OriginalGroup(a); len(group) != 2 {
		t.Errorf("grupo original mudou com import: %v", group)
	}
}

func TestIndexForMemoizesAndInvalidates(t *testing.T) {
	withDupTree(t)

	first, err := IndexFor(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("IndexFor: %v", err)
	}
	again, err := IndexFor(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("IndexFor(2): %v", err)
	}
	if first != again {
		t.Error("IndexFor não memorizou (construiu duas vezes)")
	}

	InvalidateIndex(common.GameVersionFFX)
	fresh, err := IndexFor(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("IndexFor(após invalidar): %v", err)
	}
	if fresh == first {
		t.Error("InvalidateIndex não descartou o cache")
	}
}

func TestIndexSkipsUnreadableContainer(t *testing.T) {
	root, a, _, _, _ := withDupTree(t)
	// Um arquivo corrompido não pode derrubar o índice inteiro.
	broken := filepath.Join(root, "ffx_data", "gamedata", "ps3data", "dup", "broken"+Suffix)
	if err := common.EnsurePathExists(broken); err != nil {
		t.Fatalf("criando broken: %v", err)
	}
	if err := os.WriteFile(broken, []byte("lixo"), 0o644); err != nil {
		t.Fatalf("gravando broken: %v", err)
	}

	ix, err := BuildIndex(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if _, ok := ix.byID["gamedata/ps3data/dup/broken"]; ok {
		t.Error("container ilegível entrou no índice")
	}
	if copies, _ := ix.Copies(a); len(copies) != 1 {
		t.Errorf("índice ficou inconsistente: %+v", copies)
	}
}

// TestIndexOnRealTree roda o índice contra a árvore REAL do jogo (quando o
// repositório tem build/bin/data): as cópias da otimização do DVD precisam
// agrupar — sem agrupamento nenhuma, o hash do payload não está servindo
// para nada.
func TestIndexOnRealTree(t *testing.T) {
	root := filepath.Join("..", "..", "..", "build", "bin", "data")
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Skip("árvore build/bin/data indisponível")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	prev := common.GameFilesRoot
	common.GameFilesRoot = abs
	t.Cleanup(func() {
		common.GameFilesRoot = prev
		InvalidateIndexes()
	})

	ix, err := BuildIndex(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("BuildIndex(ffx): %v", err)
	}
	if len(ix.byID) == 0 {
		t.Skip("nenhum .dds.phyre em data/ (árvore não extraída do FFX_Data.vbf)")
	}

	groups, copies := 0, 0
	for _, group := range ix.byOrig {
		groups++
		if len(group) > 1 {
			copies += len(group) - 1
		}
	}
	t.Logf("índice ffx: %d texturas, %d imagens distintas, %d cópias agrupadas",
		len(ix.byID), groups, copies)
	if copies == 0 {
		t.Error("árvore real sem duplicatas agrupadas — hash do payload não está funcionando")
	}
	if groups >= len(ix.byID) {
		t.Errorf("grupo (%d) não ficou menor que o total de texturas (%d)", groups, len(ix.byID))
	}
}

// O pareamento é ESTRITO em data/: só entra no índice id com pristine em
// data/. Cópia que existe apenas em mods/ não agrupa (a regra 4 nem a
// mostra na árvore) — é essa premissa que mantém grupo ⊆ árvore, de onde o
// filtro de representantes do ListEntries tira o representante.
func TestIndexPairsOnlyWithinData(t *testing.T) {
	root, a, b, _, sample := withDupTree(t)

	// Cópia de a gravada SÓ em mods/ (sem original em data/).
	onlyMods := "gamedata/ps3data/dup/soemods"
	modsPath := filepath.Join(root, common.ModsFolder, RelPath(common.GameVersionFFX, onlyMods))
	if err := common.EnsurePathExists(modsPath); err != nil {
		t.Fatalf("criando mods/: %v", err)
	}
	copied := mutateContainerName(t, append([]byte(nil), sample...))
	if err := os.WriteFile(modsPath, copied, 0o644); err != nil {
		t.Fatalf("gravando %s: %v", modsPath, err)
	}

	ix, err := BuildIndex(common.GameVersionFFX)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if _, ok := ix.byID[onlyMods]; ok {
		t.Error("id só em mods/ entrou no pareamento")
	}
	group := ix.OriginalGroup(a)
	if len(group) != 2 || group[0] != a || group[1] != b {
		t.Errorf("grupo de %s = %v, esperado [%s %s] sem %s", a, group, a, b, onlyMods)
	}
	if len(ix.OriginalGroup(onlyMods)) != 0 {
		t.Error("id fora de data/ agregou cópias")
	}
	// Nem vira representante: sem grupo, sem duplicatas reportadas.
	if copies, payload := ix.Copies(onlyMods); len(copies) != 0 || payload != 0 {
		t.Errorf("Copies(%s) = %+v/%d, esperado vazio", onlyMods, copies, payload)
	}
}
