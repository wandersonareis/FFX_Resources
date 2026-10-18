package dto

// AppConfigSnapshot espelha o config.json inteiro em um objeto só: é o
// payload da consulta GetConfigSnapshot (carga inicial do store do
// frontend) e do evento AppConfigChanged (re-emitido após cada setter —
// o "eco" que o v2 fazia por campo, agora num evento único).
//
// Com o snapshot completo o frontend espelha a VERDADE do backend:
// caminho normalizado (filepath.Abs), executável recusado em silêncio
// (o persistido continua) e translated derivado do gamefiles novo
// acompanham a mudança na mesma resposta.
type AppConfigSnapshot struct {
	GameFilesLocation string `json:"game_files_location"`
	TranslateLocation string `json:"translate_location"`
	GameExeLocation   string `json:"game_exe_location"`
	EnableMods        bool   `json:"enable_mods"`
	GameVersion       string `json:"game_version"`
}
