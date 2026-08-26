package app

import "time"

// AppID identifies one of AIVO's client surfaces. A minted token is
// always scoped to exactly one.
type AppID string

const (
	AppAdmin  AppID = "admin"  // web backoffice
	AppPOS    AppID = "pos"    // POS terminal
	AppWaiter AppID = "waiter" // waiter app
	AppMenu   AppID = "menu"   // public web menu
)

// defaultExpiry is how long a token minted for each surface lives
// before its holder must re-authenticate to get a new one. admin is
// long-lived back-office work; pos/waiter are sized to roughly one
// shift; menu is anonymous diner-facing and kept short since nothing
// sensitive depends on it staying valid.
var defaultExpiry = map[AppID]time.Duration{
	AppAdmin:  8 * time.Hour,
	AppPOS:    12 * time.Hour,
	AppWaiter: 12 * time.Hour,
	AppMenu:   time.Hour,
}

// ValidAppID reports whether id is one of the four known client surfaces.
func ValidAppID(id AppID) bool {
	_, ok := defaultExpiry[id]
	return ok
}

// DefaultExpiry returns id's token lifetime and whether id is known.
func DefaultExpiry(id AppID) (time.Duration, bool) {
	d, ok := defaultExpiry[id]
	return d, ok
}
