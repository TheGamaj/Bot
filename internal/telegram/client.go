// Package telegram is a minimal Telegram Bot API client for Gamaj Bot.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org"

// Update is a Telegram incoming update (only fields Gamaj Bot consumes).
type Update struct {
	UpdateID int64 `json:"update_id"`
	Message  *Message
	Callback *struct {
		ID      string `json:"id"`
		Data    string `json:"data"`
		From    User   `json:"from"`
		Message *Message
	} `json:"callback_query"`
}

// Message is an incoming Telegram message.
type Message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      User   `json:"chat"`
	From      *User  `json:"from"`
}

// User identifies a Telegram user or chat.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

// Client sends Telegram Bot API requests with retry on rate limits.
type Client struct {
	token string
	http  *http.Client
}

// New creates a Telegram client for the given bot token.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// Call performs a Telegram Bot API method call.
func (c *Client) Call(ctx context.Context, method string, params url.Values, target any) error {
	endpoint := apiBase + "/bot" + c.token + "/" + method
	body := ""
	if params != nil {
		body = params.Encode()
	}
	for attempt := 0; attempt < 3; attempt++ {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, reader)
		if err != nil {
			return errors.New("prepare Telegram request")
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		res, err := c.http.Do(req)
		if err != nil {
			return errors.New("Telegram API request failed")
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			wait := time.Second
			if seconds, err := strconv.Atoi(strings.TrimSpace(res.Header.Get("Retry-After"))); err == nil && seconds > 0 && seconds < 30 {
				wait = time.Duration(seconds) * time.Second
			}
			res.Body.Close()
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		defer res.Body.Close()
		var payload apiResponse
		if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload); err != nil {
			return errors.New("Telegram API returned an invalid response")
		}
		if !payload.OK {
			return fmt.Errorf("Telegram API: %s", payload.Description)
		}
		if target != nil && len(payload.Result) > 0 {
			return json.Unmarshal(payload.Result, target)
		}
		return nil
	}
	return errors.New("Telegram API rate limited")
}

// SendMessage delivers a text message with an optional inline keyboard.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text, keyboard string) error {
	params := url.Values{
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"text":    {text},
	}
	if keyboard != "" {
		params.Set("reply_markup", keyboard)
	}
	return c.Call(ctx, "sendMessage", params, nil)
}

// AnswerCallback acknowledges a callback query.
func (c *Client) AnswerCallback(ctx context.Context, callbackID string) error {
	return c.Call(ctx, "answerCallbackQuery", url.Values{"callback_query_id": {callbackID}}, nil)
}

// SetWebhook registers the webhook endpoint with a secret token.
func (c *Client) SetWebhook(ctx context.Context, endpoint, secret string) error {
	return c.Call(ctx, "setWebhook", url.Values{
		"url":                  {endpoint},
		"secret_token":         {secret},
		"drop_pending_updates": {"false"},
	}, nil)
}

// SetChatMenuButton configures the chat input placeholder that every Gamaj
// bot shows inside the Telegram message composer.
func (c *Client) SetChatMenuButton(ctx context.Context) error {
	button := map[string]any{
		"type": "default",
		"text": "پیام خود را بنویسید…",
	}
	encoded, err := json.Marshal(button)
	if err != nil {
		return err
	}
	return c.Call(ctx, "setChatMenuButton", url.Values{"menu_button": {string(encoded)}}, nil)
}

// GetUpdates polls updates with long polling (webhook-less deployments).
func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	params := url.Values{
		"timeout": {"25"},
		"offset":  {strconv.FormatInt(offset, 10)},
	}
	err := c.Call(ctx, "getUpdates", params, &updates)
	return updates, err
}

// MarshalKeyboard encodes an inline keyboard payload.
func MarshalKeyboard(markup any) string {
	data, err := json.Marshal(markup)
	if err != nil {
		return ""
	}
	return string(data)
}

// KeyboardRow builds one row of inline buttons.
func KeyboardRow(buttons ...map[string]string) []map[string]string {
	return buttons
}

// Button creates an inline button description.
func Button(text, data string) map[string]string {
	return map[string]string{"text": text, "callback_data": data}
}

// ButtonURL creates an inline button that opens a URL.
func ButtonURL(text, url string) map[string]string {
	return map[string]string{"text": text, "url": url}
}

var _ = bytes.MinRead // keep import stable for future multipart use
