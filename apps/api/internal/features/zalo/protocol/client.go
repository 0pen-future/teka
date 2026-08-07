package protocol

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Session holds authenticated Zalo session state.
type Session struct {
	UID       string
	IMEI      string
	UserAgent string
	Language  string
	SecretKey string // base64-encoded zpw_enk

	// DisplayName is the Zalo profile name of the logged-in account. Only the
	// QR login path learns it; a cookie re-login leaves it empty.
	DisplayName string

	LoginInfo *LoginInfo
	Settings  *Settings
	CookieJar http.CookieJar
	Client    *http.Client
}

// NewSession creates a fresh unauthenticated session.
func NewSession() *Session {
	jar, _ := cookiejar.New(nil)
	return &Session{
		UserAgent: DefaultUserAgent,
		Language:  DefaultLanguage,
		CookieJar: jar,
		Client: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= MaxRedirects {
					return fmt.Errorf("zalo_personal: too many redirects")
				}
				return nil
			},
		},
	}
}

// GenerateIMEI creates a Zalo-style IMEI from user agent.
// Format: uuid-md5(userAgent) (matching zcago generateZaloUUID).
func GenerateIMEI(userAgent string) string {
	u := uuid.New().String()
	hash := md5.Sum([]byte(userAgent))
	return u + "-" + hex.EncodeToString(hash[:])
}

// --- HTTP helpers (ported from zcago/internal/httpx) ---

// makeURL builds a Zalo API URL with query params and optional defaults.
func makeURL(_ *Session, baseURL string, params map[string]any, includeDefaults bool) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	query := u.Query()
	for key, value := range params {
		if !query.Has(key) {
			query.Set(key, convertToString(value))
		}
	}
	if includeDefaults {
		if !query.Has("zpw_ver") {
			query.Set("zpw_ver", convertToString(DefaultAPIVersion))
		}
		if !query.Has("zpw_type") {
			query.Set("zpw_type", convertToString(DefaultAPIType))
		}
	}
	u.RawQuery = query.Encode()
	return u.String()
}

// newRequest builds a request and keeps the URL out of a construction failure,
// which quotes the whole thing verbatim — query string included. Only a
// malformed URL gets here, so the caller learns why it was rejected and nothing
// else; the same reasoning as doRequest applies to what the URL holds.
func newRequest(ctx context.Context, method, u string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, fmt.Errorf("%s request: %w", method, urlErr.Err)
		}
		return nil, err
	}
	return req, nil
}

// doRequest sends req and keeps the query string out of any transport failure.
//
// Go embeds the full request URL in the *url.Error it returns, and these URLs
// carry the account's IMEI in the clear plus the ZCID, which is that same IMEI
// encrypted under a key shipped in this package. Transport failures are routine
// — an outage makes every linked account fail at once — and they are logged, so
// the error text must name only where the request was going.
func doRequest(sess *Session, req *http.Request) (*http.Response, error) {
	resp, err := sess.Client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return nil, fmt.Errorf("%s %s://%s%s: %w",
				urlErr.Op, req.URL.Scheme, req.URL.Host, req.URL.Path, urlErr.Err)
		}
		return nil, err
	}
	return resp, nil
}

// buildFormBody creates url-encoded form body from params.
func buildFormBody(data map[string]string) *strings.Reader {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	return strings.NewReader(form.Encode())
}

// generateSignKey computes md5 hash of sorted params for Zalo API signing.
func generateSignKey(typeStr string, params map[string]any) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var signStr strings.Builder
	signStr.WriteString("zsecure" + typeStr)
	for _, k := range keys {
		if v := params[k]; v != nil {
			signStr.WriteString(convertToString(v))
		}
	}
	hash := md5.Sum([]byte(signStr.String()))
	return hex.EncodeToString(hash[:])
}

// --- Encrypt param generation (ported from zcago/internal/httpx/encryption.go) ---

// getEncryptParam generates encrypted request parameters for Zalo API.
func getEncryptParam(sess *Session, typeStr string) (params map[string]any, enk *string, err error) {
	data := map[string]any{
		"computer_name": DefaultComputerName,
		"imei":          sess.IMEI,
		"language":      sess.Language,
		"ts":            time.Now().UnixMilli(),
	}

	zcid, zcidExt, encKey, err := encryptParams(sess.IMEI, data)
	if err != nil {
		return nil, nil, err
	}

	params = map[string]any{
		"zcid":           zcid,
		"enc_ver":        DefaultEncryptVer,
		"zcid_ext":       zcidExt,
		"params":         encKey.encData,
		"type":           DefaultAPIType,
		"client_version": DefaultAPIVersion,
	}

	if typeStr == "getserverinfo" {
		params["signkey"] = generateSignKey(typeStr, map[string]any{
			"imei":           sess.IMEI,
			"type":           DefaultAPIType,
			"client_version": DefaultAPIVersion,
			"computer_name":  DefaultComputerName,
		})
	} else {
		params["signkey"] = generateSignKey(typeStr, params)
	}

	return params, &encKey.key, nil
}

type encryptResult struct {
	key     string // encrypt key for response decryption
	encData string // encrypted request data (base64)
}

// encryptParams performs the ZCID + key derivation + AES encryption.
func encryptParams(imei string, data map[string]any) (zcid, zcidExt string, result *encryptResult, err error) {
	ts := time.Now().UnixMilli()
	zcidExt = randomHexString(6, 12)

	// Create ZCID
	zcidData := fmt.Sprintf("%d,%s,%d", DefaultAPIType, imei, ts)
	zcidRaw, err := EncodeAESCBC([]byte(DefaultZCIDKey), zcidData, true)
	if err != nil {
		return "", "", nil, fmt.Errorf("zalo_personal: create zcid: %w", err)
	}
	zcid = strings.ToUpper(zcidRaw)

	// Derive encrypt key
	encKey, err := deriveEncryptKey(zcidExt, zcid)
	if err != nil {
		return "", "", nil, fmt.Errorf("zalo_personal: derive key: %w", err)
	}

	// Encrypt data
	blob, err := json.Marshal(data)
	if err != nil {
		return "", "", nil, err
	}
	encData, err := EncodeAESCBC([]byte(encKey), string(blob), false)
	if err != nil {
		return "", "", nil, fmt.Errorf("zalo_personal: encrypt data: %w", err)
	}

	return zcid, zcidExt, &encryptResult{key: encKey, encData: encData}, nil
}

// deriveEncryptKey derives the AES key from zcid_ext and ZCID.
func deriveEncryptKey(ext, id string) (string, error) {
	sum := md5.Sum([]byte(ext))
	nUpper := strings.ToUpper(hex.EncodeToString(sum[:]))

	evenE, _ := processStr(nUpper)
	evenI, oddI := processStr(id)
	if len(evenE) == 0 || len(evenI) == 0 || len(oddI) == 0 {
		return "", fmt.Errorf("zalo_personal: invalid key derivation params")
	}

	var b strings.Builder
	b.WriteString(joinFirst(evenE, 8))
	b.WriteString(joinFirst(evenI, 12))
	b.WriteString(joinFirst(reverseCopy(oddI), 12))
	return b.String(), nil
}

// processStr splits string into even-index and odd-index character slices.
func processStr(s string) (even, odd []string) {
	for i, r := range s {
		if i%2 == 0 {
			even = append(even, string(r))
		} else {
			odd = append(odd, string(r))
		}
	}
	return
}

// joinFirst joins the first n elements of a string slice.
func joinFirst(parts []string, n int) string {
	if n > len(parts) {
		n = len(parts)
	}
	return strings.Join(parts[:n], "")
}

// reverseCopy returns a reversed copy of the slice.
func reverseCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// randomHexString generates a random hex string of length between min and max.
func randomHexString(minLen, maxLen int) string {
	length := minLen
	if maxLen > minLen {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(maxLen-minLen+1)))
		length = minLen + int(n.Int64())
	}
	byteLen := (length + 1) / 2
	buf := make([]byte, byteLen)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)[:length]
}

// convertToString converts any value to its string representation.
func convertToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case int:
		return strconv.Itoa(val)
	case int8, int16, int32, int64:
		return strconv.FormatInt(reflect.ValueOf(val).Int(), 10)
	case uint, uint8, uint16, uint32, uint64:
		return strconv.FormatUint(reflect.ValueOf(val).Uint(), 10)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		return fmt.Sprint(val)
	}
}

// encryptPayload encrypts a JSON payload with the session's secret key via
// AES-CBC. This is the request-body encryption every post-login API call uses;
// the login flow itself derives its key differently (see encryptParams).
func encryptPayload(sess *Session, payload map[string]any) (string, error) {
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	key, err := base64.StdEncoding.DecodeString(sess.SecretKey)
	if err != nil {
		return "", fmt.Errorf("zalo_personal: decode secret key: %w", err)
	}
	return EncodeAESCBC(key, string(blob), false)
}

// decryptDataField decrypts an encrypted base64 data string from a Zalo API
// response. The decrypted payload is itself a Response envelope
// {"error_code":0, "data":...}, so this unwraps the inner envelope and returns
// the raw data field.
func decryptDataField(sess *Session, data string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(sess.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("zalo_personal: decode secret key: %w", err)
	}
	unescaped, err := url.PathUnescape(data)
	if err != nil {
		return nil, err
	}
	plain, err := DecodeAESCBC(key, unescaped)
	if err != nil {
		return nil, err
	}

	var inner Response[json.RawMessage]
	if err := json.Unmarshal(plain, &inner); err != nil {
		return nil, fmt.Errorf("zalo_personal: unwrap inner response: %w", err)
	}
	if inner.ErrorCode != 0 {
		return nil, fmt.Errorf("zalo_personal: inner error code %d: %s", inner.ErrorCode, inner.ErrorMessage)
	}
	return inner.Data, nil
}

// defaultHeaders returns standard Zalo API request headers.
func defaultHeaders(sess *Session) http.Header {
	h := make(http.Header, 8)
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Accept-Encoding", "gzip")
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	h.Set("Origin", DefaultBaseURL.String())
	h.Set("Referer", DefaultBaseURL.String()+"/")
	h.Set("User-Agent", sess.UserAgent)
	return h
}

// ServiceURL returns the first base URL Zalo advertised for a service, or "" if
// the session has no service map yet. Only a cookie login populates the map;
// the QR path never fetches it.
func ServiceURL(sess *Session, service string) string {
	if sess.LoginInfo == nil {
		return ""
	}
	var urls []string
	switch service {
	case "chat":
		urls = sess.LoginInfo.ZpwServiceMapV3.Chat
	case "group":
		urls = sess.LoginInfo.ZpwServiceMapV3.Group
	case "file":
		urls = sess.LoginInfo.ZpwServiceMapV3.File
	case "profile":
		urls = sess.LoginInfo.ZpwServiceMapV3.Profile
	case "group_poll":
		urls = sess.LoginInfo.ZpwServiceMapV3.GroupPoll
	}
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}
