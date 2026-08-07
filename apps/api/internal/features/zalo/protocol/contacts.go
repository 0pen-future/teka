package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// FetchFriends fetches the authenticated user's friend list from Zalo. Ported
// from goclaw's FetchFriends; the group listing that sat beside it is not part
// of this port.
func FetchFriends(ctx context.Context, sess *Session) ([]FriendInfo, error) {
	baseURL := ServiceURL(sess, "profile")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no profile service URL")
	}

	payload := map[string]any{
		"page":        1,
		"count":       20000,
		"incInvalid":  1,
		"avatar_size": 120,
		"actiontime":  0,
		"imei":        sess.IMEI,
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt friends payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/social/friend/getfriends",
		map[string]any{"params": encData}, true)

	req, err := newRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := doRequest(sess, req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: fetch friends: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Response envelope: {"error_code":0, "data":"<encrypted_base64>"}
	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse friends response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, &APIError{Op: "friends", Code: envelope.ErrorCode}
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("zalo_personal: empty friends data")
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt friends: %w", err)
	}

	var friends []FriendInfo
	if err := json.Unmarshal(plain, &friends); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse friends list: %w", err)
	}
	return friends, nil
}
