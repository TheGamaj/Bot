// Package web hosts the Gamaj Bot webhook receiver. Telegram updates are only
// accepted when they carry the installation's secret token.
package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/TheGamaj/Bot/internal/botsales"
	"github.com/TheGamaj/Bot/internal/miniapp"
	"github.com/TheGamaj/Bot/internal/telegram"
)

// Updater processes a single Telegram update.
type Updater interface {
	Start(ctx context.Context, chatID int64)
	Plans(ctx context.Context, chatID int64)
	CreateOrder(ctx context.Context, chatID, planID int64)
	PayWithGateway(ctx context.Context, chatID int64, orderID string)
	PayWithWallet(ctx context.Context, chatID int64, orderID string)
	Verify(ctx context.Context, chatID int64, orderID, payID string)
	MyServices(ctx context.Context, chatID int64)
}

type webhookServer struct {
	secret string
	sales  *botsales.Handlers
	client *telegram.Client
}

// Handler builds the bot HTTP mux: /webhook for Telegram plus /healthz and
// the Gamaj Bot Mini App on /miniapp.
func Handler(secret string, sales *botsales.Handlers, client *telegram.Client) http.Handler {
	mux := http.NewServeMux()
	server := &webhookServer{secret: secret, sales: sales, client: client}
	mux.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.Handle("/miniapp", miniapp.Handler())
	mux.Handle("/miniapp/", miniapp.Handler())
	mux.HandleFunc("/webhook", server.handleWebhook)
	return mux
}

func (s *webhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.secret == "" {
		http.Error(w, "webhook is not configured", http.StatusServiceUnavailable)
		return
	}
	actual := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if len(actual) != len(s.secret) || subtle.ConstantTimeCompare([]byte(actual), []byte(s.secret)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	var update telegram.Update
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&update); err != nil {
		http.Error(w, "invalid update", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	// Respond to Telegram first, then process the update.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.process(ctx, update)
	}()
}

func (s *webhookServer) process(ctx context.Context, update telegram.Update) {
	if update.Message != nil && update.Message.Text != "" {
		chatID := update.Message.Chat.ID
		switch update.Message.Text {
		case "/start":
			s.sales.Start(ctx, chatID)
		case "/plans", "plans":
			s.sales.Plans(ctx, chatID)
		case "/services", "myservices":
			s.sales.MyServices(ctx, chatID)
		default:
			s.sales.Start(ctx, chatID)
		}
		return
	}
	if update.Callback != nil {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		chatID := update.Callback.From.ID
		data := update.Callback.Data
		_ = s.client.AnswerCallback(ctx, update.Callback.ID)
		switch {
		case data == "start":
			s.sales.Start(ctx, chatID)
		case data == "plans":
			s.sales.Plans(ctx, chatID)
		case data == "myservices":
			s.sales.MyServices(ctx, chatID)
		case len(data) > 5 && data[:5] == "plan:":
			planID, err := parseInt64(data[5:])
			if err == nil {
				s.sales.CreateOrder(ctx, chatID, planID)
			}
		case len(data) > 4 && data[:4] == "pay:":
			s.sales.PayWithGateway(ctx, chatID, data[4:])
		case len(data) > 10 && data[:10] == "paywallet:":
			s.sales.PayWithWallet(ctx, chatID, data[10:])
		case len(data) > 7 && data[:7] == "verify:":
			orderID, payID, ok := splitTwo(data[7:], ":")
			if ok {
				s.sales.Verify(ctx, chatID, orderID, payID)
			}
		}
		return
	}
}

func parseInt64(value string) (int64, error) {
	var result int64
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid number")
		}
		result = result*10 + int64(char-'0')
	}
	return result, nil
}

// splitTwo splits "a:b" at the first separator.
func splitTwo(value, sep string) (string, string, bool) {
	for i := 0; i+len(sep) <= len(value); i++ {
		if value[i:i+len(sep)] == sep {
			return value[:i], value[i+len(sep):], true
		}
	}
	return "", "", false
}

var _ = log.Printf // keep log import for operator diagnostics
