package glossary_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func walletRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY-MAP.md": `# Glossary Map

## Contexts

- [Wallet](./wallet/GLOSSARY.md): holds funds
- [Daemon](./daemon/GLOSSARY.md): hosts a wallet

## Relationships

- **Daemon → Wallet**: the daemon controls one Wallet and its Mints.
`,
		"wallet/GLOSSARY.md": `# Wallet

## Language

**Wallet**:
A user's holding. A Wallet is rebuilt from its Wallet Seed, never from a ` + "`Wallet`" + ` value or [Wallet](x.md).
_Avoid_: account

**Wallet Seed**:
The secret root of a wallet. Wallet's identity derives from it, as do the Units of Work.

**Mint**:
A server that issues Mints Wallet funds; Mint Wallet Seed pairs are kept. Cashu and BIP39 are not terms.

**Unit of Work**:
One task.

**dfsu**:
A file format. Another dfsu file mentions Mint.
`,
		"daemon/GLOSSARY.md": `# Daemon

## Language

**Host**:
The process that serves one Wallet.
`,
	})
	return root
}

func TestCapitalisedMentionsAreReferences(t *testing.T) {
	model, err := glossary.Load(walletRoot(t))
	require.NoError(t, err)
	refs := func(id string) []string { return model.Terms[id].Refs }
	assert.Equal(t, []string{"wallet/wallet-seed"}, refs("wallet/wallet"),
		"the longest name wins, a self-mention does not count, and code spans and links are left alone")
	assert.Equal(t, []string{"wallet/wallet", "wallet/unit-of-work"}, refs("wallet/wallet-seed"),
		"a lowercase mention does not count; a possessive and a head-noun plural do")
	assert.Equal(t, []string{"wallet/wallet", "wallet/wallet-seed"}, refs("wallet/mint"),
		"adjacent names both count; a plural of self does not")
	assert.Equal(t, []string{"wallet/mint"}, refs("wallet/dfsu"), "a lowercase name counts only in bold")
	assert.Equal(t, []string{"wallet/wallet"}, refs("daemon/host"), "a mention resolves in other contexts too")
	assert.Empty(t, model.Unresolved, "a capitalised phrase that names no term is ignored")
	assert.Contains(t, model.Terms["wallet/wallet"].Definition, `<a class="t" href="#wallet/wallet-seed" data-id="wallet/wallet-seed">Wallet Seed</a>`)
	assert.Contains(t, model.Terms["wallet/wallet"].Definition, "<code>Wallet</code>")
}

func TestCapitalisedMentionsInContextRelationshipsAreReferences(t *testing.T) {
	model, err := glossary.Load(walletRoot(t))
	require.NoError(t, err)
	require.Len(t, model.Relationships, 1)
	assert.Equal(t, []string{"wallet/wallet", "wallet/mint"}, model.Relationships[0].TermIDs)
}
