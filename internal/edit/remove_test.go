package edit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/edit"
	"github.com/stefaneg/ubiq/internal/glossary"
)

// remove runs Remove on text with the model read from it, and requires it to succeed.
func remove(t *testing.T, text, name string) string {
	t.Helper()
	out, err := edit.Remove(text, load(t, text), "shop", name)
	require.NoError(t, err)
	return out
}

func TestRemoveDeletesExactlyTheEntry(t *testing.T) {
	out := remove(t, shop, "customer")
	assert.Equal(t, `# Shop

Selling things.

## Language

### Ordering

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestRemoveOfTheFirstAndLastEntryKeepsTheTextAroundIt(t *testing.T) {
	text := "# Shop\n\n## Language\n\n**Order**:\nA request.\n\n**Invoice**:\nA bill for an Order.\n"
	assert.Equal(t, "# Shop\n\n## Language\n\n**Invoice**:\nA bill for an Order.\n", remove(t, text, "Order"))
	assert.Equal(t, "# Shop\n\n## Language\n\n**Order**:\nA request.\n", remove(t, text, "Invoice"))
}

func TestRemoveRoundTripsThroughTheParser(t *testing.T) {
	for _, name := range []string{"Order", "Customer", "Invoice"} {
		t.Run(name, func(t *testing.T) {
			before := load(t, shop)
			after := load(t, remove(t, shop, name))
			assert.Len(t, after.Terms, len(before.Terms)-1)
			for id, term := range after.Terms {
				assert.Equal(t, before.Terms[id].Cluster, term.Cluster, id)
				assert.Equal(t, before.Terms[id].Avoid, term.Avoid, id)
			}
		})
	}
}

func TestRemoveLeavesUnresolvedPhrasesWhereTheTermWasReferenced(t *testing.T) {
	after := load(t, remove(t, shop, "Order"))
	var phrases []string
	for _, u := range after.Unresolved {
		phrases = append(phrases, u.Phrase+" in "+u.Where)
	}
	assert.Equal(t, []string{"Order in Customer", "Order in Invoice", "Order in Relationships"}, phrases)
	assert.Equal(t, "A person who places an Order.", glossary.PlainText(after.Terms["shop/customer"].Definition))
}

func TestRemoveRefusesATermThatDoesNotExistNamingIt(t *testing.T) {
	_, err := edit.Remove(shop, load(t, shop), "shop", "Refund")
	require.EqualError(t, err, "there is no Term Refund in Shop")
}

func TestRemoveRefusesAnEmptyName(t *testing.T) {
	_, err := edit.Remove(shop, load(t, shop), "shop", "")
	require.EqualError(t, err, "the Term's name is empty")
}

func TestRemoveRefusesAnUnknownContextNamingIt(t *testing.T) {
	_, err := edit.Remove(shop, load(t, shop), "billing", "Order")
	require.EqualError(t, err, "no Context billing")
}
