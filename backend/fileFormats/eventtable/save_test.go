package eventtable_test

import (
	"os"
	"path/filepath"
	"testing"

	"ffxresources/testData/tempdir"
	"ffxresources/backend/common"
	"ffxresources/backend/fileFormats/eventtable"
	testcommon "ffxresources/testData"
)

// Cloud: Save sempre regrava bin+bin — mesmo quando só um foi editado. A
// store lê os dois juntos (id 'cloud' único), edita e salva os dois.
func TestCloudSaveWritesBothBins(t *testing.T) {
	root := filepath.Dir(testcommon.GetTestDataRootDirectory())
	dataRoot := filepath.Join(root, "build", "bin", "data")
	if _, err := os.Stat(dataRoot); err != nil {
		t.Skip("build/bin/data indisponível")
	}
	prev := common.GameFilesRoot
	dataPath := dataRoot
	common.GameFilesRoot = dataPath
	defer func() { common.GameFilesRoot = prev }()

	f, err := eventtable.Load(eventtable.KindCloud, common.GameVersionFFX2, "cloud")
	if err != nil {
		t.Skip("cloud fixture ausente:", err)
	}
	if len(f.Bins) != 1 {
		t.Fatalf("cloud deve carregar 1 bin, tem %d", len(f.Bins))
	}
	fv, err := eventtable.Load(eventtable.KindCloud, common.GameVersionFFX2, "cloudv")
	if err != nil {
		t.Skip("cloudv fixture ausente:", err)
	}

	tmp := tempdir.TempDir(t)
	common.GameFilesRoot = tmp
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if err := fv.Save(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cloud.bin", "cloudv.bin"} {
		p := filepath.Join(tmp, "mods", "ffx_ps2", "ffx2", "master", "new_uspc", "cloudsave", name)
		fi, err := os.Stat(p)
		if err != nil || fi.Size() < 64 {
			t.Errorf("binário ausente ou vazio: %s (%v)", p, err)
		}
	}
}
