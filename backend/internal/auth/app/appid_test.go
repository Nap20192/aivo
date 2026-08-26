package app

import (
	"testing"
	"time"
)

func TestValidAppID(t *testing.T) {
	cases := []struct {
		id   AppID
		want bool
	}{
		{AppAdmin, true},
		{AppPOS, true},
		{AppWaiter, true},
		{AppMenu, true},
		{"", false},
		{"unknown", false},
		{"Admin", false}, // case-sensitive, no fuzzy matching
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			if got := ValidAppID(tc.id); got != tc.want {
				t.Errorf("ValidAppID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestDefaultExpiry(t *testing.T) {
	cases := []struct {
		id     AppID
		want   time.Duration
		wantOK bool
	}{
		{AppAdmin, 8 * time.Hour, true},
		{AppPOS, 12 * time.Hour, true},
		{AppWaiter, 12 * time.Hour, true},
		{AppMenu, time.Hour, true},
		{"unknown", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			got, ok := DefaultExpiry(tc.id)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("DefaultExpiry(%q) = (%v, %v), want (%v, %v)", tc.id, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
