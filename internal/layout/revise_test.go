package layout_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/layout"
)

// revise runs Revise on text with the model read from it, and requires it to succeed.
func revise(t *testing.T, text string, r layout.Term, newName string) string {
	t.Helper()
	out, err := layout.Revise(text, load(t, text), r, newName)
	require.NoError(t, err)
	return out
}

func TestReviseReplacesTheDefinitionAndAvoidedWordsInFullInPlace(t *testing.T) {
	out := revise(t, shop, layout.Term{Context: "shop", Name: "customer",
		Definition: "Someone who pays an Invoice\nfor Orders.", Avoid: []string{" client ", ""}}, "")
	assert.Equal(t, `# Shop

Selling things.

## Language

### Ordering

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

**Customer**:
Someone who pays an **Invoice** for **Orders**.
_Avoid_: client

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestReviseWithoutAvoidedWordsDropsTheAvoidLine(t *testing.T) {
	out := revise(t, shop, layout.Term{Context: "shop", Name: "Order", Definition: "A request for goods."}, "")
	assert.Contains(t, out, "### Ordering\n\n**Order**:\nA request for goods.\n\n**Customer**:\n")
}

func TestReviseNamingAnotherClusterMovesTheTermToItsEnd(t *testing.T) {
	out := revise(t, shop, layout.Term{Context: "shop", Name: "Order", Definition: "A request for goods.", Cluster: "billing"}, "")
	assert.Equal(t, `# Shop

Selling things.

## Language

### Ordering

**Customer**:
A person who places an **Order**.

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

**Order**:
A request for goods.

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestReviseNamingAnUnknownClusterCreatesItAfterTheLastTerm(t *testing.T) {
	out := revise(t, shop, layout.Term{Context: "shop", Name: "Customer", Definition: "A buyer.", Cluster: "People"}, "")
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

### People

**Customer**:
A buyer.

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestReviseNamingTheTermsOwnClusterLeavesItInPlace(t *testing.T) {
	out := revise(t, shop, layout.Term{Context: "shop", Name: "Order", Definition: "A request.", Cluster: "ORDERING"}, "")
	assert.Contains(t, out, "### Ordering\n\n**Order**:\nA request.\n\n**Customer**:\n")
}

func TestReviseMovingTheLastTermOfAGlossaryKeepsTheTextAroundIt(t *testing.T) {
	text := "# Shop\n\n## Language\n\n### Ordering\n\n**Order**:\nA request.\n\n### Billing\n\n**Invoice**:\nA bill.\n"
	out := revise(t, text, layout.Term{Context: "shop", Name: "Invoice", Definition: "A bill.", Cluster: "Ordering"}, "")
	assert.Equal(t, "# Shop\n\n## Language\n\n### Ordering\n\n**Order**:\nA request.\n\n**Invoice**:\nA bill.\n\n### Billing\n", out)
}

func TestReviseRefusesATermThatDoesNotExistNamingIt(t *testing.T) {
	_, err := layout.Revise(shop, load(t, shop), layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned."}, "")
	require.EqualError(t, err, "there is no Term Refund in Shop")
}

func TestReviseRefusesAnEmptyName(t *testing.T) {
	_, err := layout.Revise(shop, load(t, shop), layout.Term{Context: "shop", Name: " ", Definition: "A buyer."}, "")
	require.EqualError(t, err, "the Term's name is empty")
}

func TestReviseRefusesAnEmptyDefinition(t *testing.T) {
	_, err := layout.Revise(shop, load(t, shop), layout.Term{Context: "shop", Name: "customer", Definition: " \n"}, "")
	require.EqualError(t, err, "the definition of customer is empty")
}

func TestReviseRefusesAnUnknownContextNamingIt(t *testing.T) {
	_, err := layout.Revise(shop, load(t, shop), layout.Term{Context: "billing", Name: "Order", Definition: "A request."}, "")
	require.EqualError(t, err, "no Context billing")
}

func TestReviseRefusesARenameToAnExistingNameNamingIt(t *testing.T) {
	_, err := layout.Revise(shop, load(t, shop), layout.Term{Context: "shop", Name: "Order",
		Definition: "A request."}, "invoice")
	require.EqualError(t, err, "the Term Invoice already exists in Shop")
}
