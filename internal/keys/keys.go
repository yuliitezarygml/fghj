package keys

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
)

var autoupdateRepos = []string{
	"tymolu233/ManifestAutoUpdate-fix",
	"Auiowu/ManifestAutoUpdate",
	"ikun0014/ManifestHub",
	"SteamAutoCracks/ManifestHub",
}

// FetchDecryptionKeys tries to download Key.vdf or config.vdf for the appID from the AutoUpdate repositories
func FetchDecryptionKeys(client *http.Client, appID string) map[string]string {
	keys := make(map[string]string)
	vdfNames := []string{"Key.vdf", "key.vdf", "config.vdf"}

	// Regex to extract depot ID and DecryptionKey from VDF: "depotID" { "DecryptionKey" "HexKey" }
	re := regexp.MustCompile(`(?s)"(\d+)"\s*\{\s*"DecryptionKey"\s*"([a-fA-F0-9]+)"`)

	for _, repo := range autoupdateRepos {
		for _, vdfName := range vdfNames {
			url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo, appID, vdfName)
			resp, err := client.Get(url)
			if err != nil {
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					continue
				}
				content := string(bodyBytes)
				matches := re.FindAllStringSubmatch(content, -1)
				for _, match := range matches {
					if len(match) == 3 {
						depotID := match[1]
						key := match[2]
						keys[depotID] = key
					}
				}
				if len(keys) > 0 {
					fmt.Printf("[Info] Found decryption key(s) in %s/%s/%s\n", repo, appID, vdfName)
					return keys
				}
			}
		}
	}
	return keys
}
