package common

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func sanitizationPath(path string) string {
	return filepath.Clean(path)
}

// IsPathExists checks if the given path exists in the file system.
// It sanitizes the input path before performing the check.
//
// Parameters:
//   - path: The file system path to check.
//
// Returns:
//   - bool: true if the path exists, false otherwise.
//     already exists or was successfully created.
func IsPathExists(path string) bool {
	cPath := sanitizationPath(path)
	_, err := os.Stat(cPath)
	return !os.IsNotExist(err)
}

// EnsurePathExists ensures that the directory for the given path exists.
// If the path includes a file name, the directory containing the file will be created.
// If the directory does not exist, it will be created with the appropriate permissions.
//
// Parameters:
//
//	path - The path for which to ensure the existence of the directory.
//
// Returns:
//
//	error - An error if the directory could not be created, or nil if the directory
//	        already exists or was successfully created.
func EnsurePathExists(path string) error {
	cPath := sanitizationPath(path)

	if filepath.Ext(cPath) != "" {
		cPath = filepath.Dir(cPath)
	}

	if err := os.MkdirAll(cPath, os.ModePerm); err != nil {
		return fmt.Errorf("error when creating the destination directory %s: %s", cPath, err.Error())
	}

	return nil
}

// IsValidFilePath checks if the given file path is valid.
// It returns false if the base name of the path is empty or starts with a dot.
// Otherwise, it returns true.
func IsValidFilePath(path string) bool {
	base := filepath.Base(path)

	if base == "" || strings.HasPrefix(base, ".") {
		return false
	}
	return true
}

func hasExactComponent(path, component string) bool {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(os.PathSeparator))
	return slices.Contains(parts, component)
}

func checkPS2Version1(path string) (GameVersion, string, bool) {
	p1 := "ffx_ps2"
	p2 := "ffx"
	p3 := "master"

	if hasExactComponent(path, p1) && hasExactComponent(path, p2) && hasExactComponent(path, p3) {
		return GameVersionFFX, p1, true
	}
	return GameVersion{}, "", false
}

func checkPS2Version2(path string) (GameVersion, string, bool) {
	p1 := filepath.Join("ffx_ps2")
	p2 := filepath.Join("ffx2")
	p3 := filepath.Join("master")
	if hasExactComponent(path, p1) && hasExactComponent(path, p2) && hasExactComponent(path, p3) {
		return GameVersionFFX2, p1, true
	}
	return GameVersion{}, "", false
}

func checkDataVersion1(path string) (GameVersion, string, bool) {
	p1 := filepath.Join("ffx_data")
	p2 := filepath.Join("gamedata")
	p3 := filepath.Join("ps3data")
	if hasExactComponent(path, p1) && hasExactComponent(path, p2) && hasExactComponent(path, p3) {
		return GameVersionFFX, p1, true
	}
	return GameVersion{}, "", false
}

func checkDataVersion2(path string) (GameVersion, string, bool) {
	p1 := filepath.Join("ffx-2_data")
	p2 := filepath.Join("gamedata")
	p3 := filepath.Join("ps3data")
	if hasExactComponent(path, p1) && hasExactComponent(path, p2) && hasExactComponent(path, p3) {
		return GameVersionFFX2, p1, true
	}
	return GameVersion{}, "", false
}

// CheckFFXPath resolves the given path to its absolute form and then validates it
// by running a series of checks for known PS2 and Data version patterns. If one of
// the checks succeeds, the function returns the detected game version along with
// a nil error. If no check passes, it returns ffx and an error indicating that the
// supplied path does not conform to a valid spira us path.
func CheckFFXPath(path string) (GameVersion, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return GameVersionFFX, fmt.Errorf("error getting absolute path: %s", err.Error())
	}

	checks := []func(string) (GameVersion, string, bool){
		checkPS2Version1,
		checkPS2Version2,
		checkDataVersion1,
		checkDataVersion2,
	}
	for _, check := range checks {
		if version, _, ok := check(absPath); ok {
			return version, nil
		}
	}
	return GameVersionFFX, fmt.Errorf("not a valid spira us path: %s", path)
}

// MacroBinaryPath retorna o caminho absoluto do binário do dicionário de
// macros de uma versão/localização
// (ex.: ffx+us -> .../ffx_ps2/ffx/master/new_uspc/menu/macrodic.dcp em mods).
// Versão explícita: lastmiss divide a árvore com o ffx2 e o estado global
// pode divergir do arquivo sendo salvo.
func MacroBinaryPath(version GameVersion, localization string) string {
	rel := filepath.Join("menu", "macrodic.dcp")
	return filepath.Join(GameFilesRoot, ModsFolder, GetLocalizationRootForVersion(version, localization), rel)
}
