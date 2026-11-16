package layout_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/glossarytest"
	"github.com/stefaneg/ubiq/internal/layout"
)

// orders mentions Order in bold and plain, singular, plural and possessive, beside Order Line, code and an Avoid line.
const orders = `# Shop

Orders and an Order's lines.

## Language

**Order**:
A request for goods.
_Avoid_: Order request

**Order Line**:
One row of an **Order**, never of two Orders.

**Invoice**:
A bill for **orders** or an **Order's** lines, not ` + "`Order`" + ` or an order.

## Relationships

- An **Invoice** follows an Order; see [Order](#order) and **Order Lines**.

` + "```" + `
**Order** in code.
` + "```" + `
`

func TestReviseWithANewNameRewritesEveryReferenceInItsGlossary(t *testing.T) {
	out := revise(t, orders, layout.Term{Context: "shop", Name: "order", Definition: "A request for goods."}, "Purchase")
	assert.Equal(t, `# Shop

Purchases and an Purchase's lines.

## Language

**Purchase**:
A request for goods.

**Order Line**:
One row of an **Purchase**, never of two Purchases.

**Invoice**:
A bill for **Purchases** or an **Purchase's** lines, not `+"`Order`"+` or an order.

## Relationships

- An **Invoice** follows an Purchase; see [Order](#order) and **Order Lines**.

`+"```"+`
**Order** in code.
`+"```"+`
`, out)
}

func TestARenameLeavesNoUnresolvedPhraseBehind(t *testing.T) {
	out := revise(t, orders, layout.Term{Context: "shop", Name: "Order", Definition: "Goods chosen."}, "Basket")
	model := load(t, out)
	assert.Empty(t, model.Unresolved)
	assert.ElementsMatch(t, []string{"shop/basket"}, model.Terms["shop/invoice"].Refs)
}

func TestARenameWithAMoveRoundTripsThroughTheParser(t *testing.T) {
	out := revise(t, shop, layout.Term{Context: "shop", Name: "Order", Cluster: "Billing",
		Definition: "A request for goods by a Customer.", Avoid: []string{"order"}}, "Purchase")
	after := load(t, out)
	require.Len(t, after.Terms, 3, out)
	purchase := after.Terms["shop/purchase"]
	require.NotNil(t, purchase, out)
	assert.Equal(t, "Billing", purchase.Cluster)
	assert.Equal(t, "order", purchase.Avoid)
	assert.Equal(t, []string{"shop/customer"}, purchase.Refs)
	assert.Equal(t, []string{"shop/purchase"}, after.Terms["shop/invoice"].Refs)
	assert.Empty(t, after.Unresolved)
	assert.Contains(t, out, "- An **Invoice** follows an **Purchase**.\n")
}

func TestARenameLeavesHeadingsAlone(t *testing.T) {
	text := "# Order\n\n## Language\n\n### Order handling\n\n**Order**:\nA request.\n\n## Relationships\n\n### Orders and Invoices\n\n- An Order.\n"
	out := revise(t, text, layout.Term{Context: "order", Name: "Order", Definition: "A request."}, "Basket")
	assert.Equal(t, "# Order\n\n## Language\n\n### Order handling\n\n**Basket**:\nA request.\n\n## Relationships\n\n### Orders and Invoices\n\n- An Basket.\n", out)
}

func TestARenamePluralisesTheNewName(t *testing.T) {
	text := "# Shop\n\n## Language\n\n**Order**:\nA request.\n\n**Invoice**:\nA bill for Orders and **Orders**.\n"
	for to, want := range map[string]string{
		"Batch":          "A bill for Batches and **Batches**.",
		"Party":          "A bill for Parties and **Parties**.",
		"Day":            "A bill for Days and **Days**.",
		"Unit of Work":   "A bill for Units of Work and **Units of Work**.",
		"Purchase Order": "A bill for Purchase Orders and **Purchase Orders**.",
	} {
		t.Run(to, func(t *testing.T) {
			out := revise(t, text, layout.Term{Context: "shop", Name: "Order", Definition: "A request."}, to)
			assert.Contains(t, out, "**Invoice**:\n"+want+"\n")
			assert.Empty(t, load(t, out).Unresolved, out)
		})
	}
}

func TestARenameRewritesOnlyReferencesThatResolveToTheTerm(t *testing.T) {
	root := glossarytest.Root(t)
	model, err := glossary.Load(root)
	require.NoError(t, err)
	read := func(path string) string {
		b, err := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, err)
		return string(b)
	}
	// Common's Run is renamed; Workspace has its own Run, and Stacks, which has none, resolves Run in Workspace, the
	// first Context of the map that defines it, so neither Glossary changes.
	stacks := read("internal/stacks/GLOSSARY.md")
	assert.Equal(t, stacks, layout.Rename(stacks, model, "stacks", "common", "Run", "Job"))
	workspace := read("internal/workspace/GLOSSARY.md")
	assert.Equal(t, workspace, layout.Rename(workspace, model, "workspace", "common", "Run", "Job"))

	// Workspace's Image is renamed: Stacks and the Context Map refer to it.
	assert.Equal(t, `# Stacks

The envctl core.

## Language

### Environment Model

**Stack**:
A named **Stack Image** in an environment, deployed by a **Run**.
_Avoid_: deployment, deploy

**Stack Image**:
An image a **Stack** runs, identified by a workspace **Picture**.
_Avoid_: stack container
`, layout.Rename(stacks, model, "stacks", "workspace", "Image", "Picture"))
	assert.Contains(t, layout.Rename(read("GLOSSARY-MAP.md"), model, "", "workspace", "Image", "Picture"),
		"- **Workspace → Stacks**: a **Stack Image** names a workspace **Picture**.\n")
}
