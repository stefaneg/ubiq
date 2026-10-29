package glossary_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefaneg/ubiq/internal/glossary"
	"github.com/stefaneg/ubiq/internal/glossarytest"
)

func TestLoadReadsTheContextsTheMapLinks(t *testing.T) {
	model := glossarytest.Load(t)
	var keys, paths []string
	for _, c := range model.Contexts {
		keys = append(keys, c.Key)
		paths = append(paths, c.Path)
	}
	assert.Equal(t, []string{"workspace", "stacks", "common"}, keys, "map order")
	assert.Equal(t, []string{"internal/workspace/GLOSSARY.md", "internal/stacks/GLOSSARY.md", "GLOSSARY.md"}, paths)
}

func TestLoadWithoutAMapReadsTheRootGlossaryAsTheOnlyContext(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY.md": `# Ordering

## Language

**Order**:
A request for goods by a **Customer**.

**Customer**:
A person who places an **Order**.
`,
		"docs/adr/0001-orders-are-immutable.md": "# Orders are immutable\n\nAn Order never changes once placed.\n",
	})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	require.Len(t, model.Contexts, 1)
	assert.Equal(t, "ordering", model.Contexts[0].Key)
	assert.Equal(t, "GLOSSARY.md", model.Contexts[0].Path)
	assert.Equal(t, []string{"ordering/customer"}, model.Terms["ordering/order"].Refs)
	require.Len(t, model.ADRs, 1)
	assert.Equal(t, "ordering", model.ADRs[0].Context, "without a context map, the root ADRs belong to the only context")
	assert.Equal(t, []string{"ordering/order"}, model.ADRs[0].TermIDs)
}

// contextsOnlyRoot is a multi-context repo laid out as the domain-modeling skill shows it: no root glossary, one
// GLOSSARY.md and docs/adr per context.
func contextsOnlyRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY-MAP.md": `# Glossary Map

## Contexts

- [Ordering](./src/ordering/GLOSSARY.md): receives and tracks customer orders
- [Billing](./src/billing/GLOSSARY.md): generates invoices

## Relationships

- **Ordering → Billing**: Ordering emits events Billing consumes.

## Cross-context vocabulary

### Deliberate mirrors

#### Customer

- Billing: a **Customer** receives an **Invoice**.
- Ordering: a **Customer** places an **Order**.
`,
		"src/ordering/GLOSSARY.md": `# Ordering

## Language

**Order**:
A request for goods by a **Customer**.

**Customer**:
A person who places an **Order**.
`,
		"src/billing/GLOSSARY.md": `# Billing

## Language

**Invoice**:
A request for payment sent to a **Customer**.

**Customer**:
A person or organisation that pays an **Invoice**.
`,
		"docs/adr/0001-bill-on-shipment.md":              "# Bill on shipment\n\nAn Invoice is raised for a Customer once goods ship.\n",
		"src/ordering/docs/adr/0001-orders-are-final.md": "# Orders are final\n\nA Customer cannot amend an Order.\n",
	})
	return root
}

func TestLoadWithoutARootGlossaryKeepsMapOrder(t *testing.T) {
	model, err := glossary.Load(contextsOnlyRoot(t))
	require.NoError(t, err)
	require.Len(t, model.Contexts, 2)
	assert.Equal(t, "ordering", model.Contexts[0].Key)
	assert.Equal(t, "src/ordering/GLOSSARY.md", model.Contexts[0].Path)
	assert.Equal(t, "billing", model.Contexts[1].Key)
	assert.Empty(t, model.Unresolved)
}

func TestLoadReadsEachContextsOwnADRs(t *testing.T) {
	model, err := glossary.Load(contextsOnlyRoot(t))
	require.NoError(t, err)
	adrs := map[string]glossary.ADR{}
	for _, a := range model.ADRs {
		adrs[a.ID] = a
	}
	require.Len(t, adrs, 2)
	system := adrs["adr/0001-bill-on-shipment"]
	assert.Equal(t, "docs/adr/0001-bill-on-shipment.md", system.Path)
	assert.Empty(t, system.Context, "a root ADR is system-wide when there is a context map")
	own := adrs["adr/ordering/0001-orders-are-final"]
	assert.Equal(t, "src/ordering/docs/adr/0001-orders-are-final.md", own.Path)
	assert.Equal(t, "ordering", own.Context, "a context's own ADR stays in that context")
	assert.Equal(t, []string{"ordering/customer", "ordering/order"}, own.TermIDs)
}

func TestLoadErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"no glossary": {
			files: map[string]string{"README.md": "# Readme\n"},
			want:  "no GLOSSARY-MAP.md or GLOSSARY.md",
		},
		"map without contexts": {
			files: map[string]string{"GLOSSARY-MAP.md": "# Map\n\n## Contexts\n\nNone yet.\n"},
			want:  "GLOSSARY-MAP.md links no context glossary under ## Contexts",
		},
		"missing context glossary": {
			files: map[string]string{"GLOSSARY-MAP.md": "# Map\n\n## Contexts\n\n- [Ordering](src/ordering/GLOSSARY.md)\n"},
			want:  "read glossary src/ordering/GLOSSARY.md",
		},
		"two contexts with one name": {
			files: map[string]string{
				"GLOSSARY-MAP.md": "# Map\n\n## Contexts\n\n- [A](a/GLOSSARY.md)\n- [B](b/GLOSSARY.md)\n",
				"a/GLOSSARY.md":   "# Sales\n",
				"b/GLOSSARY.md":   "# Sales\n",
			},
			want: `b/GLOSSARY.md: context key "sales" is already used by a/GLOSSARY.md`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			glossarytest.WriteTree(t, root, tc.files)
			_, err := glossary.Load(root)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoadReadsTheMapsRelationships(t *testing.T) {
	model := glossarytest.Load(t)
	require.Len(t, model.Relationships, 2)
	first, second := model.Relationships[0], model.Relationships[1]
	assert.Equal(t, "Workspace → Stacks", first.Label)
	assert.Equal(t, []string{"workspace", "stacks"}, first.Contexts)
	assert.Equal(t, []string{"stacks/stack-image", "workspace/image"}, first.TermIDs)
	assert.Contains(t, first.HTML, `data-id="workspace/image"`)
	assert.Equal(t, []string{"stacks", "common"}, second.Contexts)
	assert.Equal(t, []string{"stacks/stack-image", "common/image-ref"}, second.TermIDs, "continuation lines join the bullet")
}

func TestRelationshipLabelsNameContextsByHeadingOrMapLink(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{
		"GLOSSARY-MAP.md": `# Glossary Map

## Contexts

- [Orders](./ordering/GLOSSARY.md): takes orders
- [Billing](./billing/GLOSSARY.md): sends invoices

## Relationships

- **Orders -> Billing**: each **Order** becomes an **Invoice**.
- **Billing ↔ Ordering**: a **Refund** reverses an **Invoice**.
- **Shipping → Billing**: no such context.
- Without a label.
`,
		"ordering/GLOSSARY.md": "# Ordering\n\n## Language\n\n**Order**:\nA request for goods.\n",
		"billing/GLOSSARY.md":  "# Billing\n\n## Language\n\n**Invoice**:\nA request for payment.\n",
	})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	require.Len(t, model.Relationships, 4)
	assert.Equal(t, []string{"ordering", "billing"}, model.Relationships[0].Contexts, "by the map's link text")
	assert.Equal(t, []string{"ordering/order", "billing/invoice"}, model.Relationships[0].TermIDs)
	assert.Equal(t, []string{"billing", "ordering"}, model.Relationships[1].Contexts, "by heading")
	assert.Equal(t, []string{"billing"}, model.Relationships[2].Contexts)
	assert.Equal(t, glossary.ContextRelationship{HTML: "Without a label."}, model.Relationships[3])
	assert.Equal(t, []glossary.Unresolved{
		{Phrase: "Refund", Path: "GLOSSARY-MAP.md", Context: "billing", Where: "relationship Billing ↔ Ordering"},
		{Phrase: "Shipping", Path: "GLOSSARY-MAP.md", Where: "relationship Shipping → Billing"},
	}, model.Unresolved, "a label name that matches no context is unresolved")
}
