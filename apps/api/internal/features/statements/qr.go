package statements

import (
	"fmt"

	"github.com/skip2/go-qrcode"
)

// BankConfig is the teacher's transfer target a VietQR payload is built
// from. Kept as its own type here (rather than importing config.BankConfig)
// so this package's QR contract does not depend on how the value happens to
// be sourced — application configuration today, possibly a per-teacher
// database row later.
type BankConfig struct {
	BankCode      string
	AccountNumber string
	AccountName   string
}

// QRBuilder renders a VietQR-compatible bank transfer code. Kept as an
// interface — rather than calling qrcode.Encode directly from render.go — so
// render_test.go can substitute a fake and exercise the QR-absent branch
// without a real EMVCo payload or PNG encode.
type QRBuilder interface {
	// Payload returns the EMVCo/VietQR TLV string for a transfer of amount
	// (VND, whole units) carrying note. ok is false when cfg is missing any
	// required field — the caller must never fabricate a placeholder in that
	// case, only omit the QR block entirely.
	Payload(cfg BankConfig, amount int64, note string) (payload string, ok bool)
	// Render turns a payload built by Payload into a PNG QR code image.
	Render(payload string) ([]byte, error)
}

// napasAID is NAPAS's EMVCo application identifier for VietQR interbank
// transfers — every VietQR payload's merchant account info group (tag 38)
// starts with this constant.
const napasAID = "A000000727"

// vietQRServiceCode selects a same-bank-or-interbank account transfer, as
// opposed to a card or e-wallet target.
const vietQRServiceCode = "QRIBFTTA"

type emvQRBuilder struct{}

// NewQRBuilder returns the production QRBuilder: an EMVCo/VietQR payload
// encoder backed by github.com/skip2/go-qrcode for PNG rendering.
func NewQRBuilder() QRBuilder { return emvQRBuilder{} }

func (emvQRBuilder) Payload(cfg BankConfig, amount int64, note string) (string, bool) {
	if cfg.BankCode == "" || cfg.AccountNumber == "" || cfg.AccountName == "" {
		return "", false
	}

	// Field 38's sub-tag 01 is meant to carry NAPAS's numeric bank BIN, not a
	// bank's own short code. V1 has no BIN lookup table, so cfg.BankCode is
	// used directly as the acquirer id here — a simplification recorded in
	// adr.md. It changes what a scanning wallet resolves as the bank, never
	// this payload's TLV structure or checksum, which is what this package's
	// tests assert.
	beneficiary := tlv("00", cfg.BankCode) + tlv("01", cfg.AccountNumber)
	merchantAccountInfo := tlv("00", napasAID) + tlv("01", beneficiary) + tlv("02", vietQRServiceCode)
	// The transfer note is caller-supplied and derived from a contact name
	// that can be up to 100 characters, but EMVCo field lengths are two ASCII
	// digits, so a value of 100+ characters would emit a three-digit length
	// and corrupt the payload. Clamp to NAPAS's 25-character limit for the
	// purpose-of-transaction field, on rune boundaries so a multibyte name is
	// never split mid-character.
	additionalData := tlv("08", clampRunes(note, 25))

	body := tlv("00", "01") + // payload format indicator
		tlv("01", "12") + // point of initiation method: dynamic
		tlv("38", merchantAccountInfo) + // merchant account info (VietQR/NAPAS)
		tlv("53", "704") + // transaction currency: VND
		tlv("54", fmt.Sprintf("%d", amount)) + // transaction amount
		tlv("58", "VN") + // country code
		tlv("62", additionalData) + // additional data (purpose/note)
		"6304" // CRC tag + length; the checksum itself is appended below

	crc := crc16CCITT([]byte(body))
	return fmt.Sprintf("%s%04X", body, crc), true
}

func (emvQRBuilder) Render(payload string) ([]byte, error) {
	return qrcode.Encode(payload, qrcode.Medium, 256)
}

// tlv encodes one EMVCo tag-length-value field. Length is two ASCII digits,
// so callers must keep every value under 100 bytes; the one field fed
// caller-supplied text (the transfer note) is clamped before it reaches here.
func tlv(id, value string) string {
	return fmt.Sprintf("%s%02d%s", id, len(value), value)
}

// clampRunes returns s truncated to at most limit runes, never splitting a
// multibyte character. Byte length can still exceed limit for multibyte text,
// so callers that need a byte ceiling must keep limit small enough for it.
func clampRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

// crc16CCITT implements EMVCo's checksum for QR payloads: CRC-16/CCITT-FALSE
// — polynomial 0x1021, initial value 0xFFFF, no input/output reflection, no
// final XOR — computed over the whole payload up to and including the "6304"
// CRC tag+length, excluding the four checksum digits themselves.
func crc16CCITT(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
