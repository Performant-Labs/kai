package settings

import (
	"math"

	"github.com/spf13/viper"
)

// DefaultFontSize is the default text size (issue #195): 120% of the size the app had before the
// setting existed. It is what a fresh install, an old settings.json without the key and any value
// outside FontSizeSteps read as.
const DefaultFontSize = 120

// fontSizeSteps is the ladder: an even 20 percentage points per step, measured from the
// pre-#195 size (not compounding). The frontend keeps a copy in
// frontend/src/constants/fontSize.ts; a test pins the two together.
var fontSizeSteps = [...]int{80, 100, 120, 140, 160, 180}

// FontSizeSteps returns the six allowed text sizes in percent, smallest first. It returns a copy.
func FontSizeSteps() []int {
	out := make([]int, len(fontSizeSteps))
	copy(out, fontSizeSteps[:])
	return out
}

// NormalizeFontSize returns p when it is one of the six allowed sizes and DefaultFontSize
// otherwise. It is the one place that decides what a stored or submitted size means; nothing
// that reads or saves the setting fails on a bad value.
func NormalizeFontSize(p int) int {
	for _, s := range fontSizeSteps {
		if s == p {
			return p
		}
	}
	return DefaultFontSize
}

// fontSizeKey is the settings.json key of Settings.FontSize.
const fontSizeKey = "font_size"

// readFontSize reads Settings.FontSize from the loaded file: a whole number that is one of the
// allowed sizes is taken as it is (JSON numbers arrive as float64, so 135.0 is 135), and
// everything else, a missing key, null, a string, a boolean, an object, a fraction, a number
// out of range, is DefaultFontSize.
func readFontSize(v *viper.Viper) int {
	var f float64
	switch x := v.Get(fontSizeKey).(type) {
	case float64:
		f = x
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	default:
		return DefaultFontSize
	}
	if f != math.Trunc(f) || f < 0 || f > 1000 {
		return DefaultFontSize
	}
	return NormalizeFontSize(int(f))
}
