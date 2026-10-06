// Gamaj Bot is the official Telegram bot of the Gamaj ecosystem. It is a
// standalone Go binary with no database access: every operation goes through
// the Gamaj API with the bot's dedicated API key.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TheGamaj/Bot/internal/botpanel"
	"github.com/TheGamaj/Bot/internal/botsales"
	"github.com/TheGamaj/Bot/internal/config"
	"github.com/TheGamaj/Bot/internal/panel"
	"github.com/TheGamaj/Bot/internal/telegram"
	"github.com/TheGamaj/Bot/internal/web"
)

func main() {
	configPath := flag.String("config", ".gamaj-bot.json", "path to the Gamaj Bot configuration file")
	setupWebhook := flag.Bool("setup-webhook", false, "register the Telegram webhook from the configuration and exit")
	showVersion := flag.Bool("version", false, "print the Gamaj Bot version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("Gamaj Bot " + config.Version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("gamajbot: %v", err)
	}

	bot := telegram.New(cfg.BotToken)
	api := panel.New(cfg.APIBase(), cfg.APIKey)
	sales := botsales.New(cfg, api, bot)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *setupWebhook {
		if cfg.WebhookBaseURL == "" {
			log.Fatal("gamajbot: webhook_base_url is not set in the configuration")
		}
		if err := bot.SetWebhook(ctx, cfg.WebhookBaseURL+"/webhook", cfg.WebhookSecret); err != nil {
			log.Fatalf("gamajbot: set webhook: %v", err)
		}
		// The shared Gamaj chat-input placeholder for all Gamaj bots.
		if err := bot.SetChatMenuButton(ctx); err != nil {
			log.Printf("gamajbot: set chat menu button: %v", err)
		}
		fmt.Println("Gamaj Bot webhook registered:", cfg.WebhookBaseURL+"/webhook")
		return
	}

	if err := api.Ping(ctx); err != nil {
		log.Printf("gamajbot: warning: Gamaj API check failed: %v", err)
	}

	panelTarget, err := url.Parse(cfg.PanelURL)
	if err != nil {
		log.Fatalf("gamajbot: invalid panel_url: %v", err)
	}

	address := cfg.ListenHost + ":" + fmt.Sprint(cfg.ListenPort)
	// The webhook receiver owns /, /healthz, /miniapp and /webhook; the Bot
	// Panel owns /panel and proxies its data calls to the Gamaj API.
	root := http.NewServeMux()
	panelHandler := botpanel.Handler(cfg.AdminID, botpanel.NewAPIProxy(panelTarget, cfg.APIKey))
	root.Handle("/panel", panelHandler)
	root.Handle("/panel/", panelHandler)
	root.Handle("/", web.Handler(cfg.WebhookSecret, sales, bot))

	server := &http.Server{
		Addr:              address,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("gamajbot: Gamaj Bot %s listening on %s", config.Version, address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("gamajbot: server: %v", err)
	}
}
