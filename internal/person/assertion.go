package person

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// AssertionHeader is the header the proxy forwards the Person in, as a signed JWT.
const AssertionHeader = "X-Pomerium-Jwt-Assertion"

// WriterRule tells a Writer from a Reader: one claim of the proxy's assertion and the values it accepts. A claim that
// is a list matches when any element is accepted, a string claim when it equals one.
type WriterRule struct {
	Claim  string
	Values []string
}

// Role is Writer when claims satisfy the rule and Reader otherwise; never Maintainer.
func (r WriterRule) Role(claims map[string]any) Role {
	switch v := claims[r.Claim].(type) {
	case string:
		if slices.Contains(r.Values, v) {
			return Writer
		}
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && slices.Contains(r.Values, s) {
				return Writer
			}
		}
	}
	return Reader
}

// Assertion resolves a request to the Person named by the proxy's signed assertion: it verifies the assertion against
// the key set published at keys, trusting the PEM certificates in ca besides the system's roots, and gives the Person
// the role rule decides. It fetches the key set on first use and again whenever an assertion names a key id the set
// lacks, so the proxy may rotate its keys without a restart.
func Assertion(keys string, ca []byte, rule WriterRule) (Resolver, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if len(ca) > 0 && !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("the CA holds no PEM certificate")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	return &verifier{
		keys:   keys,
		client: &http.Client{Transport: transport, Timeout: 10 * time.Second},
		rule:   rule,
	}, nil
}

// algorithms are the signatures an assertion may carry; Pomerium signs with ES256 by default.
var algorithms = []jose.SignatureAlgorithm{jose.ES256, jose.ES384, jose.ES512, jose.RS256, jose.PS256, jose.EdDSA}

type verifier struct {
	keys   string
	client *http.Client
	rule   WriterRule

	mu  sync.Mutex
	set *jose.JSONWebKeySet
}

func (v *verifier) Resolve(header http.Header) (Person, error) {
	raw := header.Get(AssertionHeader)
	if raw == "" {
		return Person{}, fmt.Errorf("no %s header: not behind the proxy, or it passes no identity headers", AssertionHeader)
	}
	tok, err := jwt.ParseSigned(raw, algorithms)
	if err != nil {
		return Person{}, fmt.Errorf("the assertion is not a signed JWT: %w", err)
	}
	if len(tok.Headers) != 1 || tok.Headers[0].KeyID == "" {
		return Person{}, errors.New("the assertion names no key id")
	}
	key, err := v.key(tok.Headers[0].KeyID)
	if err != nil {
		return Person{}, err
	}
	var std jwt.Claims
	var all map[string]any
	if err := tok.Claims(key, &std, &all); err != nil {
		return Person{}, fmt.Errorf("the assertion's signature does not verify: %w", err)
	}
	if err := std.ValidateWithLeeway(jwt.Expected{Time: time.Now()}, jwt.DefaultLeeway); err != nil {
		return Person{}, fmt.Errorf("the assertion is not valid now: %w", err)
	}
	name, _ := all["name"].(string)
	email, _ := all["email"].(string)
	return Person{Subject: std.Subject, Name: name, Email: email, Role: v.rule.Role(all)}, nil
}

// key is the public key with id kid, fetching the key set when it is not yet known or lacks kid.
func (v *verifier) key(kid string) (*jose.JSONWebKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.set != nil {
		if found := v.set.Key(kid); len(found) > 0 {
			return &found[0], nil
		}
	}
	set, err := v.fetch()
	if err != nil {
		return nil, err
	}
	v.set = set
	if found := set.Key(kid); len(found) > 0 {
		return &found[0], nil
	}
	return nil, fmt.Errorf("the assertion's key id %q is not in the key set at %s", kid, v.keys)
}

// fetch reads the key set at the keys URL.
func (v *verifier) fetch() (*jose.JSONWebKeySet, error) {
	res, err := v.client.Get(v.keys)
	if err != nil {
		return nil, fmt.Errorf("fetch the assertion keys: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch the assertion keys at %s: %s", v.keys, res.Status)
	}
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(res.Body).Decode(&set); err != nil {
		return nil, fmt.Errorf("read the assertion keys at %s: %w", v.keys, err)
	}
	return &set, nil
}
