package service

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// NotificationService wraps permission checks and safe sending for native desktop
// notifications, so callers never bare-call the Wails notifications service and miss the
// permission check (on macOS an unauthorized send fails silently and is undebuggable).
type NotificationService struct {
	svc *notifications.NotificationService
}

// NewNotificationService constructs the notification service. Passing nil for svc falls back
// internally to the notifications.NotificationService_ singleton, letting main.go directly
// reuse the already-registered service instance.
func NewNotificationService(svc *notifications.NotificationService) *NotificationService {
	if svc == nil {
		svc = notifications.NotificationService_
	}
	return &NotificationService{svc: svc}
}

// ensureAuthorized checks notification permission and actively requests it when missing.
// Returns whether authorization was ultimately granted; any error is only logged and never
// affects the caller's main flow.
func (n *NotificationService) ensureAuthorized() bool {
	authorized, err := n.svc.CheckNotificationAuthorization()
	if err != nil {
		slog.Warn(i18n.T("log.notification_auth_request_failed"), "error", err)
	}
	if authorized {
		return true
	}
	if authorized, err = n.svc.RequestNotificationAuthorization(); err != nil {
		slog.Warn(i18n.T("log.notification_auth_request_failed"), "error", err)
	}
	return authorized
}

// Notify safely sends a notification: ensures authorization first; skips and logs when
// unauthorized or denied. Send failures are also only logged, never propagated upward
// (background flows like update checks must not be blocked by notification problems).
func (n *NotificationService) Notify(opts notifications.NotificationOptions) {
	if !n.ensureAuthorized() {
		slog.Warn(i18n.T("log.notification_denied"))
		return
	}
	if err := n.svc.SendNotification(opts); err != nil {
		slog.Warn(i18n.T("log.send_update_notice_failed"), "error", err)
	}
}
