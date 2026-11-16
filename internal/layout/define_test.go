package layout_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/glossarytest"
	"github.com/stefaneg/ubiq/internal/layout"
)

// shop is a single-Context Glossary with two Clusters and a section after its Terms.
const shop = `# Shop

Selling things.

## Language

### Ordering

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

**Customer**:
A person who places an **Order**.

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

## Relationships

- An **Invoice** follows an **Order**.
`

// load reads text as a single-Context repository's GLOSSARY.md.
func load(t *testing.T, text string) *glossary.Model {
	t.Helper()
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": text})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	return model
}

// define runs Define on text with the model read from it, and requires it to succeed.
func define(t *testing.T, text string, term layout.Term) string {
	t.Helper()
	out, err := layout.Define(text, load(t, text), term)
	require.NoError(t, err)
	return out
}

func TestDefineWithoutClusterGoesAfterTheLastTerm(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned."})
	assert.Equal(t, `# Shop

Selling things.

## Language

### Ordering

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

**Customer**:
A person who places an **Order**.

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

**Refund**:
Money returned.

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestDefineInAClusterGoesAtTheEndOfThatCluster(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Basket", Definition: "Goods chosen.", Cluster: "ordering"})
	assert.Equal(t, `# Shop

Selling things.

## Language

### Ordering

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

**Customer**:
A person who places an **Order**.

**Basket**:
Goods chosen.

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestDefineInAnUnknownClusterCreatesItAfterTheLastTermBeforeTheNextSection(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Parcel", Definition: "A box sent.", Cluster: "Shipping"})
	assert.Equal(t, `# Shop

Selling things.

## Language

### Ordering

**Order**:
A request for goods by a **Customer**.
_Avoid_: purchase

**Customer**:
A person who places an **Order**.

### Billing

**Invoice**:
A request for payment for an **Order**.
_Avoid_: bill

### Shipping

**Parcel**:
A box sent.

## Relationships

- An **Invoice** follows an **Order**.
`, out)
}

func TestDefineMakesEveryMentionOfATermOrItsPluralAReference(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Refund",
		Definition: "Money a Customer gets back for Orders, settled against an Invoice."})
	assert.Contains(t, out, "**Refund**:\nMoney a **Customer** gets back for **Orders**, settled against an **Invoice**.\n")
}

func TestDefineMakesTheLongestTermNameTheReference(t *testing.T) {
	text := `# Shop

## Language

**Order**:
A request for goods.

**Order Line**:
One row of an **Order**.

**Unit of Work**:
A task.
`
	out := define(t, text, layout.Term{Context: "shop", Name: "Picking",
		Definition: "Taking the Order Lines of an Order off the shelf, as Units of Work."})
	assert.Contains(t, out, "**Picking**:\nTaking the **Order Lines** of an **Order** off the shelf, as **Units of Work**.\n")
}

func TestDefineNeverRewritesEarlierDefinitions(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Person", Definition: "A human."})
	assert.Contains(t, out, "**Customer**:\nA person who places an **Order**.\n")
}

func TestDefineMatchesTermNamesWithoutRegardToCase(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Refund", Definition: "Money a customer gets back for ORDERS."})
	assert.Contains(t, out, "**Refund**:\nMoney a **customer** gets back for **ORDERS**.\n")
}

func TestDefineMatchesWholeWordsOnly(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Refund", Definition: "Reorders and customers-only orderly invoicing."})
	assert.Contains(t, out, "**Refund**:\nReorders and customers-only orderly invoicing.\n")
}

func TestDefineLeavesProperNounsTheGlossaryLacksAsTyped(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Login", Definition: "A Customer signing in through OpenID Connect."})
	assert.Contains(t, out, "**Login**:\nA **Customer** signing in through OpenID Connect.\n")
}

func TestDefineWritesAvoidedWordsOnAnAvoidLineUnderTheDefinition(t *testing.T) {
	out := define(t, shop, layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned.",
		Avoid: []string{" chargeback", "", "money back "}})
	assert.Contains(t, out, "**Refund**:\nMoney returned.\n_Avoid_: chargeback, money back\n\n## Relationships")
}

func TestDefineRefusesATermThatExistsNamingIt(t *testing.T) {
	_, err := layout.Define(shop, load(t, shop), layout.Term{Context: "shop", Name: "customer", Definition: "A buyer."})
	require.EqualError(t, err, "the Term Customer already exists in Shop")
}

func TestDefineRefusesAnEmptyName(t *testing.T) {
	_, err := layout.Define(shop, load(t, shop), layout.Term{Context: "shop", Name: " ", Definition: "A buyer."})
	require.EqualError(t, err, "the Term's name is empty")
}

func TestDefineRefusesAnEmptyDefinition(t *testing.T) {
	_, err := layout.Define(shop, load(t, shop), layout.Term{Context: "shop", Name: "Refund", Definition: " \n"})
	require.EqualError(t, err, "the definition of Refund is empty")
}

func TestDefineRefusesAnUnknownContextNamingIt(t *testing.T) {
	_, err := layout.Define(shop, load(t, shop), layout.Term{Context: "billing", Name: "Refund", Definition: "Money returned."})
	require.EqualError(t, err, "no Context billing")
}

func TestDefineAllowsANameAnotherContextDefinesAndLeavesItsOwnNamePlain(t *testing.T) {
	root := glossarytest.Root(t)
	model, err := glossary.Load(root)
	require.NoError(t, err)
	text, err := os.ReadFile(filepath.Join(root, "GLOSSARY.md"))
	require.NoError(t, err)
	out, err := layout.Define(string(text), model, layout.Term{Context: "common", Name: "Workspace",
		Definition: "A desk; Workspaces hold a Stack."})
	require.NoError(t, err)
	assert.Contains(t, out, "**Workspace**:\nA desk; Workspaces hold a **Stack**.\n")
}

func TestDefineTouchesNothingButTheNewTerm(t *testing.T) {
	text := "# Shop\n\n## Language\n\n### Ordering\n\n**Order**:\nA request.\n\n\n### Billing\n**Invoice**:\nA bill.\n\n\n## Relationships\n\n- An Invoice   follows an Order."
	cases := []struct{ cluster, lines string }{
		{"", "\n**Refund**:\nMoney returned.\n"},
		{"Ordering", "\n**Refund**:\nMoney returned.\n"},
		{"Billing", "\n**Refund**:\nMoney returned.\n"},
		{"Shipping", "\n### Shipping\n\n**Refund**:\nMoney returned.\n"},
	}
	for _, c := range cases {
		t.Run("Cluster "+c.cluster, func(t *testing.T) {
			out := define(t, text, layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned.", Cluster: c.cluster})
			require.Equal(t, 1, strings.Count(out, c.lines), out)
			assert.Equal(t, text, strings.Replace(out, c.lines, "", 1))
		})
	}
}

func TestDefineSeparatesTheTermFromAHeadingRightBelowIt(t *testing.T) {
	text := "# Shop\n\n## Language\n\n### Ordering\n**Order**:\nA request.\n### Billing\n**Invoice**:\nA bill.\n"
	out := define(t, text, layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned.", Cluster: "Ordering"})
	assert.Equal(t, "# Shop\n\n## Language\n\n### Ordering\n**Order**:\nA request.\n\n**Refund**:\nMoney returned.\n\n### Billing\n**Invoice**:\nA bill.\n", out)
}

func TestDefineIgnoresTermsAndHeadingsInFencedCode(t *testing.T) {
	text := "# Shop\n\n## Language\n\n### Ordering\n\n**Order**:\nA request.\n\n### Billing\n\n```md\n## Example\n\n**Bill**:\nNot a Term.\n```\n\n**Invoice**:\nA bill.\n\n```md\n**Fake**:\nNot a Term.\n```\n"
	out := define(t, text, layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned."})
	assert.Equal(t, strings.Replace(text, "A bill.\n", "A bill.\n\n**Refund**:\nMoney returned.\n", 1), out)
	out = define(t, text, layout.Term{Context: "shop", Name: "Refund", Definition: "Money returned.", Cluster: "example"})
	assert.Equal(t, text+"\n### example\n\n**Refund**:\nMoney returned.\n", out)
}

const empty = `# Shop

Selling things.

## Language

## Relationships

- None yet.
`

const flat = `# Shop

## Language

**Order**:
A request for goods.
`

func TestDefineRoundTripsThroughTheParserWithTheTermInItsCluster(t *testing.T) {
	cases := []struct {
		name, text, cluster, want string
	}{
		{"no Cluster named takes the last Term's", shop, "", "Billing"},
		{"a known Cluster", shop, "Ordering", "Ordering"},
		{"an unknown Cluster", shop, "Shipping", "Shipping"},
		{"no Terms yet", empty, "", "General"},
		{"no Terms yet, an unknown Cluster", empty, "Shipping", "Shipping"},
		{"Terms under Language only", flat, "", "General"},
		{"Terms under Language only, an unknown Cluster", flat, "Shipping", "Shipping"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := define(t, c.text, layout.Term{Context: "shop", Name: "Refund",
				Definition: "Money\nreturned for an Order.\n\nSettled monthly.", Cluster: c.cluster})
			after := load(t, out)
			refund := after.Terms["shop/refund"]
			require.NotNil(t, refund, out)
			assert.Equal(t, c.want, refund.Cluster, out)
			assert.Equal(t, "Money returned for an Order. Settled monthly.", glossary.PlainText(refund.Definition), out)
			assert.Len(t, after.Terms, len(load(t, c.text).Terms)+1, out)
		})
	}
}

func TestDefineInAnUnknownClusterSkipsTextAfterTheLastTerm(t *testing.T) {
	text := `# Shop

## Language

### Ordering

**Order**:
A request for goods.

Orders are numbered.

## Relationships

- None.
`
	out := define(t, text, layout.Term{Context: "shop", Name: "Parcel", Definition: "A box sent.", Cluster: "Shipping"})
	assert.Equal(t, `# Shop

## Language

### Ordering

**Order**:
A request for goods.

Orders are numbered.

### Shipping

**Parcel**:
A box sent.

## Relationships

- None.
`, out)
}
