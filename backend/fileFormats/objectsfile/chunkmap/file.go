package chunkmap

import (
	"fmt"
	"path/filepath"

	"ffxresources/backend/common"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/core/strings"
	"ffxresources/backend/datastore"
	"ffxresources/backend/dto"
)

/*
Ciclo de vida próprio do fluxo novo — o uso é tão simples quanto o fluxo
antigo, sem depender de nenhum tipo dele:

	chunkmap.Register... (não há: a struct T é passada direto no generics)

	f, err := chunkmap.LoadFile[ffx2.PCommand](common.GameVersionFFX2, "battle/kernel/command.bin")
	texts, err := f.ExportText(lang)      // export: campos de texto por chunk
	err = f.ImportText(texts)             // import: DTO de volta nos slots
	out, err := f.ToBytes()               // header + chunks + string table
	err = f.SaveToBinary(path)            // grava na árvore mods + cópia extra

O arquivo guarda um idioma só (o binário do jogo é por locale); a carga
resolve o idioma padrão e mescla os demais idiomas nos slots (best-effort).
A gravação do arquivo usa o idioma padrão.
*/

// File é um binário de objetos mapeado pela struct T.
type File[T any] struct {
	Version     common.GameVersion
	PatternPath string
	Header      FileHeader
	// headerBytes é o cabeçalho ORIGINAL (assinaturas/padding preservados
	// byte-a-byte); a gravação o devolve verbatim — a struct T nunca muda
	// contagens, então o cabeçalho não precisa ser re-derivado.
	headerBytes []byte
	// StringBytes é a string table ORIGINAL (fonte dos textos lidos).
	StringBytes []byte
	// raw é o binário ORIGINAL: devolvido verbatim pelo ToBytes enquanto
	// nenhum texto foi editado (a reconstrução da string table com dedup de
	// bytes produz arquivo válido, mas não byte-idêntico ao original).
	raw []byte
	// Objects são os chunks decodificados, na ordem do arquivo.
	Objects []*MappedTextObject[T]
}

// LoadFile carrega o binário do arquivo na resolução do app (mods-first via
// FileAccessor) e mescla os textos dos demais idiomas nos slots.
func LoadFile[T any](version common.GameVersion, patternPath string) (*File[T], error) {
	if err := ffxencoding.EnsureAllCharsetsLoaded(version); err != nil {
		return nil, fmt.Errorf("%s: charsets: %w", patternPath, err)
	}
	defaultLang := common.DefaultLocalization
	data, err := readFileBytes(version, defaultLang, patternPath)
	if err != nil {
		return nil, err
	}
	f, err := loadFileFromBytes[T](version, patternPath, data, defaultLang)
	if err != nil {
		return nil, err
	}
	for _, locKey := range common.SupportedLanguageCodes() {
		if locKey == defaultLang {
			continue
		}
		locData, err := readFileBytes(version, locKey, patternPath)
		if err != nil {
			common.LogVerbose("[chunkmap] %s/%s: %v", patternPath, locKey, err)
			continue
		}
		locFile, err := loadFileFromBytes[T](version, patternPath, locData, locKey)
		if err != nil {
			common.LogVerbose("[chunkmap] %s/%s: %v", patternPath, locKey, err)
			continue
		}
		f.mergeLocalizations(locFile)
	}
	return f, nil
}

// LoadFileFromBytes decodifica bytes já lidos (testes e pipelines próprios).
// A struct pode ser menor que individualLength: Decode guarda os bytes
// restantes do chunk em Tail. Uma struct maior que o chunk é rejeitada.
func LoadFileFromBytes[T any](version common.GameVersion, patternPath string, data []byte) (*File[T], error) {
	return loadFileFromBytes[T](version, patternPath, data, common.DefaultLocalization)
}

// LoadFileFromBytesForLanguage decodes one locale's binary. Use it when the
// caller reads locale files independently rather than through LoadFile.
func LoadFileFromBytesForLanguage[T any](version common.GameVersion, patternPath, languageCode string, data []byte) (*File[T], error) {
	if err := validateLanguage(languageCode); err != nil {
		return nil, err
	}
	return loadFileFromBytes[T](version, patternPath, data, languageCode)
}

func loadFileFromBytes[T any](version common.GameVersion, patternPath string, data []byte, languageCode string) (*File[T], error) {
	h, chunks, strtab, err := Split(data, version)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", patternPath, err)
	}
	if size := StructSize[T](); size > h.IndividualLength {
		return nil, fmt.Errorf("%s: struct de %d bytes excede individualLength do binário (%d)",
			patternPath, size, h.IndividualLength)
	}
	f := &File[T]{
		Version:     version,
		PatternPath: patternPath,
		Header:      h,
		headerBytes: append([]byte(nil), data[:h.HeaderLength]...),
		StringBytes: strtab,
		raw:         append([]byte(nil), data...),
		Objects:     make([]*MappedTextObject[T], 0, h.ChunkCount()),
	}
	for i := 0; i < h.ChunkCount(); i++ {
		obj, err := NewMappedTextObject[T](
			h.Chunk(chunks, i), strtab, h.IndividualLength,
			languageCode, version, patternPath,
		)
		if err != nil {
			return nil, fmt.Errorf("%s: chunk %d: %w", patternPath, i, err)
		}
		f.Objects = append(f.Objects, obj)
	}
	return f, nil
}

// mergeLocalizations mescla os textos de outro File (lido na árvore de outro
// idioma) nos slots, por posição (a ordem dos chunks é a do arquivo).
func (f *File[T]) mergeLocalizations(loc *File[T]) {
	n := min(len(f.Objects), len(loc.Objects))
	for i := 0; i < n; i++ {
		f.Objects[i].SetLocalizations(loc.Objects[i])
	}
}

// Count devolve a quantidade de chunks carregados.
func (f *File[T]) Count() int { return len(f.Objects) }

// Get devolve o i-ésimo objeto (nil fora da faixa).
func (f *File[T]) Get(i int) *MappedTextObject[T] {
	if i < 0 || i >= len(f.Objects) {
		return nil
	}
	return f.Objects[i]
}

// ExportText exporta os campos de texto de TODOS os chunks, na ordem do
// arquivo. Chunks sem texto são pulados (o Index mantém a posição real).
func (f *File[T]) ExportText(languageCode string) ([]FileText, error) {
	out := make([]FileText, 0, len(f.Objects))
	for i, obj := range f.Objects {
		fields, err := obj.ExportText(languageCode)
		if err != nil {
			return nil, fmt.Errorf("%s: chunk %d: %w", f.PatternPath, i, err)
		}
		if len(fields) == 0 {
			continue
		}
		out = append(out, FileText{Index: i, Fields: fields})
	}
	return out, nil
}

// ImportText aplica os campos de volta nos chunks. Índice fora da faixa é
// erro (pega DTO corrompido).
func (f *File[T]) ImportText(texts []FileText) error {
	for _, t := range texts {
		if t.Index < 0 || t.Index >= len(f.Objects) {
			return fmt.Errorf("%s: chunk %d fora da faixa (%d chunks)",
				f.PatternPath, t.Index, len(f.Objects))
		}
		if err := f.Objects[t.Index].ImportText(t.Fields); err != nil {
			return fmt.Errorf("%s: chunk %d: %w", f.PatternPath, t.Index, err)
		}
	}
	return nil
}

// ToBytes serializa o arquivo inteiro no idioma padrão. Sem edição, devolve
// o binário original byte-a-byte; com edição, escreve o cabeçalho original
// verbatim + chunks (com os segmentos atualizados) + string table
// reconstruída com dedup de bytes (textos iguais compartilham o offset —
// a mesma técnica de compilação do kernel).
func (f *File[T]) ToBytes() ([]byte, error) {
	if !f.Edited() {
		return append([]byte(nil), f.raw...), nil
	}
	return f.encodeLanguage(common.DefaultLocalization)
}

// Edited reporta se algum chunk mudou o texto no idioma da carga.
func (f *File[T]) Edited() bool {
	for _, obj := range f.Objects {
		if obj.Edited() {
			return true
		}
	}
	return false
}

func (f *File[T]) encodeLanguage(languageCode string) ([]byte, error) {
	var keyed []datastore.IGlobalKeyedString
	for _, obj := range f.Objects {
		for _, slot := range obj.slots {
			if content := slot.contents[languageCode]; content != nil {
				keyed = append(keyed, content)
			}
		}
	}
	// A string table ORIGINAL é a base: refs quebrados (MID/OOB) e campos
	// vazios mantêm o segmento cru, e textos já gravados não são recompilados.
	strtab := strings.RebuildKeyedStrings(
		keyed, ffxencoding.GetCharsetForLanguage(languageCode), f.Version, f.StringBytes,
	)

	chunks := make([]byte, 0, f.Header.TotalLength)
	for i, obj := range f.Objects {
		b, err := obj.ToBytes(languageCode)
		if err != nil {
			return nil, fmt.Errorf("%s: chunk %d: %w", f.PatternPath, i, err)
		}
		chunks = append(chunks, b...)
	}

	out := make([]byte, 0, f.Header.HeaderLength+len(chunks)+len(strtab))
	out = append(out, f.headerBytes...)
	out = append(out, chunks...)
	out = append(out, strtab...)
	return out, nil
}

// rebuildEditedStringTable aplica as rows em um clone raso dos slots e
// reconstrói a string table do idioma pedido. O File original permanece
// intacto (medição não é edição). O applier é injetado: schemas com
// política de naming própria (w_name rotula rows pelo dono, "Tidus.Standard")
// medem com o MESMO Apply do fluxo real em vez do genérico.
func (f *File[T]) rebuildEditedStringTable(rows []dto.TextRow, languageCode string, apply func(*File[T], []dto.TextRow) error) ([]byte, error) {
	// Clona o File trocando os slots por cópias: as edições ficam no clone.
	clone := *f
	clone.Objects = make([]*MappedTextObject[T], len(f.Objects))
	for i, obj := range f.Objects {
		clone.Objects[i] = obj.cloneSlots()
	}
	if err := apply(&clone, rows); err != nil {
		return nil, err
	}
	var keyed []datastore.IGlobalKeyedString
	for _, obj := range clone.Objects {
		for _, slot := range obj.slots {
			if content := slot.contents[languageCode]; content != nil {
				keyed = append(keyed, content)
			}
		}
	}
	return strings.RebuildKeyedStrings(
		keyed, ffxencoding.GetCharsetForLanguage(languageCode), f.Version, clone.StringBytes,
	), nil
}

// MeasureStringTable aplica as rows em memória via GenericApply e devolve o
// tamanho da string table reconstruída (dedup inclusive) no idioma pedido,
// sem gravar nada. Espelha o encodeLanguage byte a byte — sem gerar os
// bytes, medindo apenas.
func (f *File[T]) MeasureStringTable(rows []dto.TextRow, languageCode string) (int, error) {
	return f.MeasureStringTableWith(rows, languageCode, GenericApply[T])
}

// MeasureStringTableWith é o MeasureStringTable com o applier injetado:
// políticas de domínio com naming próprio aplicam com o mesmo Apply do
// fluxo real, garantindo que a medição reflita exatamente o que seria
// gravado.
func (f *File[T]) MeasureStringTableWith(rows []dto.TextRow, languageCode string, apply func(*File[T], []dto.TextRow) error) (int, error) {
	if err := validateLanguage(languageCode); err != nil {
		return 0, err
	}
	if apply == nil {
		apply = GenericApply[T]
	}
	strtab, err := f.rebuildEditedStringTable(rows, languageCode, apply)
	if err != nil {
		return 0, err
	}
	return len(strtab), nil
}

// SaveToBinary grava o arquivo na árvore mods (a localização canônica do
// patternPath — o overlay que o app lê de volta) e, quando filePath é
// absoluto, uma cópia extra (rotina de teste/reimport). Caminho relativo é
// ignorado — gravar nele criaria o arquivo no CWD do processo.
func (f *File[T]) SaveToBinary(filePath string) error {
	var last []byte
	for _, localizationKey := range common.SupportedLanguageCodes() {
		if localizationKey != common.DefaultLocalization {
			continue // o binário guarda um idioma só; o overlay é do padrão
		}
		buf, err := f.encodeLanguage(localizationKey)
		if err != nil {
			return err
		}
		localePath := filepath.Join(
			common.GameFilesRoot, common.ModsFolder,
			common.GetLocalizationRootForVersion(f.Version, localizationKey),
			filepath.FromSlash(f.PatternPath),
		)
		localePath = filepath.FromSlash(localePath)
		if err := common.WriteBytesToFile(localePath, buf); err != nil {
			return fmt.Errorf("gravar %s: %w", localePath, err)
		}
		common.LogVerbose("[chunkmap] gravado: %s (%d bytes)", localePath, len(buf))
		last = buf
	}
	if filePath != "" && filepath.IsAbs(filePath) {
		return common.WriteBytesToFile(filePath, last)
	}
	return nil
}

// LoadFileFromSource carrega o binário de uma FONTE específica (SourceData
// para o original de data/, SourcePreferred para mods-first).
func LoadFileFromSource[T any](version common.GameVersion, patternPath string, src common.FileSource) (*File[T], error) {
	if err := ffxencoding.EnsureAllCharsetsLoaded(version); err != nil {
		return nil, fmt.Errorf("%s: charsets: %w", patternPath, err)
	}
	defaultLang := common.DefaultLocalization
	data, err := readFileBytesFrom(version, defaultLang, patternPath, src)
	if err != nil {
		return nil, err
	}
	f, err := loadFileFromBytes[T](version, patternPath, data, defaultLang)
	if err != nil {
		return nil, err
	}
	for _, locKey := range common.SupportedLanguageCodes() {
		if locKey == defaultLang {
			continue
		}
		locData, lerr := readFileBytesFrom(version, locKey, patternPath, src)
		if lerr != nil {
			common.LogVerbose("[chunkmap] %s/%s: %v", patternPath, locKey, lerr)
			continue
		}
		locFile, lerr := loadFileFromBytes[T](version, patternPath, locData, locKey)
		if lerr != nil {
			common.LogVerbose("[chunkmap] %s/%s: %v", patternPath, locKey, lerr)
			continue
		}
		f.mergeLocalizations(locFile)
	}
	return f, nil
}

// readFileBytesFrom lê os bytes do binário da FONTE indicada.
func readFileBytesFrom(version common.GameVersion, languageCode, patternPath string, src common.FileSource) ([]byte, error) {
	rel := filepath.Join(
		common.GetLocalizationRootForVersion(version, languageCode),
		filepath.FromSlash(patternPath),
	)
	accessor, err := common.NewFileAccessorFrom(rel, src)
	if err != nil {
		return nil, fmt.Errorf("acessar %s: %w", rel, err)
	}
	if !accessor.Exists {
		return nil, fmt.Errorf("arquivo não existe em %s", accessor.ResolvedPath)
	}
	data, err := accessor.ReadBytes()
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", accessor.ResolvedPath, err)
	}
	return data, nil
}

// readFileBytes lê os bytes do binário na resolução do app (mods-first via
// FileAccessor sobre GameFilesRoot).
func readFileBytes(version common.GameVersion, languageCode, patternPath string) ([]byte, error) {
	rel := filepath.Join(
		common.GetLocalizationRootForVersion(version, languageCode),
		filepath.FromSlash(patternPath),
	)
	accessor, err := common.NewFileAccessorFrom(rel, common.SourcePreferred)
	if err != nil {
		return nil, fmt.Errorf("acessar %s: %w", rel, err)
	}
	if !accessor.Exists {
		return nil, fmt.Errorf("arquivo não existe em %s (GameFilesRoot=%s; a raiz deve conter ffx_ps2)",
			accessor.ResolvedPath, common.GameFilesRoot)
	}
	data, err := accessor.ReadBytes()
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", accessor.ResolvedPath, err)
	}
	return data, nil
}
