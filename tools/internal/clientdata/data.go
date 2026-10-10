// Package clientdata shares the catalogue tools' local reference-data location
// and pinned upstream versions. Only the fetch command accesses the network.
package clientdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	Directory     = "tmp/client-data"
	ReleaseURL    = "https://github.com/wowgaming/client-data/releases/tag/v20.0"
	ArchiveURL    = "https://github.com/wowgaming/client-data/releases/download/v20.0/Data.zip"
	CoreRevision  = "d80ce1d87720e6b6a0b9adc952f21a658b1c245e"
	ClassesURL    = "https://raw.githubusercontent.com/azerothcore/azerothcore-wotlk/" + CoreRevision + "/data/sql/base/db_world/playercreateinfo.sql"
	OpcodesURL    = "https://raw.githubusercontent.com/azerothcore/azerothcore-wotlk/" + CoreRevision + "/src/server/game/Server/Protocol/Opcodes.h"
	OpcodesSHA256 = "9109ea4d2abd098883acd1e6b552104adf8965544478439811c75426315aa447"
)

// WoW 1.12.1 (build 5875) references for cMaNGOS Classic. Opcodes.h is public;
// the DBC tables and playercreateinfo.tsv come from a server's own data:
//
//	cp <mangosd DataDir>/dbc/{CharSections,CharHairGeosets,CharacterFacialHairStyles,Map,AreaTable}.dbc tmp/client-data-5875/
//	mariadb -N -e 'SELECT race, class FROM playercreateinfo ORDER BY race, class' <world db> > tmp/client-data-5875/playercreateinfo.tsv
//
// The generators verify every file against the checksums below.
const (
	ClassicDirectory      = "tmp/client-data-5875"
	ClassicCoreRevision   = "8ec338a1704e7dcb1c0213eb7ed58f9231ade40f"
	ClassicOpcodesURL     = "https://raw.githubusercontent.com/cmangos/mangos-classic/" + ClassicCoreRevision + "/src/game/Server/Opcodes.h"
	ClassicOpcodesSHA256  = "0f5969ed6e30df7cd07186add12cf86d1cb6eed0fb4a055d3f1ed4f7269a7d60"
	ClassicSectionsSHA256 = "376b24e03742fa019053dd58519769442eb09d7ea68ab9dcb305a321710cce26"
	ClassicHairSHA256     = "cc09d9a8b82cc65817283304a6fff8e8e80a74d0d680795f66b79d8b6a8cc7e0"
	ClassicFacialSHA256   = "4acf3a4186a45b1113782b19ad8743f9f89cde6d3e15b3927319537ab42ca6ad"
	ClassicMapSHA256      = "4bb6b53841b113eb0ec4fa624ef42d7de7ebdd5169aac17d3e6f9a4f73654245"
	ClassicAreaSHA256     = "ac4c33c2ae65d37ab801050adf489ea5286d7dd9f9ebf78e6e5c7120418a3528"
	ClassicClassesSHA256  = "9ddef03b1286388bb357d1f1ecb13f5ba02e65e0427c8a16843f33e68a658937"
)

// Read reads a local reference file, explaining how to fetch missing data.
func Read(directory, name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(directory, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("client data is missing; run './run client-data:fetch' first: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("read client data %s: %w", name, err)
	}
	return data, nil
}
