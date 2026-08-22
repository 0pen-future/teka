package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SendMessage sends a text DM to a Zalo user and returns the message id Zalo
// assigned. Ported from goclaw's SendMessage, DM path only — the group branch,
// typing events, and media uploads are deliberately not part of this port.
func SendMessage(ctx context.Context, sess *Session, toUID, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("zalo_personal: message text cannot be empty")
	}

	baseURL := ServiceURL(sess, "chat")
	if baseURL == "" {
		return "", fmt.Errorf("zalo_personal: no service URL for chat")
	}

	payload := map[string]any{
		"message":  text,
		"clientId": time.Now().UnixMilli(),
		"ttl":      0,
		"toid":     toUID,
		"imei":     sess.IMEI,
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: encrypt send payload: %w", err)
	}

	sendURL := makeURL(sess, baseURL+"/api/message/sms", map[string]any{"nretry": 0}, true)

	form := buildFormBody(map[string]string{"params": encData})
	req, err := newRequest(ctx, http.MethodPost, sendURL, form)
	if err != nil {
		return "", err
	}
	setDefaultHeaders(req, sess)

	resp, err := doRequest(sess, req)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: send message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Send response is encrypted: {"error_code":0, "data":"<encrypted>"}
	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return "", fmt.Errorf("zalo_personal: parse send response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return "", &APIError{Op: "send", Code: envelope.ErrorCode}
	}
	if envelope.Data == nil {
		return "", nil
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: decrypt send response: %w", err)
	}

	var result struct {
		MsgID json.Number `json:"msgId"`
	}
	if err := json.Unmarshal(plain, &result); err != nil {
		return "", fmt.Errorf("zalo_personal: parse send result: %w", err)
	}

	return result.MsgID.String(), nil
}
