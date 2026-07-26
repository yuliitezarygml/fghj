package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"backend/internal/bot"
	"backend/internal/downloader"
	"backend/internal/keys"
	"backend/internal/lua"
	"backend/internal/steam"
)

func main() {
	appIDFlag := flag.String("appid", "", "Steam App ID to download manifests for (e.g. 418370)")
	apiKeyFlag := flag.String("apikey", "", "Server 2 API Key if manifest needs to be fetched from Steam (optional)")
	botTokenFlag := flag.String("token", "8760503467:AAERatqPzmdBhEa5pZFD6irVfMuH_qAfOW0", "Telegram Bot Token")
	botModeFlag := flag.Bool("bot", false, "Run in Telegram Bot mode")
	flag.Parse()

	appID := *appIDFlag
	apiKey := *apiKeyFlag

	fmt.Println("==================================================")
	fmt.Println("    Steam Depot Manifest Downloader (Go CLI)")
	fmt.Println("==================================================")

	// Run in Bot mode by default if no AppID is provided
	if appID == "" {
		*botModeFlag = true
	}

	if *botModeFlag {
		fmt.Printf("[Info] Запуск Телеграм Бота (токен: %s)...\n", *botTokenFlag)
		err := bot.Start(*botTokenFlag, apiKey)
		if err != nil {
			fmt.Printf("[Error] Ошибка запуска бота: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// CLI Downloader Mode
	if appID == "" {
		fmt.Print("Enter Steam App ID: ")
		_, err := fmt.Scanln(&appID)
		if err != nil || strings.TrimSpace(appID) == "" {
			fmt.Println("[Error] Invalid Steam App ID.")
			os.Exit(1)
		}
	}

	appID = strings.TrimSpace(appID)

	fmt.Printf("[Info] Querying details for App ID: %s...\n", appID)
	depots, err := steam.GetAppDepots(appID)
	if err != nil {
		fmt.Printf("[Error] Failed to fetch depots: %v\n", err)
		os.Exit(1)
	}

	if len(depots) == 0 {
		fmt.Println("[Warning] No depots or manifests found for this App ID.")
		os.Exit(0)
	}

	fmt.Printf("[Info] Found %d depot(s) with manifests. Starting downloads...\n\n", len(depots))

	// Create local manifests directory
	outputDir := filepath.Join(".", "manifests", appID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("[Error] Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	fmt.Println("[Info] Attempting to fetch decryption keys from community repositories...")
	decryptionKeys := keys.FetchDecryptionKeys(client, appID)
	if len(decryptionKeys) > 0 {
		fmt.Printf("[Info] Successfully loaded %d decryption key(s).\n\n", len(decryptionKeys))
	} else {
		fmt.Println("[Warning] No decryption keys found in community repositories. Lua script will be generated without depot keys.\n")
	}

	for i, d := range depots {
		filename := fmt.Sprintf("%s_%s.manifest", d.DepotID, d.ManifestID)
		destPath := filepath.Join(outputDir, filename)

		fmt.Printf("[%d/%d] Processing %s...\n", i+1, len(depots), filename)

		// 1. Check if file already exists locally
		if fileInfo, err := os.Stat(destPath); err == nil && fileInfo.Size() > 0 {
			fmt.Printf("   -> [Skipped] Manifest already exists locally (%s, %.2f MB)\n", destPath, float64(fileInfo.Size())/(1024*1024))
			continue
		}

		// 2. Try to download from Github mirror
		success, err := downloader.DownloadFromGithub(client, d.DepotID, d.ManifestID, destPath)
		if success {
			continue
		}

		if err != nil {
			fmt.Printf("   -> Github download failed: %v\n", err)
		}

		// 3. Try to fetch from Supabase edge function proxy if Github failed
		fmt.Println("   -> [Retry] Trying Supabase proxy (Server 2)...")
		success, err = downloader.DownloadFromSupabase(client, d.DepotID, d.ManifestID, apiKey, destPath)
		if success {
			continue
		}

		fmt.Printf("   -> [Failed] Could not download manifest for Depot %s, Manifest %s: %v\n", d.DepotID, d.ManifestID, err)
		if apiKey == "" {
			fmt.Println("      Note: Some manifests require a Server 2 API Key to download from Steam.")
			fmt.Println("      You can pass it with the -apikey flag: go run . -appid <appid> -apikey <key>")
		}
	}

	// Generate the Lua config file for SteamTools / OpenSteamTool
	err = lua.GenerateLuaConfig(outputDir, appID, depots, decryptionKeys)
	if err != nil {
		fmt.Printf("[Warning] Failed to generate Lua config file: %v\n", err)
	} else {
		luaPath := filepath.Join(outputDir, fmt.Sprintf("%s.lua", appID))
		fmt.Printf("[Success] Generated SteamTools Lua config at: %s\n", luaPath)
	}

	fmt.Println("\n==================================================")
	fmt.Printf("Finished processing. Manifests and Lua script saved to:\n%s\n", outputDir)
	fmt.Println("==================================================")
}
