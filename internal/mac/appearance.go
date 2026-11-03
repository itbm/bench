package mac

import "github.com/itbm/bench/internal/config"

// The appearance setting of Bench's windows, as the native side numbers
// it. The values match bridge_darwin.h's BENCH_APPEARANCE_ constants.
const (
	appearanceSystem = 0
	appearanceLight  = 1
	appearanceDark   = 2
)

// appearanceCode returns the native number of the appearance setting. An
// appearance that isn't one, which validation doesn't let through, is
// dark, as Bench always was.
func appearanceCode(appearance string) int {
	switch appearance {
	case config.AppearanceSystem:
		return appearanceSystem
	case config.AppearanceLight:
		return appearanceLight
	}
	return appearanceDark
}
