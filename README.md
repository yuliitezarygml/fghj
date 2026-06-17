# Steam Depot Manifest Downloader

Steam Depot Manifest Downloader bot written in Go.

## Features
- Fully automated Steam Depot manifests downloading
- Automatically retrieves decryption keys from community VDFs
- Generates SteamTools / OpenSteamTool Lua config files
- Serves as a Telegram Bot: send a Steam URL or App ID, get a ZIP with all manifests and the config file
- Ready for Docker & Docker Compose deployment

## Quick Start on Linux
```bash
docker compose up -d --build
```
