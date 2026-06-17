package bot

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"backend/internal/downloader"
	"backend/internal/keys"
	"backend/internal/lua"
	"backend/internal/steam"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Start initializes and runs the Telegram bot long-polling loop
func Start(token string, apiKey string) error {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return fmt.Errorf("failed to start Telegram bot: %w", err)
	}

	fmt.Printf("[Bot] Authorized on account %s\n", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Handle text messages
		if update.Message.Text != "" {
			go handleMessage(bot, update.Message, apiKey)
		}
	}

	return nil
}

func handleMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, apiKey string) {
	chatID := msg.Chat.ID
	appID := extractAppID(msg.Text)

	if appID == "" {
		reply := "Привет! Отправь мне ID игры Steam (например, `1938090`) или ссылку на игру в магазине Steam (например, `https://store.steampowered.com/app/1938090/Call_of_Duty/`), и я скачаю манифесты и сгенерирую Lua-конфиг для SteamTools."
		sendTextMessage(bot, chatID, reply, msg.MessageID)
		return
	}

	// 1. Fetch game details
	details, err := steam.GetAppDetails(appID)
	if err != nil {
		// Fallback to text message if details can't be fetched
		sendTextMessage(bot, chatID, fmt.Sprintf("🔍 Получен App ID: %s. Не удалось загрузить информацию из Steam Store. Начинаю обработку...", appID), msg.MessageID)
	} else {
		// Format caption
		desc := details.ShortDescription
		if len(desc) > 300 {
			desc = desc[:300] + "..."
		}
		caption := fmt.Sprintf(
			"🎮 **%s** (App ID: %s)\n\n"+
				"📅 **Дата выхода:** %s\n"+
				"💻 **Разработчик:** %s\n"+
				"🏢 **Издатель:** %s\n\n"+
				"📝 **Описание:** %s\n\n"+
				"⚙️ _Начинаю скачивание манифестов..._",
			details.Name, appID,
			details.ReleaseDate,
			strings.Join(details.Developers, ", "),
			strings.Join(details.Publishers, ", "),
			desc,
		)
		sendPhotoMessage(bot, chatID, details.HeaderImage, caption, msg.MessageID)
	}

	// 2. Fetch depots
	depots, err := steam.GetAppDepots(appID)
	if err != nil {
		sendTextMessage(bot, chatID, fmt.Sprintf("❌ Ошибка получения списка депо: %v", err), msg.MessageID)
		return
	}

	if len(depots) == 0 {
		sendTextMessage(bot, chatID, "⚠️ Для данного App ID не найдено депо или манифестов.", msg.MessageID)
		return
	}

	// Create directories
	outputDir := filepath.Join(".", "manifests", appID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		sendTextMessage(bot, chatID, fmt.Sprintf("❌ Ошибка создания папки для сохранения: %v", err), msg.MessageID)
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Fetch keys
	decryptionKeys := keys.FetchDecryptionKeys(client, appID)
	keysCount := len(decryptionKeys)

	var downloadedCount int
	var failedCount int
	var skippedCount int

	for _, d := range depots {
		filename := fmt.Sprintf("%s_%s.manifest", d.DepotID, d.ManifestID)
		destPath := filepath.Join(outputDir, filename)

		// Check local file
		if fileInfo, err := os.Stat(destPath); err == nil && fileInfo.Size() > 0 {
			skippedCount++
			continue
		}

		// Try Github
		success, _ := downloader.DownloadFromGithub(client, d.DepotID, d.ManifestID, destPath)
		if success {
			downloadedCount++
			continue
		}

		// Try Supabase
		success, _ = downloader.DownloadFromSupabase(client, d.DepotID, d.ManifestID, apiKey, destPath)
		if success {
			downloadedCount++
			continue
		}

		failedCount++
	}

	// Generate Lua
	err = lua.GenerateLuaConfig(outputDir, appID, depots, decryptionKeys)
	if err != nil {
		sendTextMessage(bot, chatID, fmt.Sprintf("⚠️ Ошибка при генерации Lua-файла: %v", err), msg.MessageID)
		return
	}

	// Create zip archive
	zipPath := filepath.Join(outputDir, fmt.Sprintf("%s_manifests.zip", appID))
	err = zipDirectory(outputDir, zipPath)
	if err != nil {
		sendTextMessage(bot, chatID, fmt.Sprintf("⚠️ Ошибка при создании ZIP-архива: %v. Отправляю только Lua-файл.", err), msg.MessageID)
		luaPath := filepath.Join(outputDir, fmt.Sprintf("%s.lua", appID))
		sendDocument(bot, chatID, luaPath, msg.MessageID)
		return
	}

	// Format final message
	statusText := fmt.Sprintf(
		"✅ **Обработка завершена!**\n\n"+
			"🎮 **App ID:** %s\n"+
			"📦 **Всего депо:** %d\n"+
			"🔑 **Найдено ключей:** %d\n"+
			"⬇️ **Скачано новых манифестов:** %d\n"+
			"⏭️ **Пропущено (уже были):** %d\n"+
			"❌ **Не удалось скачать:** %d",
		appID, len(depots), keysCount, downloadedCount, skippedCount, failedCount,
	)

	sendTextMessage(bot, chatID, statusText, msg.MessageID)

	// Send ZIP file containing manifests and Lua config
	sendDocument(bot, chatID, zipPath, msg.MessageID)

	// Clean up zip file from local disk to save space
	os.Remove(zipPath)
}

func extractAppID(input string) string {
	input = strings.TrimSpace(input)

	// Match pattern store.steampowered.com/app/12345/
	re := regexp.MustCompile(`(?:/app/|/appID/|/sub/)?(\d+)`)
	matches := re.FindAllStringSubmatch(input, -1)
	for _, match := range matches {
		if len(match) > 1 {
			return match[1]
		}
	}

	// Fallback to pure numeric check
	reNum := regexp.MustCompile(`^\d+$`)
	if reNum.MatchString(input) {
		return input
	}

	return ""
}

func sendTextMessage(bot *tgbotapi.BotAPI, chatID int64, text string, replyToMessageID int) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyToMessageID
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

func sendPhotoMessage(bot *tgbotapi.BotAPI, chatID int64, photoURL string, caption string, replyToMessageID int) {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
	photo.Caption = caption
	photo.ParseMode = "Markdown"
	photo.ReplyToMessageID = replyToMessageID
	bot.Send(photo)
}

func sendDocument(bot *tgbotapi.BotAPI, chatID int64, filePath string, replyToMessageID int) {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	doc.ReplyToMessageID = replyToMessageID
	_, err := bot.Send(doc)
	if err != nil {
		fmt.Printf("[Bot] Failed to send document %s: %v\n", filePath, err)
	}
}

func zipDirectory(sourceDir, zipFile string) error {
	archive, err := os.Create(zipFile)
	if err != nil {
		return err
	}
	defer archive.Close()

	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Don't include other zip files (like the one we are creating)
		if strings.HasSuffix(path, ".zip") {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(writer, file)
		return err
	})

	return err
}
