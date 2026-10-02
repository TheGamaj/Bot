// Package config loads the Gamaj Bot secret configuration file. The same file
// is written by the Gamaj Panel installer, so an install from the Panel is
// auto-configured without any manual steps.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Version is the Gamaj Bot release version (is.0.0.1).
const Version = "is.0.0.1"

// Config is the bot runtime configuration. It contains database credentials
// by design: Gamaj Bot never talks to a database, only to the Gamaj API.
type Config struct {
	PanelURL       string `json:"panel_url"`
	APIKey         string `json:"api_key"`
	BotToken       string `json:"bot_token"`
	AdminID        string `json:"admin_id"`
	WebhookSecret  string `json:"webhook_secret"`
	ListenHost     string `json:"listen_host"`
	ListenPort     int    `json:"listen_port"`
	WebhookBaseURL string `json:"webhook_base_url,omitempty"`
}

// Load reads and validates the configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Gamaj Bot configuration: %w", err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, errors.New("Gamaj Bot configuration is not valid JSON")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

// Validate enforces required fields and safe values.
func (c *Config) Validate() error {
	c.PanelURL = strings.TrimRight(strings.TrimSpace(c.PanelURL), "/")
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.BotToken = strings.TrimSpace(c.BotToken)
	c.AdminID = strings.TrimSpace(c.AdminID)
	c.WebhookSecret = strings.TrimSpace(c.WebhookSecret)
	if c.PanelURL == "" {
		return errors.New("panel_url is required in the Gamaj Bot configuration")
	}
	parsed, err := url.Parse(c.PanelURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return errors.New("panel_url must be a valid http(s) URL")
	}
	if c.APIKey == "" {
		return errors.New("api_key is required in the Gamaj Bot configuration")
	}
	if c.BotToken == "" {
		return errors.New("bot_token is required in the Gamaj Bot configuration")
	}
	if _, err := strconv.ParseInt(c.AdminID, 10, 64); err != nil {
		return errors.New("admin_id must be a numeric Telegram ID")
	}
	if c.ListenHost == "" {
		c.ListenHost = "127.0.0.1"
	}
	if c.ListenPort == 0 {
		c.ListenPort = 8080
	}
	return nil
}

// APIBase returns the base URL for Gamaj API calls.
func (c *Config) APIBase() string {
	return c.PanelURL + "/api"
}
