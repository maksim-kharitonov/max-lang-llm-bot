package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

const (
	groqAPIURL = "https://api.groq.com/openai/v1/chat/completions"
	modelName  = "llama-3.3-70b-versatile"
)

type GroqRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ .env не найден — используем переменные окружения")
	}
}

func callGroq(prompt string) (string, error) {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GROQ_API_KEY не задан")
	}

	messages := []Message{
		{
			Role: "system",
			Content: `You are Professor Minerva McGonagall, acting as a kind but no-nonsense English tutor. The user is learning English.

- **Only correct clear grammar, spelling, or serious word-choice errors** — ignore minor stylistic quirks, informal phrasing, or harmless repetitions unless they cause confusion.
- **If a correction is needed, send it as a separate, standalone message** before your main reply. Format it like this:

optional part:
[CORRECTION]
  🔍 *“I goed to park” → “I went to the park.”*  
  *(We use “went” as the past tense of “go,” and “the park” sounds more natural here.)*

- **Then, in a second message**, continue the conversation in your signature tone: calm, precise, slightly formal, quietly encouraging — never condescending. Keep this reply to 1–3 sentences.
- **If there’s no error worth correcting, send only the conversational reply** — no correction message at all.`,
		},
		{Role: "user", Content: prompt},
	}

	reqBody := GroqRequest{
		Model:       modelName,
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   500,
	}

	jsonData, _ := json.Marshal(reqBody)

	httpReq, _ := http.NewRequest("POST", groqAPIURL, bytes.NewBuffer(jsonData))
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ошибка сети: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ошибка Groq API (%d): %s", resp.StatusCode, string(body))
	}

	var groqResp GroqResponse
	if err := json.Unmarshal(body, &groqResp); err != nil {
		return "", fmt.Errorf("не удалось распарсить ответ Groq: %w", err)
	}

	if len(groqResp.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от Groq")
	}

	return groqResp.Choices[0].Message.Content, nil
}

func main() {
	loadEnv()

	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("❌ TELEGRAM_BOT_TOKEN не задан")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal("❌ Ошибка инициализации Telegram бота:", err)
	}

	bot.Debug = false
	log.Printf("✅ Бот запущен как @%s", bot.Self.UserName)

	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatal("❌ WEBHOOK_URL не задан (например: https://123456.ngrok-free.app)")
	}

	webhook, err := tgbotapi.NewWebhook(webhookURL + "/")
	if err != nil {
		log.Fatal("❌ Ошибка создания webhook:", err)
	}
	_, err = bot.Request(webhook)
	if err != nil {
		log.Fatal("❌ Ошибка установки webhook:", err)
	}

	// Проверка текущего webhook (опционально, для отладки)
	if info, err := bot.GetWebhookInfo(); err == nil {
		log.Printf("📡 Текущий webhook URL: %s, pending updates: %d", info.URL, info.PendingUpdateCount)
	} else {
		log.Printf("❌ Текущий webhook URL: %s, pending updates error: %+v", info.URL, err)
	}

	updates := bot.ListenForWebhook("/")
	go func() {
		log.Println("📡 Слушаю порт :1984 для Telegram webhook...")
		log.Fatal(http.ListenAndServe(":1984", nil))
	}()

	for update := range updates {
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		chatID := update.Message.Chat.ID
		text := update.Message.Text
		msgID := update.Message.MessageID

		//log.Printf("📩 От %d: %s", chatID, text)

		response, err := callGroq(text)
		if err != nil {
			log.Printf("⚠️ Ошибка Groq: %v", err)
			response = "Sorry, I couldn't process that. Try again?"
		}

		msg := tgbotapi.NewMessage(chatID, response)
		msg.ReplyToMessageID = msgID
		msg.ParseMode = "Markdown" // опционально, если Groq вернёт Markdown
		_, _ = bot.Send(msg)
	}
}
