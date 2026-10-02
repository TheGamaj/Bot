// Package botsales implements the Gamaj Bot user-facing flows:
// start → plans → order → payment → verification → provisioning →
// credentials → Telegram delivery → my services → wallet. All state lives in
// the Gamaj Panel; the bot itself is stateless and database-free.
package botsales

import (
	"context"
	"fmt"
	"strconv"

	"github.com/TheGamaj/Bot/internal/config"
	"github.com/TheGamaj/Bot/internal/panel"
	"github.com/TheGamaj/Bot/internal/telegram"
)

// Handlers wires the Telegram client to the Gamaj API for user flows.
type Handlers struct {
	cfg     *config.Config
	panel   *panel.Client
	bot     *telegram.Client
	adminID int64
}

// New creates sales handlers.
func New(cfg *config.Config, api *panel.Client, bot *telegram.Client) *Handlers {
	adminID, _ := strconv.ParseInt(cfg.AdminID, 10, 64)
	return &Handlers{cfg: cfg, panel: api, bot: bot, adminID: adminID}
}

// inputPlaceholder is the shared Gamaj chat-input placeholder shown in every
// Gamaj bot's message composer.
const inputPlaceholder = "پیام خود را بنویسید…"

// Start greets a user and shows the main menu.
func (h *Handlers) Start(ctx context.Context, chatID int64) {
	text := "🚀 به **گمج بات** خوش آمدید!\n\n" +
		"از منوی زیر استفاده کنید:\n" +
		"🛒 خرید سرویس — مشاهده و خرید پلن‌ها\n" +
		"👤 سرویس‌های من — مشخصات و لینک سرویس\n" +
		"💼 کیف پول — موجودی و شارژ\n" +
		"📞 پشتیبانی — ارتباط با پشتیبانی"
	buttons := [][]map[string]string{
		telegram.KeyboardRow(
			telegram.Button("🛒 خرید سرویس", "plans"),
			telegram.Button("👤 سرویس‌های من", "myservices"),
		),
		telegram.KeyboardRow(
			telegram.Button("💼 کیف پول", "wallet"),
			telegram.Button("📞 پشتیبانی", "support"),
		),
	}
	_ = h.bot.SendMessage(ctx, chatID, text, telegram.MarshalKeyboard(buttons))
}

// Plans lists purchasable plans from the Gamaj API.
func (h *Handlers) Plans(ctx context.Context, chatID int64) {
	plans, err := h.panel.Plans(ctx)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "خطا در دریافت پلن‌ها، کمی بعد دوباره تلاش کنید.", "")
		return
	}
	if len(plans) == 0 {
		_ = h.bot.SendMessage(ctx, chatID, "در حال حاضر پلنی برای فروش موجود نیست.", "")
		return
	}
	var buttons [][]map[string]string
	var list string
	for _, plan := range plans {
		label := fmt.Sprintf("%s — %d تومان", plan.Name, plan.Price)
		list += "▪️ " + label
		if plan.DataLimit > 0 {
			list += fmt.Sprintf(" | %s", panel.FormatBytes(plan.DataLimit))
		} else {
			list += " | نامحدود"
		}
		list += "\n"
		buttons = append(buttons, telegram.KeyboardRow(
			telegram.Button(label, "plan:"+strconv.FormatInt(plan.ID, 10)),
		))
	}
	buttons = append(buttons, telegram.KeyboardRow(telegram.Button("بازگشت", "start")))
	_ = h.bot.SendMessage(ctx, chatID, "🛒 پلن‌های موجود:\n\n"+list+"\nیکی را انتخاب کنید:", telegram.MarshalKeyboard(buttons))
}

// CreateOrder creates an order and offers wallet or gateway payment.
func (h *Handlers) CreateOrder(ctx context.Context, chatID, planID int64) {
	order, err := h.panel.CreateOrder(ctx, planID, chatID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "خطا در ثبت سفارش: "+err.Error(), "")
		return
	}
	text := fmt.Sprintf("🧾 سفارش ثبت شد\n\nپلن: %s\nمبلغ: %d تومان\nکد سفارش: %s\n\nروش پرداخت را انتخاب کنید:", order.PlanName, order.Amount, order.ID)
	buttons := [][]map[string]string{
		telegram.KeyboardRow(telegram.Button("💳 پرداخت آنلاین", "pay:"+order.ID)),
		telegram.KeyboardRow(telegram.Button("💼 پرداخت از کیف پول", "paywallet:"+order.ID)),
		telegram.KeyboardRow(telegram.Button("بازگشت", "plans")),
	}
	_ = h.bot.SendMessage(ctx, chatID, text, telegram.MarshalKeyboard(buttons))
}

// PayWithGateway requests a Tetraminator payment link through the Gamaj API.
func (h *Handlers) PayWithGateway(ctx context.Context, chatID int64, orderID string) {
	link, err := h.panel.CreatePaymentLink(ctx, orderID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "خطا در ساخت لینک پرداخت: "+err.Error(), "")
		return
	}
	text := "💳 برای پرداخت روی دکمه زیر بزنید.\nپس از پرداخت، سرویس به‌صورت خودکار ساخته و ارسال می‌شود."
	buttons := [][]map[string]string{
		{telegram.ButtonURL("💳 پرداخت", link.PaymentURL)},
		telegram.KeyboardRow(telegram.Button("بازگشت", "plans")),
	}
	_ = h.bot.SendMessage(ctx, chatID, text, telegram.MarshalKeyboard(buttons))
}

// PayWithWallet settles the order from wallet balance.
func (h *Handlers) PayWithWallet(ctx context.Context, chatID int64, orderID string) {
	order, err := h.panel.PayWithWallet(ctx, orderID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "پرداخت از کیف پول ناموفق بود: "+err.Error(), "")
		return
	}
	h.deliver(ctx, chatID, order.ID)
}

// Verify confirms a gateway payment through the Gamaj API inquiry before
// delivery, then provisions and delivers credentials.
func (h *Handlers) Verify(ctx context.Context, chatID int64, orderID, payID string) {
	order, err := h.panel.VerifyPayment(ctx, orderID, payID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "تأیید پرداخت ناموفق بود: "+err.Error(), "")
		return
	}
	h.deliver(ctx, chatID, order.ID)
}

// Wallet shows the buyer's wallet balance and how to charge it.
func (h *Handlers) Wallet(ctx context.Context, chatID int64) {
	wallet, err := h.panel.Wallet(ctx, chatID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "خطا در دریافت موجودی، کمی بعد دوباره تلاش کنید.", "")
		return
	}
	text := fmt.Sprintf("💼 کیف پول\n\nموجودی: %d تومان\n\nبرای شارژ کیف پول با پشتیبانی در ارتباط باشید.", wallet.Balance)
	buttons := [][]map[string]string{
		telegram.KeyboardRow(telegram.Button("🛒 خرید سرویس", "plans")),
		telegram.KeyboardRow(telegram.Button("📞 پشتیبانی", "support")),
	}
	_ = h.bot.SendMessage(ctx, chatID, text, telegram.MarshalKeyboard(buttons))
}

// Support shows the Gamaj support channels configured for this installation.
func (h *Handlers) Support(ctx context.Context, chatID int64) {
	text := "📞 پشتیبانی گمج\n\n" +
		"کانال گمج: t.me/TheGamaj\n" +
		"گروه پشتیبانی: t.me/GamajGP\n\n" +
		"برای شارژ کیف پول و مشکلات پرداخت به گروه پشتیبانی پیام دهید."
	buttons := [][]map[string]string{
		{telegram.ButtonURL("📢 کانال گمج", "https://t.me/TheGamaj")},
		{telegram.ButtonURL("👥 گروه پشتیبانی", "https://t.me/GamajGP")},
	}
	_ = h.bot.SendMessage(ctx, chatID, text, telegram.MarshalKeyboard(buttons))
}

// MyServices shows the buyer's provisioned services and credentials.
func (h *Handlers) MyServices(ctx context.Context, chatID int64) {
	service, err := h.panel.Service(ctx, chatID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, "سرویسی برای شما یافت نشد. برای خرید از «خرید سرویس» استفاده کنید.", telegram.MarshalKeyboard([][]map[string]string{telegram.KeyboardRow(telegram.Button("🛒 خرید سرویس", "plans"))}))
		return
	}
	text := fmt.Sprintf("👤 سرویس شما\n\nنام کاربری: %s\nوضعیت: %s\nحجم مصرفی: %s / %s\n\n🔗 لینک اشتراک:\n%s",
		service.Username, service.Status,
		panel.FormatBytes(service.UsedTraffic), panel.FormatBytes(service.DataLimit),
		service.SubscriptionURL)
	buttons := [][]map[string]string{
		telegram.KeyboardRow(telegram.Button("🔄 به‌روزرسانی", "myservices")),
		telegram.KeyboardRow(telegram.Button("🛒 خرید سرویس", "plans")),
	}
	_ = h.bot.SendMessage(ctx, chatID, text, telegram.MarshalKeyboard(buttons))
}

// deliver sends provisioned credentials to the buyer.
func (h *Handlers) deliver(ctx context.Context, chatID int64, orderID string) {
	service, err := h.panel.Service(ctx, chatID)
	if err != nil {
		_ = h.bot.SendMessage(ctx, chatID, fmt.Sprintf("پرداخت سفارش %s تأیید شد اما دریافت مشخصات سرویس ناموفق بود. کمی بعد دوباره «سرویس‌های من» را بزنید.", orderID), "")
		return
	}
	text := fmt.Sprintf("✅ پرداخت تأیید و سرویس شما فعال شد!\n\nنام کاربری: %s\nلینک اشتراک:\n%s", service.Username, service.SubscriptionURL)
	if len(service.Links) > 0 {
		text += "\n\n🔗 کانفیگ‌ها:"
		for _, link := range service.Links {
			text += "\n" + link
		}
	}
	text += "\n\n📞 پشتیبانی: t.me/GamajGP"
	_ = h.bot.SendMessage(ctx, chatID, text, "")
}
