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

// FindUser resolves phone numbers to Zalo accounts through Zalo's reverse
// lookup. Ported from zcago's FindUser, which accepts a batch of phones per
// call. The result map is keyed by phone exactly as sent; phones Zalo cannot
// resolve — unknown numbers, or accounts whose privacy disallows phone
// discovery — are absent. A found account is not necessarily a friend.
//
// The batch travels in the GET query string (~20 bytes per phone), so callers
// own chunking: keep batches small enough (tens, not hundreds) that the URL
// stays under common proxy limits, and pace successive chunks.
func FindUser(ctx context.Context, sess *Session, phones []string) (map[string]FoundUser, error) {
	if len(phones) == 0 {
		return nil, fmt.Errorf("zalo_personal: phone list cannot be empty")
	}

	baseURL := ServiceURL(sess, "friend")
	if baseURL == "" {
		return nil, fmt.Errorf("zalo_personal: no friend service URL")
	}

	payload := map[string]any{
		"phones":      phones,
		"avatar_size": 240,
		"language":    sess.Language,
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: encrypt lookup payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/friend/profile/multiget",
		map[string]any{"params": encData}, true)

	req, err := newRequest(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	setDefaultHeaders(req, sess)

	resp, err := doRequest(sess, req)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: find user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse lookup response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return nil, &APIError{Op: "lookup", Code: envelope.ErrorCode}
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("zalo_personal: empty lookup data")
	}

	plain, err := decryptDataField(sess, *envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decrypt lookup: %w", err)
	}

	var found map[string]FoundUser
	if err := json.Unmarshal(plain, &found); err != nil {
		return nil, fmt.Errorf("zalo_personal: parse lookup result: %w", err)
	}
	return found, nil
}

// SendFriendRequest sends one friend request to one Zalo user. Ported from
// zcago's SendFriendRequest. Success is a zero error_code; the data field is
// opaque and deliberately not parsed. Callers own the one-per-explicit-action
// pacing — this function never batches.
func SendFriendRequest(ctx context.Context, sess *Session, userID, message string) error {
	if userID == "" {
		return fmt.Errorf("zalo_personal: friend request user id cannot be empty")
	}

	baseURL := ServiceURL(sess, "friend")
	if baseURL == "" {
		return fmt.Errorf("zalo_personal: no friend service URL")
	}

	// srcParams is a JSON string embedded in the payload, not a nested object —
	// that is the upstream wire shape.
	srcParams, err := json.Marshal(map[string]string{"uidTo": userID})
	if err != nil {
		return fmt.Errorf("zalo_personal: marshal srcParams: %w", err)
	}

	payload := map[string]any{
		"toid":      userID,
		"msg":       message,
		"reqsrc":    30,
		"imei":      sess.IMEI,
		"language":  sess.Language,
		"srcParams": string(srcParams),
	}

	encData, err := encryptPayload(sess, payload)
	if err != nil {
		return fmt.Errorf("zalo_personal: encrypt friend request payload: %w", err)
	}

	reqURL := makeURL(sess, baseURL+"/api/friend/sendreq", nil, true)

	form := buildFormBody(map[string]string{"params": encData})
	req, err := newRequest(ctx, http.MethodPost, reqURL, form)
	if err != nil {
		return err
	}
	setDefaultHeaders(req, sess)

	resp, err := doRequest(sess, req)
	if err != nil {
		return fmt.Errorf("zalo_personal: send friend request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var envelope Response[*string]
	if err := readJSON(resp, &envelope); err != nil {
		return fmt.Errorf("zalo_personal: parse friend request response: %w", err)
	}
	if envelope.ErrorCode != 0 {
		return &APIError{Op: "friend_request", Code: envelope.ErrorCode}
	}
	return nil
}
