package mac

import "testing"

func TestAppearanceCode(t *testing.T) {
	for _, tt := range []struct {
		appearance string
		want       int
	}{
		{"system", appearanceSystem},
		{"light", appearanceLight},
		{"dark", appearanceDark},
		{"", appearanceDark},
		{"sepia", appearanceDark},
	} {
		if got := appearanceCode(tt.appearance); got != tt.want {
			t.Errorf("appearanceCode(%q) = %d, want %d", tt.appearance, got, tt.want)
		}
	}
}
