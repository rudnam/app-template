package account

// Confirming email addresses: after sign-up we email a link; opening it
// marks the address confirmed. The app keeps working meanwhile and shows a
// reminder. Signing in with Google confirms the address too.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/francis/app-template/internal/db"
	"github.com/francis/app-template/internal/httpx"
	"github.com/francis/app-template/internal/mailer"
)

const confirmationTTL = 48 * time.Hour

// sendConfirmation emails the link, in lang ("en" or "ja": the language the
// app was in when the person signed up or asked again).
func (h *Handler) sendConfirmation(ctx context.Context, userID int64, email, lang string) error {
	token, hash, err := newSessionToken() // same shape: random token, stored hashed
	if err != nil {
		return err
	}
	if err := h.store.createEmailToken(ctx, userID, email, hash, h.now().Add(confirmationTTL)); err != nil {
		return err
	}
	link := h.cfg.PublicOrigin + "/api/verify-email?token=" + token
	hours := int(confirmationTTL.Hours())
	if lang == "ja" {
		return h.cfg.Mail.Send(ctx, mailer.Message{
			To:      email,
			Subject: h.cfg.AppName + "のメールアドレスの確認",
			Body: fmt.Sprintf("こんにちは。\n\n次のリンクを開いて、このメールアドレスがあなたのものであることを確認してください。\n\n%s\n\n"+
				"リンクの有効期限は%d時間です。%sのアカウントを作成した覚えがない場合は、このメールは無視してください。\n",
				link, hours, h.cfg.AppName),
		})
	}
	return h.cfg.Mail.Send(ctx, mailer.Message{
		To:      email,
		Subject: "Confirm your email for " + h.cfg.AppName,
		Body: fmt.Sprintf("Hello,\n\nPlease confirm that this is your email address by opening this link:\n\n%s\n\n"+
			"The link works for %d hours. If you didn't create %s account, you can ignore this email.\n",
			link, hours, article(h.cfg.AppName)),
	})
}

// resendConfirmation emails a fresh link to the signed-in person.
func (h *Handler) resendConfirmation(w http.ResponseWriter, r *http.Request) error {
	u := UserFrom(r)
	if h.cfg.Mail == nil {
		return httpx.Errorf(http.StatusNotFound, "this server doesn't send email")
	}
	if u.EmailVerified {
		return httpx.Errorf(http.StatusConflict, "your email is already confirmed")
	}
	if err := h.sendConfirmation(r.Context(), u.ID, u.Email, httpx.Language(r)); err != nil {
		slog.ErrorContext(r.Context(), "send confirmation email", "user", u.ID, "err", err)
		return httpx.Errorf(http.StatusBadGateway, "the email could not be sent; try again in a few minutes")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// verifyEmail is the link in the email. It sends the browser back to the
// app with ?email=confirmed, or ?email=expired if the link no longer works.
func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	result := "confirmed"
	err := h.store.verifyEmail(r.Context(), hashToken(r.URL.Query().Get("token")))
	if errors.Is(err, db.ErrNotFound) {
		result = "expired"
	} else if err != nil {
		slog.ErrorContext(r.Context(), "verify email", "err", err)
		result = "failed"
	}
	http.Redirect(w, r, "/app/?email="+result, http.StatusFound)
}

// article puts "a" or "an" before name, for "a RudWallet account".
func article(name string) string {
	if name != "" && strings.ContainsRune("AEIOUaeiou", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}
