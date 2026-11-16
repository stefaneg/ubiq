package main

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusal runs ubiq serve with args and returns why it refused to start.
func refusal(t *testing.T, args ...string) string {
	t.Helper()
	var status bytes.Buffer
	err := run(append([]string{"serve", "-root", t.TempDir()}, args...), nil, nil, &status)
	require.Error(t, err)
	return err.Error()
}

func TestServeAcceptsEveryFlagInDevDirectModeAndListens(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })
	// the address is taken, so getting as far as listening is the proof every flag passed
	msg := refusal(t, "-origin", "file:///srv/handbook.git", "-listen", taken.Addr().String(),
		"-person", "Ada Lovelace <ada@example.com>", "-ca", "ca.pem", "-writer-claim", "email",
		"-writers", "ada@example.com,bob@example.com", "-committer", "ubiq-bot <bot@example.com>")
	assert.Contains(t, msg, "address already in use")
}

func TestServeRefusesWhenNobodyMayWrite(t *testing.T) {
	assert.Equal(t, "ubiq serve: -writers is missing, so nobody may write; name the Writer Rule's accepted values.",
		refusal(t, "-origin", "https://example.com/handbook.git", "-person", "Ada Lovelace <ada@example.com>"))
}

func TestServeRefusesWithoutAssertionKeysOrPerson(t *testing.T) {
	assert.Equal(t, "ubiq serve: give -assertion-keys behind a proxy, or -person in dev direct mode.",
		refusal(t, "-origin", "https://example.com/handbook.git", "-writers", "w"))
}

func TestServeRefusesBothAssertionKeysAndPerson(t *testing.T) {
	assert.Equal(t, "ubiq serve: give -assertion-keys or -person, not both.",
		refusal(t, "-origin", "https://example.com/handbook.git", "-writers", "w",
			"-assertion-keys", "https://authenticate.example.com/jwks.json", "-person", "Ada Lovelace <ada@example.com>"))
}

func TestServeRefusesPersonOffLoopback(t *testing.T) {
	for _, listen := range []string{"0.0.0.0:8766", ":8766", "192.0.2.1:8766", "example.com:8766"} {
		assert.Equal(t, `ubiq serve: -person is for dev direct mode on loopback, and -listen "`+listen+`" is not loopback.`,
			refusal(t, "-origin", "https://example.com/handbook.git", "-writers", "w",
				"-person", "Ada Lovelace <ada@example.com>", "-listen", listen), listen)
	}
}

func TestServeRefusesAMissingOrigin(t *testing.T) {
	assert.Equal(t, "ubiq serve: -origin is missing; give the repository's https URL.",
		refusal(t, "-writers", "w", "-person", "Ada Lovelace <ada@example.com>"))
}

func TestServeRefusesAnSSHOrigin(t *testing.T) {
	for _, origin := range []string{"git@github.com:example/handbook.git", "ssh://git@github.com/example/handbook.git"} {
		assert.Equal(t, `ubiq serve: -origin "`+origin+`" is SSH, and the push credential serves HTTPS only; give the repository's https URL.`,
			refusal(t, "-origin", origin, "-writers", "w", "-person", "Ada Lovelace <ada@example.com>"), origin)
	}
}

func TestServeRefusesAnOriginThatIsNotHTTPS(t *testing.T) {
	for _, origin := range []string{"http://example.com/handbook.git", "git://example.com/handbook.git"} {
		assert.Equal(t, `ubiq serve: -origin "`+origin+`" is not HTTPS; give the repository's https URL, or a local path in development.`,
			refusal(t, "-origin", origin, "-writers", "w", "-person", "Ada Lovelace <ada@example.com>"), origin)
	}
}

func TestServeRefusesAPersonThatIsNotNameAndEmail(t *testing.T) {
	assert.Equal(t, `ubiq serve: -person "Ada" is not "Name <email>".`,
		refusal(t, "-origin", "/srv/handbook.git", "-writers", "w", "-person", "Ada"))
}

func TestServeRefusesACommitterThatIsNotNameAndEmail(t *testing.T) {
	assert.Equal(t, `ubiq serve: -committer "bot" is not "Name <email>".`,
		refusal(t, "-origin", "/srv/handbook.git", "-writers", "w", "-person", "Ada Lovelace <ada@example.com>",
			"-committer", "bot"))
}
