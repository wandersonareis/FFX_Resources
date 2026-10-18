package weaponnames

import (
	"fmt"

	"sync"

	"ffxresources/backend/common"
	"ffxresources/backend/dto"
	ffxencoding "ffxresources/backend/core/encoding"
	"ffxresources/backend/fileFormats/objectsfile"
	"ffxresources/backend/fileFormats/objectsfile/chunkmap"
	"ffxresources/backend/fileFormats/objectsfile/schema/ffx"
)

// registerOnce garante que Register é idempotente.
var registerOnce sync.Once

// Register registra o schema de w_name.bin nos maps do chunkmap. As rows
// seguem a política de domínio deste pacote (rotuladas pelo dono, apply por
// model_id) — os wrappers apenas constroem o File[ffx.WeaponName] na fonte
// pedida e delegam para Rows/Apply/Collection. O registro mora aqui (e não
// em chunkmap/schemas_*.go) para evitar import cíclico: weaponnames já
// importa chunkmap. O chamador é o services (package que enxerga os dois).
func Register() {
	registerOnce.Do(func() {
		key := objectsfile.FileLayoutKey(common.GameVersionFFX, Pattern)
		chunkmap.SchemaRegistry[key] = load
		chunkmap.SchemaSourceLoaders[key] = loadFrom
		chunkmap.SchemaAppliers[key] = apply
		chunkmap.SchemaMeasurers[key] = measure
	})
}

// load monta a coleção de exibição (mods-first): carrega o binário padrão e
// mescla os demais idiomas, validando os model_ids contra a tabela estática.
func load(version common.GameVersion, _ string) (dto.Collection, error) {
	if version != common.GameVersionFFX {
		return nil, fmt.Errorf("w_name: versao %s nao suportada (FFX only)", version)
	}
	if err := ffxencoding.EnsureAllCharsetsLoaded(version); err != nil {
		return nil, fmt.Errorf("w_name: charsets: %w", err)
	}
	main, err := chunkmap.LoadFileFromSource[ffx.WeaponName](version, Pattern, common.SourcePreferred)
	if err != nil {
		return nil, err
	}
	files := map[string]*chunkmap.File[ffx.WeaponName]{common.DefaultLocalization: main}
	for _, lang := range common.SupportedLanguageCodes() {
		if lang == common.DefaultLocalization {
			continue
		}
		langFile, lerr := chunkmap.LoadFileFromBytesForLanguage[ffx.WeaponName](version, Pattern, lang, nil)
		if lerr != nil {
			continue
		}
		files[lang] = langFile
	}
	rows, err := Rows(main, files)
	if err != nil {
		return nil, err
	}
	return Collection(version, rows)
}

// loadFrom monta a coleção a partir da FONTE indicada (SourceData para o
// original de data/, SourceVbf/SourceVbfPreferred para o container). Só o
// idioma padrão entra na leitura de "original".
func loadFrom(version common.GameVersion, _ string, src common.FileSource) (dto.Collection, error) {
	if version != common.GameVersionFFX {
		return nil, fmt.Errorf("w_name: versao %s nao suportada (FFX only)", version)
	}
	if err := ffxencoding.EnsureAllCharsetsLoaded(version); err != nil {
		return nil, fmt.Errorf("w_name: charsets: %w", err)
	}
	main, err := chunkmap.LoadFileFromSource[ffx.WeaponName](version, Pattern, src)
	if err != nil {
		return nil, err
	}
	rows, err := Rows(main, map[string]*chunkmap.File[ffx.WeaponName]{common.DefaultLocalization: main})
	if err != nil {
		return nil, err
	}
	return Collection(version, rows)
}

// apply aplica as rows no binário (mods-first) e grava em mods/.
func apply(version common.GameVersion, entry dto.FileEntry) error {
	if version != common.GameVersionFFX {
		return fmt.Errorf("w_name: versao %s nao suportada (FFX only)", version)
	}
	if err := ffxencoding.EnsureAllCharsetsLoaded(version); err != nil {
		return fmt.Errorf("w_name: charsets: %w", err)
	}
	file, err := chunkmap.LoadFile[ffx.WeaponName](version, Pattern)
	if err != nil {
		return err
	}
	if err := Apply(file, entry.Rows); err != nil {
		return err
	}
	return file.SaveToBinary("")
}

// measure aplica as rows em memória e devolve o tamanho da string table
// reconstruída (dedup inclusive). Sem gravar nada. As rows de w_name são
// rotuladas pelo dono ("Tidus.Standard") — o medidor genérico não as
// entende, por isso o MESMO Apply do fluxo real é injetado: a medição
// reflete exatamente o que seria gravado (inclusive a replicação do
// Standard na leitura Simplified ausente).
func measure(version common.GameVersion, _ string, rows []dto.TextRow) (int, error) {
	if version != common.GameVersionFFX {
		return 0, fmt.Errorf("w_name: versao %s nao suportada (FFX only)", version)
	}
	if err := ffxencoding.EnsureAllCharsetsLoaded(version); err != nil {
		return 0, fmt.Errorf("w_name: charsets: %w", err)
	}
	file, err := chunkmap.LoadFile[ffx.WeaponName](version, Pattern)
	if err != nil {
		return 0, err
	}
	return file.MeasureStringTableWith(rows, common.DefaultLocalization, Apply)
}