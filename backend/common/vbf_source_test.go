package common

import (
	"ffxresources/testData/tempdir"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Escopo do .vbf: o overlay vive SÓ durante a chamada, resolve pelo
// accessor (nunca pelo disco) e o decodificador de reserva cobre o que o
// conjunto pré-montado não previu.

func vbfTestRoot(t *testing.T) string {
	t.Helper()
	prev := GameFilesRoot
	root := tempdir.TempDir(t)
	GameFilesRoot = root
	t.Cleanup(func() { GameFilesRoot = prev })
	return root
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// Overlay resolve dentro do escopo e some fora dele — e nunca cai no disco.
func TestWithVbfSourceReader_ResolveNoOverlayEDepoisSome(t *testing.T) {
	vbfTestRoot(t)
	rel := filepath.FromSlash("ffx_data/gamedata/ps3data/lockit/us.bin")

	err := WithVbfSourceReader(map[string][]byte{rel: []byte("do-container")}, nil, func() error {
		acc, aerr := NewFileAccessorFrom(rel, SourceVbf)
		if aerr != nil {
			t.Fatalf("accessor: %v", aerr)
		}
		if !acc.Exists {
			t.Fatal("fonte SourceVbf não enxergou o overlay")
		}
		if !acc.IsVbf() {
			t.Fatal("accessor deveria estar marcado como vindo do .vbf")
		}
		data, rerr := acc.ReadBytes()
		if rerr != nil {
			t.Fatalf("ReadBytes: %v", rerr)
		}
		if string(data) != "do-container" {
			t.Fatalf("conteúdo = %q, queria %q", data, "do-container")
		}
		if !VbfSourceActive() {
			t.Fatal("VbfSourceActive deveria ser true dentro do escopo")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithVbfSourceReader: %v", err)
	}

	if VbfSourceActive() {
		t.Fatal("escopo não foi encerrado")
	}
	acc, err := NewFileAccessorFrom(rel, SourceVbf)
	if err != nil {
		t.Fatalf("accessor fora do escopo: %v", err)
	}
	if acc.Exists {
		t.Fatal("fora do escopo a fonte SourceVbf não pode enxergar nada")
	}
	if data, rerr := acc.ReadBytes(); !errors.Is(rerr, ErrVbfMissing) {
		t.Fatalf("ReadBytes fora do escopo = %v, queria ErrVbfMissing (conteúdo %q)", rerr, data)
	}
}

// Miss do overlay cai no decodificador de reserva — mas HIT não o chama
// (o overlay é a memória-cache; a reserva só cobre o que faltou).
func TestWithVbfSourceReader_DecodificadorDeReservaSoNoMiss(t *testing.T) {
	vbfTestRoot(t)
	hits, misses := 0, 0
	overlay := map[string][]byte{"a/b.bin": []byte("cache")}
	decode := func(p string) ([]byte, bool) {
		if p == "a/b.bin" {
			hits++
		}
		misses++
		return []byte("reserva:" + p), true
	}

	err := WithVbfSourceReader(overlay, decode, func() error {
		for _, p := range []string{"a/b.bin", "c/d.bin"} {
			acc, aerr := NewFileAccessorFrom(p, SourceVbf)
			if aerr != nil {
				return aerr
			}
			if !acc.Exists {
				t.Fatalf("%s: não resolveu", p)
			}
			data, rerr := acc.ReadBytes()
			if rerr != nil {
				return rerr
			}
			want := "cache"
			if p != "a/b.bin" {
				want = "reserva:" + p
			}
			if string(data) != want {
				t.Fatalf("%s = %q, queria %q", p, data, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("escopo: %v", err)
	}
	if hits != 0 {
		t.Fatalf("overlay hit chamou a reserva %d vezes", hits)
	}
	if misses != 1 {
		t.Fatalf("reserva chamada %d vezes, queria 1", misses)
	}
}

// SourceVbfPreferred mantém mods-first: o arquivo traduzido vence o
// container; sem ele, o container.
func TestWithVbfSourceReader_PreferredModsFirst(t *testing.T) {
	root := vbfTestRoot(t)
	rel := filepath.FromSlash("ffx_data/menu/x.bin")
	writeFile(t, filepath.Join(root, ModsFolder, rel), []byte("de-mods"))
	overlay := map[string][]byte{rel: []byte("do-container")}

	err := WithVbfSourceReader(overlay, nil, func() error {
		acc, aerr := NewFileAccessorFrom(rel, SourceVbfPreferred)
		if aerr != nil {
			return aerr
		}
		data, rerr := acc.ReadBytes()
		if rerr != nil {
			return rerr
		}
		if string(data) != "de-mods" {
			t.Fatalf("mods-first violado: %q", data)
		}
		if acc.IsVbf() {
			t.Fatal("conteúdo veio do disco: não deve estar marcado como .vbf")
		}

		// Sem o arquivo em mods/, o mesmo caminho cai no container.
		if err := os.Remove(filepath.Join(root, ModsFolder, rel)); err != nil {
			t.Fatalf("removendo mods: %v", err)
		}
		acc2, aerr2 := NewFileAccessorFrom(rel, SourceVbfPreferred)
		if aerr2 != nil {
			return aerr2
		}
		data2, rerr2 := acc2.ReadBytes()
		if rerr2 != nil {
			return rerr2
		}
		if string(data2) != "do-container" {
			t.Fatalf("sem mods deveria vir do container, veio %q", data2)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("escopo: %v", err)
	}
}

// Fontes normais (data/ e preferred) continuam lendo DISCO dentro do
// escopo: o overlay só existe para quem pediu fonte de .vbf.
func TestWithVbfSourceReader_FontesNormaisIgnoramOverlay(t *testing.T) {
	root := vbfTestRoot(t)
	rel := filepath.FromSlash("ffx_data/menu/y.bin")
	writeFile(t, filepath.Join(root, rel), []byte("de-data"))

	err := WithVbfSourceReader(map[string][]byte{rel: []byte("do-container")}, nil, func() error {
		for _, src := range []FileSource{SourceData, SourcePreferred} {
			acc, aerr := NewFileAccessorFrom(rel, src)
			if aerr != nil {
				return aerr
			}
			data, rerr := acc.ReadBytes()
			if rerr != nil {
				return rerr
			}
			if string(data) != "de-data" {
				t.Fatalf("fonte %s leu %q, queria o arquivo de data/", src, data)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("escopo: %v", err)
	}
}

// Erro/panic dentro do escopo sempre desmonta o overlay.
func TestWithVbfSourceReader_DesmontaNoErro(t *testing.T) {
	vbfTestRoot(t)
	sentinel := errors.New("falhou")

	err := WithVbfSourceReader(map[string][]byte{"a/b.bin": []byte("x")}, nil, func() error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro não propagado: %v", err)
	}
	if VbfSourceActive() {
		t.Fatal("overlay ficou preso ativo após o erro")
	}
}
