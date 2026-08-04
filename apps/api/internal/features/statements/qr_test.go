package statements

import (
	"bytes"
	"fmt"
	"image/png"
	"strings"
	"testing"
)

func testBankConfig() BankConfig {
	// Obviously-fake fixture values: not a real bank code or account.
	return BankConfig{BankCode: "TESTBANK", AccountNumber: "0000000000", AccountName: "NGUYEN VAN A"}
}

func TestEMVQRBuilderPayloadAbsentConfigReturnsNotOK(t *testing.T) {
	b := NewQRBuilder()
	cases := []BankConfig{
		{},
		{BankCode: "TESTBANK"},
		{BankCode: "TESTBANK", AccountNumber: "0000000000"},
		{AccountNumber: "0000000000", AccountName: "NGUYEN VAN A"},
	}
	for _, cfg := range cases {
		if _, ok := b.Payload(cfg, 150000, "HP Test 08/2026"); ok {
			t.Errorf("Payload(%+v) ok = true, want false for an incomplete config", cfg)
		}
	}
}

func TestEMVQRBuilderPayloadCRCIsCorrect(t *testing.T) {
	b := NewQRBuilder()
	payload, ok := b.Payload(testBankConfig(), 150000, "HP Nguyen Van A 08/2026")
	if !ok {
		t.Fatal("Payload ok = false, want true for a complete config")
	}
	if len(payload) < 8 {
		t.Fatalf("payload too short: %q", payload)
	}
	body, gotCRC := payload[:len(payload)-4], payload[len(payload)-4:]
	if !strings.HasSuffix(body, "6304") {
		t.Fatalf("payload body does not end in the 6304 CRC tag: %q", body)
	}
	wantCRC := fmt.Sprintf("%04X", crc16CCITT([]byte(body)))
	if gotCRC != wantCRC {
		t.Fatalf("CRC = %q, recomputing over the body gives %q", gotCRC, wantCRC)
	}
}

func TestEMVQRBuilderPayloadContainsNoRawReason(t *testing.T) {
	// The note is the only free text a QR payload carries; it must be exactly
	// what the caller passed, never anything else appended silently.
	b := NewQRBuilder()
	payload, ok := b.Payload(testBankConfig(), 50000, "HP Tran Thi B 09/2026")
	if !ok {
		t.Fatal("Payload ok = false")
	}
	if !strings.Contains(payload, "HP Tran Thi B 09/2026") {
		t.Fatalf("payload does not contain the expected note: %q", payload)
	}
}

func TestEMVQRBuilderPayloadClampsLongNoteToValidLength(t *testing.T) {
	// A note derived from a maximum-length (100-char) contact name would push
	// an EMVCo field past its two-digit length and corrupt the payload. The
	// builder must clamp it: the payload still verifies against its own CRC
	// (structure intact) and the note is truncated on a rune boundary, not
	// mid-character.
	b := NewQRBuilder()
	longNote := "HP " + strings.Repeat("Nguyễn", 20) + " 08/2026" // >100 bytes, multibyte
	payload, ok := b.Payload(testBankConfig(), 150000, longNote)
	if !ok {
		t.Fatal("Payload ok = false, want true")
	}
	body, gotCRC := payload[:len(payload)-4], payload[len(payload)-4:]
	if want := fmt.Sprintf("%04X", crc16CCITT([]byte(body))); gotCRC != want {
		t.Fatalf("CRC = %q, want %q — payload structure is corrupt", gotCRC, want)
	}
	clamped := clampRunes(longNote, 25)
	if !strings.Contains(payload, clamped) {
		t.Fatalf("payload does not contain the clamped note %q", clamped)
	}
	if strings.Contains(payload, longNote) {
		t.Fatal("payload contains the full un-clamped note")
	}
}

func TestEMVQRBuilderRenderReturnsDecodablePNG(t *testing.T) {
	b := NewQRBuilder()
	payload, ok := b.Payload(testBankConfig(), 150000, "HP Test 08/2026")
	if !ok {
		t.Fatal("Payload ok = false")
	}
	imgBytes, err := b.Render(payload)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		t.Fatalf("decode rendered PNG: %v", err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("decoded image has zero size")
	}
}
