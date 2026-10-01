// Package push sends Web Push notifications: payloads encrypted for the
// browser (RFC 8291, aes128gcm) and signed with the server's VAPID key
// (RFC 8292), using only the standard library.
package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

var b64 = base64.RawURLEncoding

// Subscription is what the browser's PushManager hands back.
type Subscription struct {
	Endpoint string
	P256dh   string // the browser's public key, base64url
	Auth     string // the browser's auth secret, base64url
}

// ErrGone means the subscription no longer exists; forget it.
var ErrGone = errors.New("push subscription is gone")

// Sender holds the server's VAPID identity.
type Sender struct {
	key     *ecdsa.PrivateKey
	contact string // mailto: or https: address push services can reach
	client  *http.Client
}

// LoadOrCreate reads the VAPID key at path, creating it (mode 0600) if it
// doesn't exist yet. Keep the file: browsers' subscriptions are tied to it.
func LoadOrCreate(path, contact string) (*Sender, error) {
	b, err := os.ReadFile(path)
	var key *ecdsa.PrivateKey
	switch {
	case err == nil:
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, fmt.Errorf("%s: not a PEM file", path)
		}
		if key, err = x509.ParseECPrivateKey(blk.Bytes); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	case errors.Is(err, os.ErrNotExist):
		if key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
			return nil, err
		}
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	return &Sender{key: key, contact: contact, client: &http.Client{Timeout: 15 * time.Second}}, nil
}

// WithClient makes the sender use c for requests (tests use it to reach a
// fake push service).
func (s *Sender) WithClient(c *http.Client) *Sender {
	s.client = c
	return s
}

// PublicKey is the VAPID public key the browser subscribes with (base64url).
func (s *Sender) PublicKey() string {
	pub, _ := s.key.PublicKey.ECDH()
	return b64.EncodeToString(pub.Bytes())
}

// Send encrypts payload for sub and posts it to the push service. ttl is
// how long the service should keep it for an offline browser.
func (s *Sender) Send(ctx context.Context, sub Subscription, payload []byte, ttl time.Duration) error {
	body, err := Encrypt(payload, sub.P256dh, sub.Auth)
	if err != nil {
		return err
	}
	u, err := url.Parse(sub.Endpoint)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("bad push endpoint %q", sub.Endpoint)
	}
	jwt, err := s.vapidJWT(u.Scheme+"://"+u.Host, time.Now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("TTL", fmt.Sprint(int(ttl.Seconds())))
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+s.PublicKey())
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return ErrGone
	case res.StatusCode >= 300:
		return fmt.Errorf("push service: %s: %s", res.Status, bytes.TrimSpace(msg))
	}
	return nil
}

// vapidJWT signs the claim that this server is allowed to push to aud.
func (s *Sender) vapidJWT(aud string, now time.Time) (string, error) {
	head := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]any{"aud": aud, "exp": now.Add(12 * time.Hour).Unix(), "sub": s.contact})
	unsigned := head + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(unsigned))
	r, sv, err := ecdsa.Sign(rand.Reader, s.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	sv.FillBytes(sig[32:])
	return unsigned + "." + b64.EncodeToString(sig), nil
}

// Encrypt makes an aes128gcm body for one browser (RFC 8291).
func Encrypt(payload []byte, p256dh, auth string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return encrypt(payload, p256dh, auth, salt, as)
}

func encrypt(payload []byte, p256dh, auth string, salt []byte, as *ecdh.PrivateKey) ([]byte, error) {
	uaRaw, err := b64.DecodeString(p256dh)
	if err != nil {
		return nil, fmt.Errorf("bad p256dh key: %w", err)
	}
	authSecret, err := b64.DecodeString(auth)
	if err != nil || len(authSecret) == 0 {
		return nil, fmt.Errorf("bad auth secret")
	}
	ua, err := ecdh.P256().NewPublicKey(uaRaw)
	if err != nil {
		return nil, fmt.Errorf("bad p256dh key: %w", err)
	}
	secret, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPub := as.PublicKey().Bytes()

	// IKM = HKDF(auth_secret, ecdh_secret, "WebPush: info" || 0 || ua || as, 32)
	keyInfo := append(append(append([]byte("WebPush: info\x00"), uaRaw...), asPub...), 1)
	ikm := mac(mac(authSecret, secret), keyInfo)
	prk := mac(salt, ikm)
	cek := mac(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16]
	nonce := mac(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12]

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain := append(append([]byte{}, payload...), 2) // last (and only) record
	if len(plain)+16 > 4096 {
		return nil, fmt.Errorf("push payload too big: %d bytes", len(payload))
	}
	head := make([]byte, 0, 86)
	head = append(head, salt...)
	head = binary.BigEndian.AppendUint32(head, 4096)
	head = append(head, byte(len(asPub)))
	head = append(head, asPub...)
	return gcm.Seal(head, nonce, plain, nil), nil
}

func mac(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// Decrypt reverses Encrypt with the browser's private key and auth secret.
// Browsers do this themselves; it's here for tests and debugging tools.
func Decrypt(body []byte, ua *ecdh.PrivateKey, authSecret []byte) ([]byte, error) {
	if len(body) < 21 {
		return nil, errors.New("push body too short")
	}
	salt, idlen := body[:16], int(body[20])
	if len(body) < 21+idlen {
		return nil, errors.New("push body too short")
	}
	asRaw, ct := body[21:21+idlen], body[21+idlen:]
	as, err := ecdh.P256().NewPublicKey(asRaw)
	if err != nil {
		return nil, err
	}
	secret, err := ua.ECDH(as)
	if err != nil {
		return nil, err
	}
	keyInfo := append(append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asRaw...), 1)
	prk := mac(salt, mac(mac(authSecret, secret), keyInfo))
	block, err := aes.NewCipher(mac(prk, []byte("Content-Encoding: aes128gcm\x00\x01"))[:16])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, mac(prk, []byte("Content-Encoding: nonce\x00\x01"))[:12], ct, nil)
	if err != nil {
		return nil, err
	}
	if n := len(plain); n == 0 || plain[n-1] != 2 {
		return nil, errors.New("push body isn't a single final record")
	}
	return plain[:len(plain)-1], nil
}
