package model

// App window name constants, centralized to prevent raw-string typos.
// Kept separate from internal/events (event names) to avoid muddled responsibilities.
const (
	// WindowTranslate is the input translate window (mainWindow, /translate.html)
	WindowTranslate = "translate"
	// WindowSettings is the settings window (/settings.html)
	WindowSettings = "settings"
	// WindowScreenshot is the screenshot translate window (/screenshot.html)
	WindowScreenshot = "screenshot"
)
