package mac

import (
	"encoding/hex"
	"strings"
	"testing"
)

// urlNamespace is RFC 9562's namespace for URLs,
// 6ba7b811-9dad-11d1-80b4-00c04fd430c8.
var urlNamespace = [16]byte{
	0x6b, 0xa7, 0xb8, 0x11, 0x9d, 0xad, 0x11, 0xd1,
	0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
}

func TestProfileNamespace(t *testing.T) {
	got := uuid5(urlNamespace, "https://github.com/itbm/bench/web-profile")
	want := hex.EncodeToString(profileNamespace[:])
	if strings.ReplaceAll(got, "-", "") != want {
		t.Errorf("profileNamespace is %s, but the URL namespace gives %s", want, got)
	}
}

func TestProfileUUID(t *testing.T) {
	// Expected values from Python: uuid.uuid5(namespace, name).
	tests := []struct{ profile, want string }{
		{"", "668ca316-1bfc-54c3-9f04-04e238ffd8b2"},
		{"default", "668ca316-1bfc-54c3-9f04-04e238ffd8b2"},
		{"slack", "49bd67ca-f5bf-5cdb-bea2-e8f2a0cfa8ea"},
		{"outlook", "f1b674bc-01ab-5565-a4b6-f03a4d617ca7"},
		{"Work", "4614d227-3cb9-5761-9be5-00a358497716"},
		{"work", "c8b2079c-86dc-5552-9d22-170de07368da"},
		{"café", "5a12a81d-1c9e-5304-9217-c5bc90289a5e"},
		{"a b", "13c45e40-e694-5dac-91f9-25e52508283e"},
	}
	for _, tt := range tests {
		if got := ProfileUUID(tt.profile); got != tt.want {
			t.Errorf("ProfileUUID(%q) = %s, want %s", tt.profile, got, tt.want)
		}
	}
}

func TestProfileUUIDShape(t *testing.T) {
	for _, profile := range []string{"", "slack", strings.Repeat("x", 500)} {
		u := ProfileUUID(profile)
		if len(u) != 36 || u[8] != '-' || u[13] != '-' || u[18] != '-' || u[23] != '-' {
			t.Errorf("ProfileUUID(%q) = %q, which isn't in canonical form", profile, u)
		}
		if u[14] != '5' {
			t.Errorf("ProfileUUID(%q) = %q, want version 5", profile, u)
		}
		if !strings.ContainsRune("89ab", rune(u[19])) {
			t.Errorf("ProfileUUID(%q) = %q, want the RFC 9562 variant", profile, u)
		}
		if ProfileUUID(profile) != u {
			t.Errorf("ProfileUUID(%q) isn't stable", profile)
		}
	}
}

func TestUserAgentSuffix(t *testing.T) {
	tests := []struct {
		macOS int
		want  string
	}{
		{14, "Version/17.0 Safari/605.1.15"},
		{15, "Version/18.0 Safari/605.1.15"},
		{26, "Version/26.0 Safari/605.1.15"},
		{27, "Version/27.0 Safari/605.1.15"},
		{11, "Version/14.0 Safari/605.1.15"},
		{0, "Version/17.0 Safari/605.1.15"},
	}
	for _, tt := range tests {
		if got := userAgentSuffix(tt.macOS); got != tt.want {
			t.Errorf("userAgentSuffix(%d) = %q, want %q", tt.macOS, got, tt.want)
		}
	}
}
