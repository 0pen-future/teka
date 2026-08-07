// Package protocol speaks Zalo's personal-account web protocol.
//
// It is a reverse-engineered, unofficial client. Zalo publishes no contract for
// any endpoint used here, ships no compatibility guarantee, and can change the
// wire format without notice — when that happens this package breaks and has to
// be re-ported. Keep it small for exactly that reason.
//
// Scope is authentication only: QR link (LoginQR) and cookie re-login
// (LoginWithCredentials), plus the Session and Credentials values that carry
// state between them. Messaging, contacts, groups, media, and the WebSocket
// listener are deliberately absent.
//
// The package is quarantined: it imports nothing from the rest of Teka, so it
// can be deleted or swapped wholesale. Credentials produced here are full
// account-takeover material — callers must encrypt them at rest and keep them
// out of logs and API responses.
//
// Ported from zcago (MIT): https://github.com/amrakk/zcago
package protocol
