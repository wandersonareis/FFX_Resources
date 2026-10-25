package common

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	ModsFolder        = "mods/"
	ModsTranslatedDir = "translated"
	// ModsImagesDir guarda os artefatos de imagem extraídos (.dds/.png) com
	// o caminho interno do .vbf — irmão de mods/edits, que é SÓ de texto.
	ModsImagesDir = "images"
	DirData       = "data"
	DirTranslated = "translated"
)

// DefaultTranslatedDir deriva o diretório de tradução de um gamefiles:
// <gamefiles>/mods/translated. Usado quando não há valor no config.json.
func DefaultTranslatedDir(gameFilesDir string) string {
	return filepath.Join(gameFilesDir, ModsFolder, ModsTranslatedDir)
}

var (
	ResourcesRoot = GetExecDir()
	GameFilesRoot = ResourcesRoot
	// DisableMods desliga a preferência por arquivos em mods/. O estado
	// é controlado em runtime via SetModsEnabled (config EnableMods).
	DisableMods = false
)

// SetModsEnabled liga/desliga a preferência mods-first da resolução de
// caminhos (FileAccessor + loaders de binário). O valor vem do config
// (EnableMods) e é aplicado no bootstrap da aplicação.
func SetModsEnabled(enabled bool) {
	DisableMods = !enabled
}

// FileSource escolhe a árvore usada na LEITURA de um caminho relativo ao
// GameFilesRoot. `data/` é imutável e é a fonte da verdade; `mods/` guarda
// o binário traduzido do último save e só pode ser usado para a coluna
// Traduzido (e para export/apply, que nunca escrevem em data/).
type FileSource int

const (
	// SourcePreferred mantém o fluxo mods-first: mods/ quando EnableMods,
	// senão a árvore original. É a leitura da TRADUÇÃO (estado do último
	// save) e de tudo que alimenta export/apply.
	SourcePreferred FileSource = iota

	// SourceData lê SEMPRE a árvore original (data/), ignorando mods/ e o
	// toggle EnableMods. É a fonte da verdade: coluna Original e presença
	// de arquivo/linha/idioma.
	SourceData

	// SourceMods lê SEMPRE a árvore mods/ (para testes e conferência de
	// presença do overlay).
	SourceMods

	// SourceVbf lê SEMPRE o overlay do .vbf em memória (coluna Original
	// vinda do container). Só tem conteúdo dentro de WithVbfSource.
	SourceVbf

	// SourceVbfPreferred é o mods-first de SourcePreferred com data/ trocado
	// pelo .vbf: mods/ quando existe, senão o container (estado atual da
	// tradução para uma entrada que só existe no .vbf).
	SourceVbfPreferred
)

// IsVbfSource informa se a fonte resolve pelo overlay do .vbf.
func (s FileSource) IsVbfSource() bool {
	return s == SourceVbf || s == SourceVbfPreferred
}

// String devolve o rótulo da fonte (logs/diagnóstico).
func (s FileSource) String() string {
	switch s {
	case SourceData:
		return "data"
	case SourceMods:
		return "mods"
	case SourceVbf:
		return "vbf"
	case SourceVbfPreferred:
		return "mods>vbf"
	default:
		return "preferred"
	}
}

func SetGameFilesRoot(path string) {
	if path == "" {
		fmt.Println("Invalid path provided for GameFilesRoot")
		return
	}
	GameFilesRoot = filepath.Clean(path)
	fmt.Printf("GameFilesRoot set to: %s\n", GameFilesRoot)
}

func getRealFile(path string) string {
	return filepath.Join(GameFilesRoot, path)
}

func getModdedFile(path string) string {
	return filepath.Join(GameFilesRoot, ModsFolder, path)
}

// WriteBytesToFileAnywhere writes a slice of integers as bytes to a file
// WITHOUT the tree guard: o acesso externo (menu/CLI avulso) lê e grava em
// QUALQUER lugar — inclusive dentro da árvore do jogo, ao lado do arquivo
// apontado pelo usuário (regra: o menu não respeita a estrutura do frontend).
// Cria os diretórios necessários antes de gravar.
func WriteBytesToFileAnywhere(path string, bytes []byte) error {
	CreateDirectories(path)
	err := os.WriteFile(path, bytes, 0644)
	recordWrite(path, len(bytes), err)
	return err
}

// WriteBytesToFile writes a slice of integers as bytes to a file
// Creates necessary directories before writing
//
// Todo escritor de binário/texto do app (frontend) passa por aqui, então é
// também o ponto único da trava de gravação: alvo .vbf ou fora de mods/ é
// recusado antes de criar diretório (ver CheckWritablePath). O caminho SEM
// a trava é WriteBytesToFileAnywhere — é o que o menu/CLI usa.
func WriteBytesToFile(path string, bytes []byte) error {
	if err := CheckWritablePath(path); err != nil {
		// Recusado antes de criar diretório: o destino é conhecido e a
		// operação precisa vê-lo no log (data/ somente leitura, .vbf).
		recordWrite(path, len(bytes), err)
		return err
	}
	return WriteBytesToFileAnywhere(path, bytes)
}

// CreateDirectories creates the necessary directories for a given file path
// If the path is a directory, it ensures the directory exists
// If the path is a file, it creates all necessary parent directories
func CreateDirectories(path string) {
	info, err := os.Stat(path)
	var dirPath string

	if err == nil && info.IsDir() {
		dirPath = path
	} else {
		dirPath = filepath.Dir(path)
	}

	if dirPath == "" || dirPath == "." {
		return
	}

	err = os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		fmt.Println("Failed to create directories: ", err)
	}
}

type FileAccessor struct {
	RootPath     string
	ResolvedPath string
	Info         os.FileInfo
	Size         int64
	Exists       bool

	// source é a árvore/fonte resolvida (para leituras que precisam saber
	// de onde veio — ex.: pular o atalho de disco quando é .vbf).
	source FileSource
	// mem/vbf marcam conteúdo vindo do overlay do .vbf: a leitura não toca
	// o disco (ResolvedPath é um caminho-de-falsa-conta só para log).
	mem []byte
	vbf bool
}

// NewFileAccessor creates a new FileAccessor instance for the given path
// This function handles both existing and non-existing paths gracefully
//
// Parameters:
//   - path: File path to access (can be relative or absolute)
//
// Returns:
//   - *FileAccessor: FileAccessor instance with file information
//   - error: Error if path resolution fails (not if file doesn't exist)
func NewFileAccessor(path string) (FileAccessor, error) {
	return NewFileAccessorFrom(path, SourcePreferred)
}

// NewFileAccessorFrom cria um FileAccessor resolvido na árvore indicada
// (ver FileSource). Caminhos absolutos passam direto (a fonte não se aplica)
// — exceto as fontes de .vbf, que nunca trabalham com caminho absoluto.
func NewFileAccessorFrom(path string, src FileSource) (FileAccessor, error) {
	if src.IsVbfSource() {
		if filepath.IsAbs(path) {
			return FileAccessor{}, fmt.Errorf("fonte %s não aceita caminho absoluto: %s", src, path)
		}
		return newVbfAccessor(path, src)
	}

	resolvedPath, err := resolvePathFrom(path, src)
	if err != nil {
		return FileAccessor{}, err
	}

	fileInfo, exists := getFileInfo(resolvedPath)

	return FileAccessor{
		RootPath:     path,
		ResolvedPath: resolvedPath,
		Info:         fileInfo,
		Size:         getFileSize(fileInfo),
		Exists:       exists,
		source:       src,
	}, nil
}

// IsVbf informa se o arquivo resolvido veio do overlay do .vbf.
func (f *FileAccessor) IsVbf() bool { return f.vbf }

// Source devolve a árvore/fonte da qual o accessor foi resolvido.
func (f *FileAccessor) Source() FileSource { return f.source }

func (f *FileAccessor) ReadBytes() ([]byte, error) {
	if f.vbf {
		if f.mem == nil {
			return nil, ErrVbfMissing
		}
		return f.mem, nil
	}
	data, err := os.ReadFile(f.ResolvedPath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// NewRealFileAccessor cria um FileAccessor apontando SEMPRE para o arquivo
// original (<gamefiles>/<path>), ignorando mods — para enumeração de
// diretórios (descoberta de eventos) e casos onde o caminho deve ser o
// original mesmo com mods habilitado. A leitura por arquivo continua podendo
// preferir mods via NewFileAccessor.
func NewRealFileAccessor(path string) (FileAccessor, error) {
	return NewFileAccessorFrom(path, SourceData)
}

// getRealPath resolve o caminho absoluto na árvore original, sem mods.
func getRealPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return getRealFile(path), nil
}

func resolvePath(path string) (string, error) {
	return resolvePathFrom(path, SourcePreferred)
}

// resolvePathFrom resolve o caminho conforme a fonte pedida. Caminhos
// absolutos são inalterados (a árvore só se aplica a caminhos relativos).
func resolvePathFrom(path string, src FileSource) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	switch src {
	case SourceData:
		return getRealFile(path), nil
	case SourceMods:
		return getModdedFile(path), nil
	case SourceVbf, SourceVbfPreferred:
		// Caminho-de-falsa-conta do overlay: o conteúdo só sai pelo
		// FileAccessor.ReadBytes (os acessores VBF nem passam por aqui).
		return vbfPathFor(path), nil
	default:
		if DisableMods {
			return getRealFile(path), nil
		}
		return resolveModdedPath(path), nil
	}
}

func resolveModdedPath(path string) string {
	moddedPath := getModdedFile(path)
	if fileExists(moddedPath) {
		return moddedPath
	}
	return getRealFile(path)
}

func getFileInfo(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}

func getFileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (f *FileAccessor) String() string {
	if f.Exists {
		return fmt.Sprintf("FileAccessor{RootPath: %s, ResolvedPath: %s, Size: %d bytes}", f.RootPath, f.ResolvedPath, f.Size)
	}
	return fmt.Sprintf("FileAccessor{RootPath: %s, ResolvedPath: %s, Exists: false}", f.RootPath, f.ResolvedPath)
}
