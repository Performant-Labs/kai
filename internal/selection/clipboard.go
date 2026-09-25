package selection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// readClipboardText reads the system clipboard text cross-platform. A nil app returns an
// empty string directly.
func readClipboardText(app *application.App) string {
	if app == nil {
		return ""
	}
	text, _ := app.Clipboard.Text()
	return text
}
