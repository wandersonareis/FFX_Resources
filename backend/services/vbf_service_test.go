package services

// Teste de integraÃ§Ã£o do navegador contra os .vbf REAIS do jogo (pulado
// quando a mÃ¡quina nÃ£o tem a instalaÃ§Ã£o): cobre o caminho completo
// descoberta â†’ abertura â†’ listagem â†’ matcher, sem decodificar conteÃºdo
// (isso exige a Ã¡rvore data/ extraÃ­da, que Ã© justamente o que pode faltar).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	"ffxresources/backend/interactions"
)

const realGameDir = `D:\Steam\steamapps\common\FINAL FANTASY FFX&FFX-2 HD Remaster`

func TestVbfNavegador_ContraOsContainersReais(t *testing.T) {
	vbfFile := filepath.Join(realGameDir, "data", "FFX_Data.vbf")
	if _, err := os.Stat(vbfFile); err != nil {
		t.Skipf("container do jogo ausente: %s", vbfFile)
	}

	// O acesso ao config Ã© um singleton que tambÃ©m reassigna GameFilesRoot
	// na primeira chamada: guarda e devolve tudo no fim.
	prevRoot := common.GameFilesRoot
	cfg := interactions.NewInteractionService().FFXAppConfig()
	prevExe := cfg.GetGameExeLocation()
	common.GameFilesRoot = prevRoot
	t.Cleanup(func() {
		cfg.SetGameExeLocation(prevExe)
		common.GameFilesRoot = prevRoot
		CloseVbfArchives()
	})
	cfg.SetGameExeLocation(filepath.Join(realGameDir, "FFX.exe"))

	svc := NewMetadataService(nil)

	roots, err := svc.ListVbfRoots()
	if err != nil {
		t.Fatalf("ListVbfRoots: %v", err)
	}
	// A pasta do jogo tem os dois containers principais (e pode ter mais,
	// ex.: metamenu.vbf) â€” o que importa Ã© que sejam todos legÃ­veis.
	if len(roots) < 2 {
		t.Fatalf("achou sÃ³ %d raÃ­zes, esperava ao menos 2: %+v", len(roots), roots)
	}
	for _, r := range roots {
		if r.Entries <= 0 || r.Size <= 0 {
			t.Errorf("%s: Ã­ndice/tamanho zerado (%d, %d)", r.Name, r.Entries, r.Size)
		}
	}

	byName := map[string]dto.VbfRoot{}
	for _, r := range roots {
		byName[r.Name] = r
	}
	ffx, okFfx := byName["FFX_Data.vbf"]
	ffx2, okFfx2 := byName["FFX2_Data.vbf"]
	if !okFfx || !okFfx2 {
		t.Fatalf("raÃ­zes inesperadas: %+v", roots)
	}
	if ffx.Version != "ffx" || ffx2.Version != "ffx2" {
		t.Errorf("versÃµes derivadas erradas: %s / %s", ffx.Version, ffx2.Version)
	}

	// Raiz do container: sÃ³ diretÃ³rios, e os conhecidos estÃ£o lÃ¡.
	rootNodes, err := svc.ListVbfDir(ffx.Path, "")
	if err != nil {
		t.Fatalf("ListVbfDir(raiz): %v", err)
	}
	if len(rootNodes) == 0 {
		t.Fatal("raiz do .vbf vazia")
	}
	names := map[string]bool{}
	for _, n := range rootNodes {
		if !n.IsDir {
			t.Errorf("raiz devolveu arquivo solto: %s", n.Name)
		}
		names[n.Name] = true
	}
	for _, want := range []string{"ffx_data", "ffx_ps2"} {
		if !names[want] {
			t.Errorf("raiz sem o diretÃ³rio %s (tem: %v)", want, names)
		}
	}

	// Descida atÃ© um evento conhecido, conferindo o matcher no caminho.
	dir := "ffx_ps2/ffx/master/new_uspc/event/obj_ps3/zn"
	dirs, err := svc.ListVbfDir(ffx.Path, dir)
	if err != nil {
		t.Fatalf("ListVbfDir(%s): %v", dir, err)
	}
	var evDir string
	for _, n := range dirs {
		if n.IsDir && n.Name == "znkd1400" {
			evDir = n.Path
		}
	}
	if evDir == "" {
		t.Fatalf("pasta znkd1400 nÃ£o apareceu em %s (%d nÃ³s)", dir, len(dirs))
	}

	files, err := svc.ListVbfDir(ffx.Path, evDir)
	if err != nil {
		t.Fatalf("ListVbfDir(%s): %v", evDir, err)
	}
	if len(files) == 0 {
		t.Fatalf("pasta do evento vazia: %s", evDir)
	}
	hit := false
	for _, n := range files {
		if n.IsDir || n.Name != "znkd1400.bin" {
			continue
		}
		hit = true
		if n.Kind != KindEvents {
			t.Errorf("kind = %q, queria %q", n.Kind, KindEvents)
		}
		if n.ID != "znkd1400" {
			t.Errorf("id = %q, queria %q", n.ID, "znkd1400")
		}
		if n.Version != "ffx" {
			t.Errorf("version = %q, queria %q", n.Version, "ffx")
		}
		if n.Size == 0 {
			t.Error("tamanho do arquivo zerado")
		}
	}
	if !hit {
		t.Errorf("znkd1400.bin nÃ£o apareceu em %s", evDir)
	}

	// Um arquivo FORA do escopo continua na Ã¡rvore, sem kind â€” Ã© o gatilho
	// do aviso "nÃ£o suportado" no clique.
	off, err := svc.ListVbfDir(ffx.Path, "version_config")
	if err != nil {
		t.Fatalf("ListVbfDir(version_config): %v", err)
	}
	for _, n := range off {
		if n.Kind != "" {
			t.Errorf("version_config/%s foi marcado como kind %q", n.Name, n.Kind)
		}
	}
}

// Fase G: mods/ Ã³rfÃ£o (sem contraparte no container) gera UM alerta por
// estado; mods/ em dia nÃ£o gera nada; e arquivos de export/traduÃ§Ã£o em
// mods/ nÃ£o contam como jogo.
func TestCheckVbfSync_AvisaUmaVezPorEstado(t *testing.T) {
	vbfFile := filepath.Join(realGameDir, "data", "FFX_Data.vbf")
	if _, err := os.Stat(vbfFile); err != nil {
		t.Skipf("container do jogo ausente: %s", vbfFile)
	}

	prevRoot := common.GameFilesRoot
	cfg := interactions.NewInteractionService().FFXAppConfig()
	prevExe := cfg.GetGameExeLocation()
	common.GameFilesRoot = prevRoot
	vbfSyncMu.Lock()
	prevSeen := vbfSyncSeen
	vbfSyncSeen = map[string]string{}
	vbfSyncMu.Unlock()
	t.Cleanup(func() {
		cfg.SetGameExeLocation(prevExe)
		common.GameFilesRoot = prevRoot
		vbfSyncMu.Lock()
		vbfSyncSeen = prevSeen
		vbfSyncMu.Unlock()
		CloseVbfArchives()
	})
	cfg.SetGameExeLocation(filepath.Join(realGameDir, "FFX.exe"))

	roots, err := (&MetadataService{}).ListVbfRoots()
	if err != nil {
		t.Fatalf("ListVbfRoots: %v", err)
	}
	var ffx dto.VbfRoot
	for _, r := range roots {
		if r.Name == "FFX_Data.vbf" {
			ffx = r
		}
	}
	if ffx.Path == "" {
		t.Fatal("FFX_Data.vbf nÃ£o apareceu")
	}
	a, err := vbfArchiveFor(ffx.Path)
	if err != nil {
		t.Fatalf("abrir container: %v", err)
	}

	// Um caminho que JÃ existe no container, para o caso "em dia".
	var real string
	for _, p := range a.Paths() {
		if strings.HasSuffix(strings.ToLower(p), ".bin") {
			real = p
			break
		}
	}
	if real == "" {
		t.Fatal("nenhum .bin no container")
	}

	root := tempdir.TempDir(t)
	common.GameFilesRoot = root
	writeTreeFile(t, filepath.Join(root, common.ModsFolder, filepath.FromSlash(real)), []byte("ok"))
	writeTreeFile(t, filepath.Join(root, common.ModsFolder, "nao_existe_no_container.bin"), []byte("orfao"))
	writeTreeFile(t, filepath.Join(root, common.ModsFolder, "edits", "qualquer.json"), []byte("{}"))
	writeTreeFile(t, filepath.Join(root, common.ModsFolder, "translated", "qualquer.bin"), []byte("x"))

	fake := &fakeNotifier{}
	svc := NewMetadataService(fake)

	svc.checkVbfSync(roots)
	if len(fake.warns) != 1 {
		t.Fatalf("alertas = %d, esperava 1: %v", len(fake.warns), fake.warns)
	}
	if !strings.Contains(fake.warns[0], "sem contraparte") {
		t.Errorf("mensagem sem a frase esperada: %q", fake.warns[0])
	}
	if !strings.Contains(fake.warns[0], "nao_existe_no_container.bin") {
		t.Errorf("mensagem sem o caminho do Ã³rfÃ£o: %q", fake.warns[0])
	}
	if strings.Contains(fake.warns[0], "qualquer.json") ||
		strings.Contains(fake.warns[0], "edits/") {
		t.Errorf("artefato de export entrou no alerta: %q", fake.warns[0])
	}

	// Mesmo estado, nova listagem: nÃ£o repete o toast.
	svc.checkVbfSync(roots)
	if len(fake.warns) != 1 {
		t.Fatalf("toast repetido: %v", fake.warns)
	}

	// Estado novo (outro Ã³rfÃ£o) â†’ novo alerta.
	writeTreeFile(t, filepath.Join(root, common.ModsFolder, "outro_orfao.bin"), []byte("orfao2"))
	svc.checkVbfSync(roots)
	if len(fake.warns) != 2 {
		t.Fatalf("alerta de estado novo ausente: %v", fake.warns)
	}

	// Estado limpo â†’ sÃ³ o diagnÃ³stico, sem alerta.
	if err := os.RemoveAll(filepath.Join(root, common.ModsFolder, "nao_existe_no_container.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, common.ModsFolder, "outro_orfao.bin")); err != nil {
		t.Fatal(err)
	}
	svc.checkVbfSync(roots)
	if len(fake.warns) != 2 {
		t.Fatalf("alerta indevido com mods/ em dia: %v", fake.warns)
	}
}

// writeTreeFile cria o arquivo (e os diretÃ³rios) no caminho dado.
func writeTreeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// fakeNotifier captura os avisos emitidos pelos services.
type fakeNotifier struct {
	warns []string
	infos []string
}

func (f *fakeNotifier) Notify(_ Severity, message string) { f.infos = append(f.infos, message) }
func (f *fakeNotifier) NotifyError(err error)             { f.infos = append(f.infos, err.Error()) }
func (f *fakeNotifier) NotifyInfo(message string)         { f.infos = append(f.infos, message) }
func (f *fakeNotifier) NotifyWarn(message string)         { f.warns = append(f.warns, message) }
func (f *fakeNotifier) NotifySuccess(message string)      { f.infos = append(f.infos, message) }

// O caminho que o frontend usa ao clicar numa folha: decode SOB DEMANDA de
// um arquivo de TEXTO e de uma TEXTURA direto do container â€” em memÃ³ria,
// sem extrair nada para disco e sem escrever no .vbf.
func TestVbfEntradas_ContraOsContainersReais(t *testing.T) {
	vbfFile := filepath.Join(realGameDir, "data", "FFX_Data.vbf")
	if _, err := os.Stat(vbfFile); err != nil {
		t.Skipf("container do jogo ausente: %s", vbfFile)
	}

	prevRoot := common.GameFilesRoot
	cfg := interactions.NewInteractionService().FFXAppConfig()
	prevExe := cfg.GetGameExeLocation()
	common.GameFilesRoot = prevRoot
	t.Cleanup(func() {
		cfg.SetGameExeLocation(prevExe)
		common.GameFilesRoot = prevRoot
		CloseVbfArchives()
	})
	cfg.SetGameExeLocation(filepath.Join(realGameDir, "FFX.exe"))

	svc := NewMetadataService(nil)
	roots, err := svc.ListVbfRoots()
	if err != nil {
		t.Fatalf("ListVbfRoots: %v", err)
	}
	var ffx dto.VbfRoot
	for _, r := range roots {
		if r.Name == "FFX_Data.vbf" {
			ffx = r
		}
	}
	if ffx.Path == "" {
		t.Fatal("FFX_Data.vbf nÃ£o apareceu")
	}

	// ---- TEXTO: o evento que a navegaÃ§Ã£o do outro teste descobriu ----
	const evDir = "ffx_ps2/ffx/master/new_uspc/event/obj_ps3/zn/znkd1400"
	entry, err := svc.GetVbfTextEntry(ffx.Path, evDir+"/znkd1400.bin", "znkd1400")
	if err != nil {
		t.Fatalf("GetVbfTextEntry: %v", err)
	}
	if len(entry.Rows) == 0 {
		t.Fatal("evento sem rows")
	}
	orig, translated := 0, 0
	for _, r := range entry.Rows {
		if len(r.Original) > 0 {
			orig++
		}
		if len(r.Text) > 0 {
			translated++
		}
	}
	if orig == 0 {
		t.Fatal("coluna Original vazia: o original nÃ£o veio do container")
	}
	if translated == 0 {
		t.Fatal("nenhuma row com texto: o estado atual nÃ£o veio do container")
	}
	// O app lÃª o container, nÃ£o o escreve: a comparaÃ§Ã£o acima Ã© de leitura.

	// ---- IMAGEM: a primeira .dds.phyre que o app sabe servir ----
	a, err := vbfArchiveFor(ffx.Path)
	if err != nil {
		t.Fatalf("abrir container: %v", err)
	}
	decoded, tried := 0, 0
	for _, p := range a.Paths() {
		if tried >= 15 {
			break
		}
		if !strings.HasSuffix(strings.ToLower(p), ".dds.phyre") {
			continue
		}
		tgt, ok := matchVbfPath(filepath.Base(ffx.Path), p)
		if !ok || tgt.Kind != KindImages {
			continue
		}
		tried++
		img, ierr := svc.GetVbfImageEntry(ffx.Path, p)
		if ierr != nil {
			continue // nem toda textura do container decoda (formato distinto)
		}
		if !strings.HasPrefix(img.PNGData, "data:image/") {
			t.Errorf("%s: prÃ©-visualizaÃ§Ã£o sem data URL (%q)", p, img.PNGData)
		}
		if img.Metadata.ID == "" {
			t.Errorf("%s: metadata.id vazio", p)
		}
		if img.Width == 0 || img.Height == 0 {
			t.Errorf("%s: dimensÃµes zeradas (%dx%d)", p, img.Width, img.Height)
		}
		decoded++
		break
	}
	if decoded == 0 {
		t.Fatalf("nenhuma das %d texturas candidatas decodificou", tried)
	}
}

// A sidebar mostra UM container por versão (FFX_Data.vbf na aba FFX,
// FFX2_Data.vbf na FFX-2): os containers de conteúdo entram, os outros que
// existem na pasta do jogo — metamenu.vbf e companhia — ficam de fora.
func TestIsContentVbf_SoOsDoisDeConteudo(t *testing.T) {
	for _, name := range []string{"FFX_Data.vbf", "ffx_data.vbf", "FFX2_Data.vbf", "ffx2_data.vbf"} {
		if !isContentVbf(name) {
			t.Errorf("%s deveria entrar na sidebar", name)
		}
	}
	for _, name := range []string{"metamenu.vbf", "Metamenu.VBF", "FFX_Data.vbf.bak", "outro.vbf"} {
		if isContentVbf(name) {
			t.Errorf("%s não deveria entrar na sidebar", name)
		}
	}
}
