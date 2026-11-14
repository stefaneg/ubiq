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

// mapStatement is a Statement of the Context Map, without its HTML.
type mapStatement struct {
	Label    string
	Contexts []string
	Terms    []string
}

func mapStatements(model *glossary.Model) []mapStatement {
	var out []mapStatement
	for _, s := range model.Statements {
		if s.Path == "GLOSSARY-MAP.md" {
			out = append(out, mapStatement{s.Label, s.Contexts, s.TermIDs})
		}
	}
	return out
}

func TestLoadReadsEveryBlockOfTheMapAsAStatement(t *testing.T) {
	model := glossarytest.Load(t)
	assert.Equal(t, []mapStatement{
		{"Contexts", []string{"workspace"}, nil},
		{"Contexts", []string{"stacks"}, nil},
		{"Contexts", []string{"common"}, nil},
		{"Contexts", []string{"workspace"}, []string{"workspace/workspace"}},
		{"Relationships", []string{"workspace", "stacks"}, []string{"stacks/stack-image", "workspace/image"}},
		{"Relationships", []string{"stacks", "common"}, []string{"stacks/stack-image", "common/image-ref"}},
		{"Cross-context vocabulary › Deliberate mirrors › Run", []string{"workspace"}, []string{"workspace/workspace", "workspace/run"}},
	}, mapStatements(model), "a Context is named by its link text, in a bold label, or written capitalised; "+
		"phrases resolve in the first Context a Statement names")
	assert.Contains(t, model.Unresolved, glossary.Unresolved{Phrase: "Spectre", Path: "GLOSSARY-MAP.md", Context: "workspace",
		Where: "Cross-context vocabulary › Deliberate mirrors › Run"})
}

func TestStatementsNameContextsByHeadingOrMapLink(t *testing.T) {
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
- Ordering hands each **Order** to Billing.
- Without a label.
`,
		"ordering/GLOSSARY.md": "# Ordering\n\n## Language\n\n**Order**:\nA request for goods.\n",
		"billing/GLOSSARY.md":  "# Billing\n\n## Language\n\n**Invoice**:\nA request for payment.\n",
	})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	assert.Equal(t, []mapStatement{
		{"Contexts", []string{"ordering"}, nil},
		{"Contexts", []string{"billing"}, nil},
		{"Relationships", []string{"ordering", "billing"}, []string{"ordering/order", "billing/invoice"}},
		{"Relationships", []string{"billing", "ordering"}, []string{"billing/invoice"}},
		{"Relationships", []string{"billing"}, nil},
		{"Relationships", []string{"ordering", "billing"}, []string{"ordering/order"}},
	}, mapStatements(model), "by the map's link text or by heading, in bold or capitalised; a Statement that names "+
		"neither a Term nor a Context is dropped")
	assert.Contains(t, model.Statements[len(model.Statements)-4].HTML, "<b>Orders -&gt; Billing</b>")
	assert.Equal(t, []glossary.Unresolved{
		{Phrase: "Refund", Path: "GLOSSARY-MAP.md", Context: "billing", Where: "Relationships"},
		{Phrase: "Shipping", Path: "GLOSSARY-MAP.md", Where: "Relationships"},
	}, model.Unresolved, "a label name that matches no context is unresolved")
}

func TestLoadReadsTermEntriesUnderAnyHeading(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": `# Common

Shared vocabulary.

## Language

**Order**:
A request for goods.

## Image References

How images are named.

### Language

**Image Ref**:
A reference to an image by **Tag**.

**Tag**:
A mutable label.

### Relationships

- An **Image Ref** has one **Tag**.

## Scheduling

### Language

**Run**:
One invocation of an **Order**.

## Notes

**Note**:
A Term under a heading that names no Cluster form.
`})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	require.Len(t, model.Contexts, 1)
	var clusters [][]string
	for _, c := range model.Contexts[0].Clusters {
		clusters = append(clusters, append([]string{c.Name}, c.TermIDs...))
	}
	assert.Equal(t, [][]string{
		{"General", "common/order"},
		{"Image References", "common/image-ref", "common/tag"},
		{"Scheduling", "common/run"},
		{"Notes", "common/note"},
	}, clusters, "a Term's Cluster is the nearest heading above it, skipping one named Language")
	assert.Equal(t, 16, model.Terms["common/image-ref"].Line)
	assert.Equal(t, []string{"common/tag"}, model.Terms["common/image-ref"].Refs)
	assert.Equal(t, []string{"common/order"}, model.Terms["common/run"].Refs)
	assert.Equal(t, "<p>How images are named.</p>", model.Contexts[0].Clusters[1].Intro)
	assert.Equal(t, "<p>Shared vocabulary.</p>", model.Contexts[0].Intro)
}

func TestLoadReadsEveryOtherBlockOfAGlossaryAsAStatement(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": "# Common\n" + `
Shared vocabulary for **Order** and **Spectre** handling.

## Language

### Orders

**Order**:
A request for goods.

**Customer**:
A person who places an **Order**.

**Invoice**:
A bill.

### Relationships

- A **Customer** places many **Orders** and
  pays each **Invoice**.
- A **Customer** owns a **Ghost**.

> An **Order** is final.

| Term | Notes |
| --- | --- |
| **Invoice** | sent to a Customer |

` + "```text\n**Phantom** is code.\n```\n" + `
#### Invoice

- Sent once per **Order**.
- No Term here.
`})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	type statement struct {
		Label string
		Line  int
		Terms []string
	}
	var got []statement
	for _, s := range model.Statements {
		assert.Equal(t, "GLOSSARY.md", s.Path)
		got = append(got, statement{s.Label, s.Line, s.TermIDs})
	}
	assert.Equal(t, []statement{
		{"Common", 3, []string{"common/order"}},
		{"Language › Relationships", 20, []string{"common/customer", "common/order", "common/invoice"}},
		{"Language › Relationships", 22, []string{"common/customer"}},
		{"Language › Relationships", 24, []string{"common/order"}},
		{"Language › Relationships", 28, []string{"common/invoice", "common/customer"}},
		{"Language › Relationships › Invoice", 36, []string{"common/order", "common/invoice"}},
		{"Language › Relationships › Invoice", 37, []string{"common/invoice"}},
	}, got, "a bullet joins its continuation lines; the nearest heading's Terms join each Statement under it")
	assert.Contains(t, model.Statements[1].HTML, `data-id="common/invoice">Invoice</a>`)
	assert.Equal(t, []glossary.Unresolved{
		{Phrase: "Ghost", Path: "GLOSSARY.md", Context: "common", Where: "Language › Relationships"},
		{Phrase: "Spectre", Path: "GLOSSARY.md", Context: "common", Where: "Common"},
	}, model.Unresolved, "fenced code is skipped, and the intro's bold is reported once")
}

func TestLoadReportsUnresolvedBoldInAGlossaryWithoutTerms(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": "# Empty\n\nUses **Missing Term**.\n"})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	assert.Equal(t, []glossary.Unresolved{
		{Phrase: "Missing Term", Path: "GLOSSARY.md", Context: "empty", Where: "Empty"},
	}, model.Unresolved)
}

func TestLoadReportsUnresolvedBoldInEachClusterIntroOnce(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": `# Common

## Language

### Orders

About **Ghost**.

**Order**:
A request.

### Bills

About **Phantom**.

**Invoice**:
A bill.
`})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	assert.Equal(t, []glossary.Unresolved{
		{Phrase: "Ghost", Path: "GLOSSARY.md", Context: "common", Where: "Language › Orders"},
		{Phrase: "Phantom", Path: "GLOSSARY.md", Context: "common", Where: "Language › Bills"},
	}, model.Unresolved)
}

func TestLoadSkipsFencedCodeInsideAListItem(t *testing.T) {
	root := t.TempDir()
	glossarytest.WriteTree(t, root, map[string]string{"GLOSSARY.md": `# Common

## Language

**Order**:
A request.

**Invoice**:
A bill.

## Examples

- A bullet with an example:
  ~~~
  Order and Invoice and **Ghost**.
  ~~~
- Each **Order** is final.
`})
	model, err := glossary.Load(root)
	require.NoError(t, err)
	require.Len(t, model.Statements, 1)
	assert.Equal(t, 17, model.Statements[0].Line)
	assert.Equal(t, []string{"common/order"}, model.Statements[0].TermIDs)
	assert.Empty(t, model.Unresolved)
}
