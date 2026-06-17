package steam

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Manifest structures for SteamCMD response parsing
type Manifest struct {
	Gid string `json:"gid"`
}

type Depot struct {
	Manifests map[string]Manifest `json:"manifests"`
}

type AppData struct {
	Depots map[string]json.RawMessage `json:"depots"`
}

type SteamCmdResponse struct {
	Status string             `json:"status"`
	Data   map[string]AppData `json:"data"`
}

// DepotInfo represents a single resolved depot with its manifest GID
type DepotInfo struct {
	DepotID    string
	ManifestID string
}

// AppDetails holds store details for a Steam app
type AppDetails struct {
	Name             string
	HeaderImage      string
	ShortDescription string
	Developers       []string
	Publishers       []string
	ReleaseDate      string
}

// GetAppDepots fetches depot list from steamcmd.net API and resolves the manifests
func GetAppDepots(appID string) ([]DepotInfo, error) {
	url := fmt.Sprintf("https://api.steamcmd.net/v1/info/%s", appID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steamcmd api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data SteamCmdResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	appInfo, exists := data.Data[appID]
	if !exists {
		return nil, fmt.Errorf("app ID not found in API response")
	}

	var resolved []DepotInfo

	for depotID, rawDepot := range appInfo.Depots {
		var d Depot
		if err := json.Unmarshal(rawDepot, &d); err != nil {
			// Skip fields like "baselanguages", "branches", etc. that are not structured Depots
			continue
		}
		if len(d.Manifests) == 0 {
			continue
		}

		manifestID := ""
		// Prioritize "public" branch
		if pub, ok := d.Manifests["public"]; ok && pub.Gid != "" {
			manifestID = pub.Gid
		} else {
			// Fallback to first available manifest ID
			for _, m := range d.Manifests {
				if m.Gid != "" {
					manifestID = m.Gid
					break
				}
			}
		}

		if manifestID != "" {
			resolved = append(resolved, DepotInfo{
				DepotID:    depotID,
				ManifestID: manifestID,
			})
		}
	}

	return resolved, nil
}

// GetAppDetails fetches store information for an appID from the Steam Store API
func GetAppDetails(appID string) (*AppDetails, error) {
	url := fmt.Sprintf("https://store.steampowered.com/api/appdetails?appids=%s&l=russian", appID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("steam store api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Name             string   `json:"name"`
			HeaderImage      string   `json:"header_image"`
			ShortDescription string   `json:"short_description"`
			Developers       []string `json:"developers"`
			Publishers       []string `json:"publishers"`
			ReleaseDate      struct {
				ComingSoon bool   `json:"coming_soon"`
				Date       string `json:"date"`
			} `json:"release_date"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	appInfo, exists := data[appID]
	if !exists || !appInfo.Success {
		return nil, fmt.Errorf("app details not found or success is false")
	}

	details := &AppDetails{
		Name:             appInfo.Data.Name,
		HeaderImage:      appInfo.Data.HeaderImage,
		ShortDescription: appInfo.Data.ShortDescription,
		Developers:       appInfo.Data.Developers,
		Publishers:       appInfo.Data.Publishers,
		ReleaseDate:      appInfo.Data.ReleaseDate.Date,
	}

	return details, nil
}
