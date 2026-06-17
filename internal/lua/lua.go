package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"backend/internal/steam"
)

// GenerateLuaConfig creates the Lua configuration file for SteamTools/OpenSteamTool.
func GenerateLuaConfig(outputDir string, appID string, depots []steam.DepotInfo, decryptionKeys map[string]string) error {
	var luaContent strings.Builder
	luaContent.WriteString(fmt.Sprintf("addappid(%s)\n", appID))

	for _, d := range depots {
		// If we found a decryption key for this depot, write it
		if key, ok := decryptionKeys[d.DepotID]; ok {
			luaContent.WriteString(fmt.Sprintf("addappid(%s, 1, \"%s\")\n", d.DepotID, key))
		}
		// Write the manifest ID mapping
		luaContent.WriteString(fmt.Sprintf("setManifestid(%s, \"%s\", 0)\n", d.DepotID, d.ManifestID))
	}

	luaPath := filepath.Join(outputDir, fmt.Sprintf("%s.lua", appID))
	return os.WriteFile(luaPath, []byte(luaContent.String()), 0644)
}
