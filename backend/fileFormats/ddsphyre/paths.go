package ddsphyre

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ffxresources/backend/common"
)

// RootForVersion devolve o diretório de árvore da versão: FFX usa
// ffx_data/, FFX-2 (e Last Mission, que divide a árvore) usa ffx-2_data/.
func RootForVersion(version common.GameVersion) string {
	if version == common.GameVersionFFX {
		return "ffx_data"
	}
	return "ffx-2_data"
}

// RelPath devolve o caminho do .dds.phyre relativo ao GameFilesRoot
// (data/) a partir do id da textura.
func RelPath(version common.GameVersion, id string) string {
	return filepath.Join(RootForVersion(version), filepath.FromSlash(id)+Suffix)
}

// ValidID confere se o id é um caminho de textura plausível: relativo,
// dentro de gamedata/ps3data, sem traversal nem o sufixo já embutido.
// É a trava de segurança dos bindings que recebem id da UI.
func ValidID(id string) bool {
	if id == "" {
		return false
	}
	s := strings.ReplaceAll(id, "\\", "/")
	if strings.HasPrefix(s, "/") || strings.Contains(s, "..") {
		return false
	}
	if strings.HasSuffix(s, Suffix) {
		return false
	}
	return strings.HasPrefix(s, "gamedata/ps3data/")
}

// ExportPaths devolve os caminhos dos artefatos extraídos (.dds e .png) em
// mods/images/<raiz>/<id> — o caminho interno do .vbf preservado sob a raiz
// de imagens de mods/ (mods/edits é só de texto).
func ExportPaths(version common.GameVersion, id string) (ddsPath, pngPath string) {
	base := filepath.Join(
		common.GameFilesRoot, common.ModsFolder, common.ModsImagesDir,
		RootForVersion(version), filepath.FromSlash(id),
	)
	return base + ".dds", base + ".png"
}

// treeRoot devolve a raiz varrida por árvore: <gamefiles>[ /mods]/<raiz>.
func treeRoot(version common.GameVersion, mods bool) string {
	root := RootForVersion(version)
	if mods {
		return filepath.Join(common.GameFilesRoot, common.ModsFolder, root)
	}
	return filepath.Join(common.GameFilesRoot, root)
}

// walkIDs varre uma árvore coletando ids de textura (caminho relativo sem
// o sufixo .dds.phyre, com "/"), em ordem canônica.
func walkIDs(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), Suffix) {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(strings.TrimSuffix(rel, Suffix)))
		return nil
	})
	sort.Strings(out)
	return out
}

// Scan inventaria as texturas da versão.
//
//   - ids: o que a árvore PRISTINE (data/) define — é ela que monta a árvore;
//   - onlyMods: ids que só existem em mods/ (diagnóstico, como nos demais
//     kinds);
//   - fallback: data/ não tem NENHUM .dds.phyre e o scan caiu para mods/
//     (árvore não extraída do FFX_Data.vbf) — o chamador avisa no log.
func Scan(version common.GameVersion) (ids, onlyMods []string, fallback bool, err error) {
	inData := walkIDs(treeRoot(version, false))
	if len(inData) == 0 {
		inMods := walkIDs(treeRoot(version, true))
		if len(inMods) == 0 {
			return nil, nil, false, nil
		}
		return inMods, nil, true, nil
	}

	inMods := walkIDs(treeRoot(version, true))
	modsSet := make(map[string]bool, len(inMods))
	for _, id := range inMods {
		modsSet[id] = true
	}
	for _, id := range inData {
		delete(modsSet, id)
	}
	onlyMods = make([]string, 0, len(modsSet))
	for id := range modsSet {
		onlyMods = append(onlyMods, id)
	}
	sort.Strings(onlyMods)
	return inData, onlyMods, false, nil
}

// Exists devolve a presença da textura nas duas árctores (data/ e mods/).
func Exists(version common.GameVersion, id string) (inData, inMods bool) {
	rel := RelPath(version, id)
	if acc, err := common.NewFileAccessorFrom(rel, common.SourceData); err == nil && acc.Exists {
		inData = true
	}
	if acc, err := common.NewFileAccessorFrom(rel, common.SourceMods); err == nil && acc.Exists {
		inMods = true
	}
	return inData, inMods
}

// Resolved é a imagem escolhida para exibição, na ordem de preferência
// .dds em disco → .png em disco → decode do .dds.phyre (sem gravar nada).
type Resolved struct {
	// Source é de onde veio a imagem servida: "dds", "png" ou "phyre".
	Source string
	// Format/Width/Height/MipmapCount vêm sempre do .dds.phyre (fonte).
	Format       string
	Width        uint32
	Height       uint32
	MipmapCount  uint32
	MaxMipmapLvl uint32
	// Texture é o container parseado (nil só em erro).
	Texture *Texture
	// PNG é a pré-visualização (sempre preenchida).
	PNG []byte
	// DDS é o DDS escolhido; nil quando a fonte é um .png em disco.
	DDS []byte
	// DDSPath/PNGPath apontam para as cópias já extraídas ("" se não há).
	DDSPath string
	PNGPath string
	// AssetPath é o caminho interno embutido no container (info de
	// exibição; vem do .dds.phyre, nunca de disco).
	AssetPath string
	// InMods indica que o .dds.phyre servido veio de mods/.
	InMods bool
}

// Resolve carrega a textura e escolhe a fonte da imagem para exibição.
//
// O .dds.phyre VISÍVEL é mods-first (a mesma semântica da coluna Traduzido):
// se o translator já importou uma textura, é ela que se vê; sem import, o
// pristine da fonte original (data/ no fluxo normal, o .vbf quando pedido
// via ResolveFrom).
func Resolve(version common.GameVersion, id string) (*Resolved, error) {
	return ResolveFrom(version, id, common.SourceData)
}

// ResolveFrom é Resolve com a fonte ORIGINAL explícita: SourceData lê o
// pristine de data/ (fluxo normal); SourceVbf lê o container — o caminho
// usado quando a árvore data/ não foi extraída na íntegra.
func ResolveFrom(version common.GameVersion, id string, originalSrc common.FileSource) (*Resolved, error) {
	rel := RelPath(version, id)
	acc, err := common.NewFileAccessorFrom(rel, common.SourceMods)
	if err != nil {
		return nil, err
	}
	inMods := acc.Exists
	if !inMods {
		if acc, err = common.NewFileAccessorFrom(rel, originalSrc); err != nil {
			return nil, err
		}
		if !acc.Exists {
			return nil, fmt.Errorf("textura %s não encontrada em %s nem em mods/", id, originalSrc)
		}
	}
	raw, err := acc.ReadBytes()
	if err != nil {
		return nil, fmt.Errorf("lendo %s: %w", id, err)
	}
	t, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}

	ddsPath, pngPath := ExportPaths(version, id)
	out := &Resolved{
		Source:       "phyre",
		Format:       t.Format,
		Width:        t.Width,
		Height:       t.Height,
		MipmapCount:  t.MipmapCount,
		MaxMipmapLvl: t.MaxMipmapLevel,
		AssetPath:    t.AssetPath,
		Texture:      t,
		InMods:       inMods,
	}

	switch {
	case fileExists(ddsPath):
		dds, rerr := os.ReadFile(ddsPath)
		if rerr == nil {
			if png, perr := DDSToPNG(dds); perr == nil {
				out.Source = "dds"
				out.DDS = dds
				out.PNG = png
				out.DDSPath = ddsPath
				out.PNGPath = existingOrEmpty(pngPath)
			}
		}
	case fileExists(pngPath):
		png, rerr := os.ReadFile(pngPath)
		if rerr == nil {
			out.Source = "png"
			out.PNG = png
			out.PNGPath = pngPath
			out.DDSPath = existingOrEmpty(ddsPath)
		}
	}

	if out.PNG == nil {
		// Nada em disco (ou falha ao ler): decodifica o próprio .phyre sem
		// gravar arquivo algum.
		dds, rerr := t.ExtractToDDS()
		if rerr != nil {
			return nil, fmt.Errorf("%s: %w", id, rerr)
		}
		png, rerr := DDSToPNG(dds)
		if rerr != nil {
			return nil, fmt.Errorf("%s: %w", id, rerr)
		}
		out.Source = "phyre"
		out.DDS = dds
		out.PNG = png
	}
	return out, nil
}

func existingOrEmpty(path string) string {
	if fileExists(path) {
		return path
	}
	return ""
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Extract grava .dds e .png em mods/images e devolve os caminhos.
func Extract(version common.GameVersion, id string) ([]string, error) {
	r, err := Resolve(version, id)
	if err != nil {
		return nil, err
	}
	ddsPath, pngPath := ExportPaths(version, id)
	dds := r.DDS
	if dds == nil {
		// Fonte .png em disco: extrai o DDS do próprio phyre.
		if r.Texture == nil {
			return nil, fmt.Errorf("%s: container indisponível", id)
		}
		if dds, err = r.Texture.ExtractToDDS(); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
	}
	png := r.PNG
	if png == nil {
		if png, err = DDSToPNG(dds); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
	}
	if err := writeFile(ddsPath, dds); err != nil {
		return nil, err
	}
	if err := writeFile(pngPath, png); err != nil {
		return nil, err
	}
	return []string{ddsPath, pngPath}, nil
}

// Save grava um dos formatos (dds|png) no caminho escolhido pelo usuário,
// sem obrigar a ter extraído antes.
func Save(version common.GameVersion, id, format, destPath string) error {
	r, err := Resolve(version, id)
	if err != nil {
		return err
	}
	var payload []byte
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "dds":
		if payload = r.DDS; payload == nil {
			if r.Texture == nil {
				return fmt.Errorf("%s: container indisponível", id)
			}
			if payload, err = r.Texture.ExtractToDDS(); err != nil {
				return fmt.Errorf("%s: %w", id, err)
			}
		}
	case "png":
		if payload = r.PNG; payload == nil {
			return fmt.Errorf("%s: png indisponível", id)
		}
	default:
		return fmt.Errorf("formato %q inválido (esperado dds ou png)", format)
	}
	return writeFile(destPath, payload)
}

// Import reempacota um .dds editado sobre o container PRISTINE de data/ e
// grava o resultado em mods/ (a árvore original nunca é alterada).
func Import(version common.GameVersion, id, ddsPath string) error {
	dds, err := os.ReadFile(ddsPath)
	if err != nil {
		return fmt.Errorf("lendo %s: %w", filepath.Base(ddsPath), err)
	}
	return ImportPayload(version, id, dds)
}

// ImportPayload é o núcleo do repack: monta container pristine de data/ +
// dds e grava em mods/. Compartilhado entre Import (arquivo escolhido pelo
// usuário) e Replicate (a imagem já ABERTA, sem arquivo externo) — os dois
// fazem exatamente o mesmo trabalho por cópia.
func ImportPayload(version common.GameVersion, id string, dds []byte) error {
	rel := RelPath(version, id)
	acc, err := common.NewFileAccessorFrom(rel, common.SourceData)
	if err != nil {
		return err
	}
	if !acc.Exists {
		return fmt.Errorf("textura %s sem original em data/ (só é possível importar sobre o pristine)", id)
	}
	original, err := acc.ReadBytes()
	if err != nil {
		return fmt.Errorf("lendo %s: %w", id, err)
	}
	return ImportPayloadFrom(version, id, original, dds)
}

// ImportPayloadFrom reempacota um DDS sobre bytes pristine fornecidos pelo
// chamador. É usado quando a fonte original é um .vbf e ainda não existe em
// data/; a saída continua sendo exclusivamente mods/.
func ImportPayloadFrom(version common.GameVersion, id string, original, dds []byte) error {
	rel := RelPath(version, id)
	packed, err := PackDDS(original, dds)
	if err != nil {
		return fmt.Errorf("%s: %w", id, err)
	}
	return writeFile(filepath.Join(common.GameFilesRoot, common.ModsFolder, rel), packed)
}

// Escopos aceitos por Delete.
const (
	// DeleteData apaga o pristine de data/ (a textura sai da árvore: sem
	// original, a regra 4 deixa o id "só em mods/").
	DeleteData = "data"
	// DeleteMods apaga a substituição em mods/ (desfaz a importação).
	DeleteMods = "mods"
	// DeleteBoth apaga nas duas árvores.
	DeleteBoth = "both"
)

// Delete apaga o container da textura conforme scope (data|mods|both) e
// SEMPRE os artefatos derivados (.dds/.png extraídos em mods/images) —
// senão Resolve passaria a servir um .dds órfão, ou falharia no container
// apagado.
//
// Devolve removed=false, err=nil quando nada existia (o chamador conta como
// "pulado", não como falha); erro real = falha de disco/permissão.
func Delete(version common.GameVersion, id, scope string) (removed bool, err error) {
	if !ValidID(id) {
		return false, fmt.Errorf("textura desconhecida: %s", id)
	}
	rel := RelPath(version, id)
	remove := func(path string) error {
		if !fileExists(path) {
			return nil
		}
		if rerr := os.Remove(path); rerr != nil {
			return fmt.Errorf("apagando %s: %w", filepath.Base(path), rerr)
		}
		removed = true
		return nil
	}

	switch scope {
	case DeleteData:
		if err := remove(filepath.Join(common.GameFilesRoot, rel)); err != nil {
			return removed, err
		}
	case DeleteMods:
		if err := remove(filepath.Join(common.GameFilesRoot, common.ModsFolder, rel)); err != nil {
			return removed, err
		}
	case DeleteBoth:
		if err := remove(filepath.Join(common.GameFilesRoot, rel)); err != nil {
			return removed, err
		}
		if err := remove(filepath.Join(common.GameFilesRoot, common.ModsFolder, rel)); err != nil {
			return removed, err
		}
	default:
		return false, fmt.Errorf("escopo %q inválido (esperado %s, %s ou %s)",
			scope, DeleteData, DeleteMods, DeleteBoth)
	}

	ddsPath, pngPath := ExportPaths(version, id)
	if err := remove(ddsPath); err != nil {
		return removed, err
	}
	if err := remove(pngPath); err != nil {
		return removed, err
	}

	// Poda do que ficou vazio: remove() só apaga ARQUIVOS e o ramo criado
	// pela gravação sobrava (ffx_data/…, mods/… e mods/images/… vazios no
	// Explorer). Floor único = GameFilesRoot, então só a raiz da árvore é
	// intocável. Roda mesmo com removed=false (limpa o que um delete antigo
	// deixou), é best-effort e cobre só o ramo destes caminhos — faxina é
	// fora do app, aqui não se caminha árvore.
	//
	// Ordem importa: mods/images/ fica DENTRO de mods/, então as imagens
	// saem primeiro — é o que permite mods/ esvaziar de verdade numa leva.
	common.PruneEmptyDirs(filepath.Dir(ddsPath), common.GameFilesRoot)
	if scope == DeleteMods || scope == DeleteBoth {
		common.PruneEmptyDirs(
			filepath.Dir(filepath.Join(common.GameFilesRoot, common.ModsFolder, rel)),
			common.GameFilesRoot)
	}
	if scope == DeleteData || scope == DeleteBoth {
		common.PruneEmptyDirs(
			filepath.Dir(filepath.Join(common.GameFilesRoot, rel)),
			common.GameFilesRoot)
	}
	return removed, nil
}

func writeFile(path string, payload []byte) error {
	if err := common.CheckWritablePath(path); err != nil {
		return fmt.Errorf("destino recusado para %s: %w", filepath.Base(path), err)
	}
	if err := common.EnsurePathExists(path); err != nil {
		return fmt.Errorf("criando diretório de %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return fmt.Errorf("gravando %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ImportContainer grava um .dds.phyre COMPLETO sobre o container em mods/.
//
// É cópia de container, não repack: a fonte já está no formato que o .vbf
// usa, então não há o que embalar no pristine (o caminho de ImportPayloadFrom,
// que recebe um DDS e o monta sobre o original de data/).
func ImportContainer(version common.GameVersion, id string, container []byte) error {
	if !ValidID(id) {
		return fmt.Errorf("textura desconhecida: %s", id)
	}
	rel := RelPath(version, id)
	return writeFile(filepath.Join(common.GameFilesRoot, common.ModsFolder, rel), container)
}
