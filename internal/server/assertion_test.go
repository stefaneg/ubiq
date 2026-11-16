package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossarytest"
	"github.com/stefaneg/ubiq/internal/person"
)

// signingKey is a key the proxy signs assertions with, named by its key id.
type signingKey struct {
	id  string
	key *ecdsa.PrivateKey
}

func newSigningKey(t *testing.T, id string) signingKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return signingKey{id: id, key: k}
}

// sign makes an assertion with claims, signed by k.
func (k signingKey) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: k.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", k.id))
	require.NoError(t, err)
	raw, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(t, err)
	return raw
}

// keyServer publishes the key set a proxy signs its assertions with, as Pomerium does, and counts its fetches.
type keyServer struct {
	*httptest.Server
	mu      sync.Mutex
	keys    []signingKey
	fetches atomic.Int32
}

// newKeyServer serves keys from a second test server, over TLS under its own CA when tls is set.
func newKeyServer(t *testing.T, tls bool, keys ...signingKey) *keyServer {
	t.Helper()
	ks := &keyServer{keys: keys}
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ks.fetches.Add(1)
		ks.mu.Lock()
		defer ks.mu.Unlock()
		var set jose.JSONWebKeySet
		for _, k := range ks.keys {
			set.Keys = append(set.Keys, jose.JSONWebKey{Key: &k.key.PublicKey, KeyID: k.id, Algorithm: "ES256", Use: "sig"})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(set)
	})
	if tls {
		ks.Server = httptest.NewTLSServer(h)
	} else {
		ks.Server = httptest.NewServer(h)
	}
	t.Cleanup(ks.Close)
	return ks
}

// rotate replaces the published keys.
func (ks *keyServer) rotate(keys ...signingKey) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.keys = keys
}

// writers is the Writer Rule the tests run under unless they say otherwise.
var writers = person.WriterRule{Claim: "groups", Values: []string{"glossary-writers"}}

// verifier is the assertion verifier for the keys ks publishes, under rule.
func verifier(t *testing.T, ks *keyServer, rule person.WriterRule) person.Resolver {
	t.Helper()
	r, err := person.Assertion(ks.URL, nil, rule)
	require.NoError(t, err)
	return r
}

// claims are an assertion's claims for Ada, valid for an hour, with extra merged in.
func claims(extra map[string]any) map[string]any {
	c := map[string]any{
		"sub":   "ada-1815",
		"name":  "Ada Lovelace",
		"email": "ada@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// getAs fetches url with assertion in the proxy's header and returns the status and body.
func getAs(t *testing.T, url, assertion string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if assertion != "" {
		req.Header.Set("X-Pomerium-Jwt-Assertion", assertion)
	}
	return do(t, req)
}

func TestThePageShowsTheWriterFromASignedAssertion(t *testing.T) {
	key := newSigningKey(t, "k1")
	url, _ := serve(t, glossarytest.Root(t), verifier(t, newKeyServer(t, false, key), writers))
	status, body := getAs(t, url+"/", key.sign(t, claims(map[string]any{"groups": []string{"staff", "glossary-writers"}})))
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"person":{"name":"Ada Lovelace","role":"Writer"}`)
}

func TestThePageShowsAReaderWhenTheWriterRuleDoesNotMatch(t *testing.T) {
	key := newSigningKey(t, "k1")
	url, _ := serve(t, glossarytest.Root(t), verifier(t, newKeyServer(t, false, key), writers))
	for _, extra := range []map[string]any{{"groups": []string{"staff"}}, {"groups": "staff"}, {}} {
		status, body := getAs(t, url+"/", key.sign(t, claims(extra)))
		assert.Equal(t, http.StatusOK, status)
		assert.Contains(t, body, `"person":{"name":"Ada Lovelace","role":"Reader"}`, extra)
	}
}

func TestAStringClaimServesAsTheWriterRulesClaim(t *testing.T) {
	key := newSigningKey(t, "k1")
	rule := person.WriterRule{Claim: "email", Values: []string{"bob@example.com", "ada@example.com"}}
	url, _ := serve(t, glossarytest.Root(t), verifier(t, newKeyServer(t, false, key), rule))
	_, body := getAs(t, url+"/", key.sign(t, claims(nil)))
	assert.Contains(t, body, `"person":{"name":"Ada Lovelace","role":"Writer"}`)
	_, body = getAs(t, url+"/", key.sign(t, claims(map[string]any{"email": "eve@example.com"})))
	assert.Contains(t, body, `"role":"Reader"`)
}

func TestAnAssertionThatDoesNotVerifyIsUnauthorisedWithTheReasonLogged(t *testing.T) {
	key, other := newSigningKey(t, "k1"), newSigningKey(t, "k1")
	stranger := newSigningKey(t, "k9")
	url, log := serve(t, glossarytest.Root(t), verifier(t, newKeyServer(t, false, key), writers))
	for _, c := range []struct{ name, assertion, reason string }{
		{"none", "", "no X-Pomerium-Jwt-Assertion header"},
		{"expired", key.sign(t, claims(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})), "expired"},
		{"signed by another key", other.sign(t, claims(nil)), "signature"},
		{"key id not in the key set", stranger.sign(t, claims(nil)), `key id "k9" is not in the key set`},
		{"not a JWT", "garbage", "not a signed JWT"},
	} {
		log.Reset()
		status, body := getAs(t, url+"/", c.assertion)
		assert.Equal(t, http.StatusUnauthorized, status, c.name)
		assert.Equal(t, "unauthorised\n", body, c.name)
		assert.Contains(t, log.String(), "GET / 401 unauthorised: ", c.name)
		assert.Contains(t, log.String(), c.reason, c.name)
	}
}

func TestTheVerifierTrustsAKeySetUnderAPrivateCAOnlyWhenGivenIt(t *testing.T) {
	key := newSigningKey(t, "k1")
	ks := newKeyServer(t, true, key)
	assertion := key.sign(t, claims(nil))

	untrusting, log := serve(t, glossarytest.Root(t), verifier(t, ks, writers))
	status, _ := getAs(t, untrusting+"/", assertion)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Contains(t, log.String(), "certificate")

	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ks.Certificate().Raw})
	trusting, err := person.Assertion(ks.URL, ca, writers)
	require.NoError(t, err)
	url, _ := serve(t, glossarytest.Root(t), trusting)
	status, body := getAs(t, url+"/", assertion)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `"person":{"name":"Ada Lovelace","role":"Reader"}`)
}

func TestACAWithNoCertificateIsRefused(t *testing.T) {
	_, err := person.Assertion("https://authenticate.example.com/jwks.json", []byte("not a certificate"), writers)
	assert.EqualError(t, err, "the CA holds no PEM certificate")
}

func TestTheKeySetIsFetchedLazilyAndRefreshedWhenTheProxyRotatesItsKey(t *testing.T) {
	first, second := newSigningKey(t, "k1"), newSigningKey(t, "k2")
	ks := newKeyServer(t, false, first)
	url, _ := serve(t, glossarytest.Root(t), verifier(t, ks, writers))
	assert.Zero(t, ks.fetches.Load(), "nothing fetched before the first request")

	for range 3 {
		status, _ := getAs(t, url+"/", first.sign(t, claims(nil)))
		assert.Equal(t, http.StatusOK, status)
	}
	assert.Equal(t, int32(1), ks.fetches.Load(), "fetched once, then kept")

	ks.rotate(second)
	status, _ := getAs(t, url+"/", second.sign(t, claims(nil)))
	assert.Equal(t, http.StatusOK, status, "the new key is fetched without a restart")
	assert.Equal(t, int32(2), ks.fetches.Load())
}

// assertingTransport sends every request as the proxy would: with the upstream's Host and the assertion.
type assertingTransport struct{ assertion string }

func (a assertingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("X-Pomerium-Jwt-Assertion", a.assertion)
	return hostRewriter{"mcp.glossary.example.com"}.RoundTrip(r)
}

func TestWhoamiAnswersThePersonFromTheAssertion(t *testing.T) {
	key := newSigningKey(t, "k1")
	url, _ := serve(t, glossarytest.Root(t), verifier(t, newKeyServer(t, false, key), writers))
	writer := key.sign(t, claims(map[string]any{"groups": []string{"glossary-writers"}}))
	text, isErr := call(t, connectVia(t, url, assertingTransport{writer}), "whoami", nil)
	assert.False(t, isErr)
	assert.Equal(t, "name: Ada Lovelace\nemail: ada@example.com\nrole: Writer\n", text)

	text, _ = call(t, connectVia(t, url, assertingTransport{key.sign(t, claims(nil))}), "whoami", nil)
	assert.Equal(t, "name: Ada Lovelace\nemail: ada@example.com\nrole: Reader\n", text)
}
