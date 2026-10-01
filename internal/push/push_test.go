package push

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The worked example from RFC 8291, section 5 and appendix A.
func TestEncryptMatchesRFC8291(t *testing.T) {
	salt, _ := b64.DecodeString("DGv6ra1nlYgDCS1FRnbzlw")
	asPriv, _ := b64.DecodeString("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw")
	as, err := ecdh.P256().NewPrivateKey(asPriv)
	if err != nil {
		t.Fatal(err)
	}
	got, err := encrypt([]byte("When I grow up, I want to be a watermelon"),
		"BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		"BTBZMqHH6r4Tts7J_aSIgg", salt, as)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml" +
		"mlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPT" +
		"pK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Errorf("got  %s\nwant %s", b64.EncodeToString(got), want)
	}
}

func TestVAPIDKeyPersistsAndSigns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vapid.pem")
	s1, err := LoadOrCreate(path, "mailto:ref@example.com")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := LoadOrCreate(path, "mailto:ref@example.com")
	if err != nil || s1.PublicKey() != s2.PublicKey() {
		t.Fatalf("key changed between loads: %v", err)
	}
	if raw, _ := b64.DecodeString(s1.PublicKey()); len(raw) != 65 || raw[0] != 4 {
		t.Errorf("public key isn't an uncompressed P-256 point: %d bytes", len(raw))
	}
	jwt, err := s1.vapidJWT("https://push.example.net", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q", jwt)
	}
	var claims map[string]any
	cb, _ := b64.DecodeString(parts[1])
	json.Unmarshal(cb, &claims)
	if claims["aud"] != "https://push.example.net" || claims["sub"] != "mailto:ref@example.com" {
		t.Errorf("claims %v", claims)
	}
	sig, _ := b64.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&s1.key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("VAPID signature doesn't verify")
	}
}

func TestSendToAPushService(t *testing.T) {
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	var got []byte
	var authz, enc string
	status := http.StatusCreated
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		authz, enc = r.Header.Get("Authorization"), r.Header.Get("Content-Encoding")
		w.WriteHeader(status)
	}))
	defer srv.Close()
	s, err := LoadOrCreate(filepath.Join(t.TempDir(), "k.pem"), "mailto:ref@example.com")
	if err != nil {
		t.Fatal(err)
	}
	s.WithClient(srv.Client())
	sub := Subscription{Endpoint: srv.URL + "/push/abc", P256dh: b64.EncodeToString(ua.PublicKey().Bytes()), Auth: b64.EncodeToString(auth)}
	if err := s.Send(context.Background(), sub, []byte(`{"title":"Turn 2 is open"}`), time.Hour); err != nil {
		t.Fatal(err)
	}
	plain, err := Decrypt(got, ua, auth)
	if err != nil || string(plain) != `{"title":"Turn 2 is open"}` {
		t.Fatalf("decrypted %q, %v", plain, err)
	}
	if enc != "aes128gcm" || !strings.HasPrefix(authz, "vapid t=") || !strings.Contains(authz, ", k="+s.PublicKey()) {
		t.Errorf("headers: %q %q", enc, authz)
	}
	status = http.StatusGone
	if err := s.Send(context.Background(), sub, []byte("x"), time.Hour); !errors.Is(err, ErrGone) {
		t.Errorf("gone subscription: %v", err)
	}
}
