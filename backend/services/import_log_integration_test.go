package services

// Teste fim-a-fim do log por binário no import de events: o artefato editado
// é aplicado e a janela de logs ganha UMA linha por binário recompilado, com
// o destino em mods/ e o status "ok" — em vez do par azul/verde que
// não dizia onde o binário foi parar.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	jsonfmt "ffxresources/backend/formatters/json"
	"ffxresources/backend/uilog"
	"ffxresources/testData/tempdir"
)

// importArtifactGravaBinarioFixture: store sintética de events + raiz
// temporária, devolvendo o serviço e o caminho do artefato editado.
func importArtifactGravaBinarioFixture(t *testing.T) (*MetadataService, string) {
	t.Helper()
	seedEventsTempRoot(t)
	clearEventsForTest(t)
	seedEventsSynthetic(common.GameVersionFFX2, "zev001", []string{
		"Primeira frase do evento",
		"Segunda frase do evento",
	})

	raw, err := NewMetadataService(nil).GetCollection(KindEvents, common.GameVersionFFX2, nil, nil)
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	entry := catArtifact(raw)["zev001"]
	entry.Rows[0].Target = "Primeira frase traduzida"

	out, err := jsonfmt.JSONFormatter{}.Marshal(dto.Collection{"zev001": entry})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	impPath := filepath.Join(tempdir.TempDir(t), "traduzido.json")
	if err := os.WriteFile(impPath, out, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return NewMetadataService(nil), impPath
}

// TestImportFileLogPorBinario: o ImportFile deixa no histórico do log uma
// linha de arquivo por binário gravado, com o destino dentro de mods/.
func TestImportFileLogPorBinario(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	svc, impPath := importArtifactGravaBinarioFixture(t)
	changed, err := svc.ImportFile(impPath, common.GameVersionFFX2)
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if changed != 1 {
		t.Fatalf("changed = %d, queria 1", changed)
	}

	var fileLines []uilog.Entry
	for _, e := range uilog.History() {
		if e.Path != "" && e.Action == "Import" {
			fileLines = append(fileLines, e)
		}
	}
	if len(fileLines) == 0 {
		t.Fatal("nenhuma linha de arquivo no log — o import precisa narrar os binários gravados")
	}

	// O binário de zev001 foi gravado nas 8 localizações: UMA linha, com o
	// destino da localização padrão (us).
	us := filepath.Join(common.GameFilesRoot, common.ModsFolder,
		filepath.FromSlash(common.GetLocalizationRootForVersion(common.GameVersionFFX2, common.DefaultLocalization)),
		filepath.FromSlash("event/obj_ps3/ze/zev001/zev001.bin"))
	var found bool
	for _, e := range fileLines {
		if e.Path == us {
			found = true
			if e.Status != uilog.StatusOk {
				t.Errorf("status = %q, queria %s", e.Status, uilog.StatusOk)
			}
			// Detalhe agora é "importado de um json" (padrão export)
			if !strings.Contains(e.Message, "importado de um json") {
				t.Errorf("detalhe %q deveria ser 'importado de um json'", e.Message)
			}
		}
		// Toda linha de arquivo do import aponta para dentro de mods/.
		if !strings.Contains(e.Path, string(filepath.Separator)+"mods"+string(filepath.Separator)) {
			t.Errorf("linha %q deveria apontar para mods/", e.Path)
		}
	}
	if !found {
		t.Fatalf("destino do 'us' não apareceu no log: %s", us)
	}

	// O binário existe de fato onde a linha disse.
	if _, serr := os.Stat(us); serr != nil {
		t.Fatalf("binário não gravado em %s: %v", us, serr)
	}
}

// TestImportFileLogFalhaDeGravacao: destino impossível de sobrescrever
// (arquivo somente-leitura já no lugar) deixa a linha "falha" com o
// destino no motivo — o caso de "permissão" que o usuário pediu para
// enxergar no log.
func TestImportFileLogFalhaDeGravacao(t *testing.T) {
	uilog.Clear()
	t.Cleanup(uilog.Clear)

	svc, impPath := importArtifactGravaBinarioFixture(t)

	// Pré-cria os destinos do binário (uma por localização) como
	// somente-leitura: a gravação do apply falha com acesso negado e o
	// destino continua sendo o de sempre.
	us := filepath.Join(common.GameFilesRoot, common.ModsFolder,
		filepath.FromSlash(common.GetLocalizationRootForVersion(common.GameVersionFFX2, common.DefaultLocalization)),
		filepath.FromSlash("event/obj_ps3/ze/zev001/zev001.bin"))
	for _, loc := range common.SupportedLanguageCodes() {
		dst := filepath.Join(common.GameFilesRoot, common.ModsFolder,
			filepath.FromSlash(common.GetLocalizationRootForVersion(common.GameVersionFFX2, loc)),
			filepath.FromSlash("event/obj_ps3/ze/zev001/zev001.bin"))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, []byte("binario bloqueado"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", dst, err)
		}
		// Somente-leitura para o app; 0o600 no cleanup para o TempDir apagar.
		if err := os.Chmod(dst, 0o400); err != nil {
			t.Fatalf("chmod %s: %v", dst, err)
		}
		t.Cleanup(func() { _ = os.Chmod(dst, 0o600) })
	}

	if _, err := svc.ImportFile(impPath, common.GameVersionFFX2); err == nil {
		t.Skip("ambiente permite sobrescrever arquivo somente-leitura — na dúvida, não configura falha")
	}

	var failed bool
	for _, e := range uilog.History() {
		if e.Path != "" && e.Status == "falha" {
			failed = true
			// O destino (ou o motivo que o nomeia) precisa estar na linha.
			if !strings.Contains(e.Message, "destino ") && !strings.Contains(e.Message, e.Path) {
				t.Errorf("linha de falha %q sem destino (%s)", e.Message, e.Path)
			}
			// O caminho exibido é o do binário que tentou gravar.
			if !strings.HasSuffix(e.Path, filepath.Join("ze", "zev001", "zev001.bin")) {
				t.Errorf("path = %q, queria o destino de zev001.bin", e.Path)
			}
		}
	}
	if !failed {
		t.Fatal("nenhuma linha de falha no log — o import deveria narrar o binário que não gravou")
	}
	_ = us
}
