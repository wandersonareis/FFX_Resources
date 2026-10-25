package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ffxresources/testData/tempdir"
)

// TestRecordWritesCapturaDestino: com o gravador ligado, a gravação em
// mods/ é registrada com destino e tamanho — é o que a janela de logs
// mostra por binário.
func TestRecordWritesCapturaDestino(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	target := filepath.Join(root, ModsFolder, "ffx_ps2", "ffx2", "master",
		"new_uspc", "event", "obj_ps3", "ev", "ev001", "ev001.bin")
	payload := []byte("binario de teste")

	records := RecordWrites(func() {
		if err := WriteBytesToFile(target, payload); err != nil {
			t.Fatalf("gravação em mods/ deveria funcionar: %v", err)
		}
	})

	if len(records) != 1 {
		t.Fatalf("queria 1 registro, veio %d (%+v)", len(records), records)
	}
	rec := records[0]
	if rec.Path != target {
		t.Errorf("path = %q, queria %q", rec.Path, target)
	}
	if rec.Bytes != len(payload) {
		t.Errorf("bytes = %d, queria %d", rec.Bytes, len(payload))
	}
	if rec.Err != nil {
		t.Errorf("erro inesperado: %v", rec.Err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("arquivo não gravado: %v", err)
	}
}

// TestRecordWritesRegistraAlvoBloqueado: destino recusado (data/ é somente
// leitura) entra no registro COM o erro e o caminho — a mensagem de falha
// do log precisa do destino.
func TestRecordWritesRegistraAlvoBloqueado(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	data := filepath.Join(root, "ffx_data", "gamedata", "ps3data", "ev001.bin")
	records := RecordWrites(func() {
		if err := WriteBytesToFile(data, []byte("x")); err == nil {
			t.Fatal("gravação em data/ deveria ser recusada")
		}
	})

	if len(records) != 1 {
		t.Fatalf("queria 1 registro do alvo bloqueado, veio %d", len(records))
	}
	rec := records[0]
	if rec.Path != data {
		t.Errorf("path = %q, queria %q", rec.Path, data)
	}
	if rec.Err == nil {
		t.Fatal("registro do alvo bloqueado deveria carregar o erro")
	}
	if !strings.Contains(rec.Err.Error(), data) {
		t.Errorf("erro %q deveria nomear o destino %q", rec.Err, data)
	}
}

// TestRecordWritesVazioSemGravador: fora do RecordWrites nada é observado
// (o gravador é opt-in e não muda a escrita).
func TestRecordWritesVazioSemGravador(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	target := filepath.Join(root, ModsFolder, "qualquer.bin")
	if err := WriteBytesToFile(target, []byte("y")); err != nil {
		t.Fatalf("gravação sem gravador deveria funcionar: %v", err)
	}
	if got := RecordWrites(func() {}); len(got) != 0 {
		t.Fatalf("gravador vazio deveria devolver 0 registros, veio %d", len(got))
	}
}

// TestRecordWritesAninhado: o gravador interno devolve o que foi gravado
// dentro dele e o externo continua vendo TUDO do período (inclusive o que
// o interno narrou).
func TestRecordWritesAninhado(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	inner := filepath.Join(root, ModsFolder, "inner.bin")
	outer := filepath.Join(root, ModsFolder, "outer.bin")

	var innerRecords []WriteRecord
	outerRecords := RecordWrites(func() {
		_ = WriteBytesToFile(outer, []byte("o"))
		innerRecords = RecordWrites(func() {
			_ = WriteBytesToFile(inner, []byte("i"))
		})
	})

	if len(innerRecords) != 1 || innerRecords[0].Path != inner {
		t.Fatalf("gravador interno = %+v, queria só %s", innerRecords, inner)
	}
	if len(outerRecords) != 2 {
		t.Fatalf("gravador externo = %d registros, queria 2 (outer + inner)", len(outerRecords))
	}
}

// TestRecordWritesDevolveAposPanico: um pânico dentro da operação não pode
// deixar o gravador ocupado (deveria travar os imports seguintes).
func TestRecordWritesDevolveAposPanico(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	func() {
		defer func() { _ = recover() }()
		RecordWrites(func() { panic("falha no meio do apply") })
	}()

	records := RecordWrites(func() {})
	if len(records) != 0 {
		t.Fatalf("gravador deveria estar livre, veio %+v", records)
	}
}

// TestModsRelative: tira a raiz de localização e devolve o padrão lógico —
// é o que permite UMA linha por binário em vez de uma por idioma.
func TestModsRelative(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	us := filepath.Join(root, ModsFolder,
		filepath.FromSlash(GetLocalizationRootForVersion(GameVersionFFX2, "us")),
		filepath.FromSlash("event/obj_ps3/ev/ev001/ev001.bin"))
	loc, pattern := ModsRelative(GameVersionFFX2, us)
	if loc != "us" || pattern != "event/obj_ps3/ev/ev001/ev001.bin" {
		t.Fatalf("us: loc=%q pattern=%q", loc, pattern)
	}

	jp := filepath.Join(root, ModsFolder,
		filepath.FromSlash(GetLocalizationRootForVersion(GameVersionFFX2, "jp")),
		filepath.FromSlash("event/obj_ps3/ev/ev001/ev001.bin"))
	loc, pattern = ModsRelative(GameVersionFFX2, jp)
	if loc != "jp" || pattern != "event/obj_ps3/ev/ev001/ev001.bin" {
		t.Fatalf("jp: loc=%q pattern=%q", loc, pattern)
	}

	// Raiz do jogo-pai não casa com a versão pedida (lastmiss usa ffx2): o
	// caminho volta inteiro, sem agrupar.
	ffx := filepath.Join(root, ModsFolder,
		filepath.FromSlash(GetLocalizationRootForVersion(GameVersionFFX, "us")),
		"scene1000.bin")
	loc, pattern = ModsRelative(GameVersionLastMiss, ffx)
	if loc != "" || pattern != ffx {
		t.Fatalf("fora da versão: loc=%q pattern=%q, queria o caminho inteiro", loc, pattern)
	}

	// Prefixo que não é componente inteiro (new_uspcX) não pode casa: o
	// destino voltaria agrupado debaixo do binário errado.
	quaseRoot := filepath.Join(root, ModsFolder,
		filepath.FromSlash(GetLocalizationRootForVersion(GameVersionFFX2, "us"))+"X",
		filepath.FromSlash("event/obj_ps3/ev/ev001/ev001.bin"))
	loc, pattern = ModsRelative(GameVersionFFX2, quaseRoot)
	if loc != "" || pattern != quaseRoot {
		t.Fatalf("prefixo parcial não pode casa: loc=%q pattern=%q", loc, pattern)
	}
}

// TestWriteRecordIntegracaoComApply: a gravação real de um binário pelo
// caminho do app (WriteBytesToFileAnywhere, usado pelo menu avulso) também
// é observada.
func TestWriteRecordIntegracaoComApply(t *testing.T) {
	root := tempdir.TempDir(t)
	withGameFilesRoot(t, root)

	target := filepath.Join(root, "avulso", "command.bin")
	records := RecordWrites(func() {
		if err := WriteBytesToFileAnywhere(target, []byte("abc")); err != nil {
			t.Fatalf("gravação avulsa: %v", err)
		}
	})
	if len(records) != 1 || records[0].Path != target || records[0].Bytes != 3 {
		t.Fatalf("registro avulso = %+v", records)
	}
	if records[0].Err != nil {
		t.Fatalf("erro inesperado: %v", records[0].Err)
	}
}
