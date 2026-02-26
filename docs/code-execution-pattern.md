# Code Execution Pattern

## Overview

The code execution pattern reduces token consumption by 80-99% by executing data transformations and tool interactions in a isolated subprocess instead of passing all data through the model's context window.

Reference: [Code execution with MCP: Building more efficient agents](https://www.anthropic.com/engineering/code-execution-with-mcp)

## Progressive Discovery Workflow

Instead of loading all tool definitions upfront, discover tools on-demand:

1. **List available tools**
   ```bash
   9p ls agent/tools/anvilmcp/
   ```

2. **Read specific tool definitions**
   ```bash
   9p read agent/tools/anvilmcp/beads.sh
   ```

3. **Import and use in bash**
   ```bash
   # Call tools from agent/tools/anvilmcp/beads';
   
   const allBeads = await beads.list();
   const open = allBeads.filter(b => b.status === 'open');
   console.log(`Found ${open.length} open beads`);
   ```

## Data Transformation Examples

### Filtering Large Datasets

Instead of loading 10,000 rows into context:

```bash
// Load data in subprocess
const allRows = await beads.list();

// Filter in subprocess
const pending = allRows.filter(row => row.status === 'open' && row.priority <= 2);

// Only return summary to context
console.log(`Found ${pending.length} high-priority open beads`);
console.log(pending.slice(0, 5)); // Show first 5 for review
```

### Polling Loops

Execute loops in subprocess instead of alternating tool calls:

```bash
let found = false;
while (!found) {
  const beads = await beads.list();
  found = beads.some(b => b.id === 'bd-abc' && b.status === 'closed');
  if (!found) await new Promise(r => setTimeout(r, 5000));
}
console.log('Bead bd-abc completed');
```

### Aggregations

Compute statistics without context bloat:

```bash
const allBeads = await beads.list();

const stats = {
  total: allBeads.length,
  byStatus: {},
  byPriority: {}
};

for (const bead of allBeads) {
  stats.byStatus[bead.status] = (stats.byStatus[bead.status] || 0) + 1;
  stats.byPriority[bead.priority] = (stats.byPriority[bead.priority] || 0) + 1;
}

console.log(JSON.stringify(stats, null, 2));
```

## Token Savings

- **Without code execution**: Load all tool definitions (150k tokens) + all intermediate results
- **With code execution**: Load only needed tools (~2k tokens) + filtered results
- **Savings**: 80-99% reduction in token consumption

## When to Use

Use code execution when:
- Working with large datasets (>100 rows)
- Performing multiple transformations on the same data
- Implementing loops or polling
- Computing aggregations or statistics
- Filtering data before presenting to user

Use direct tool calls when:
- Single, simple operations
- Small result sets
- No data transformation needed

## Extended Examples

### Multi-Source Data Joins

Join data from multiple sources without loading everything into context:

```bash
# Call tools from agent/tools/anvilmcp/beads';
# Call tools from agent/tools/anvilmcp/kb';

// Load data from both sources
const allBeads = await beads.list();
const kbEntries = await kb.search('implementation');

// Join in subprocess
const enriched = allBeads
  .filter(b => b.status === 'open')
  .map(bead => {
    const relatedKB = kbEntries.filter(kb => 
      kb.content.includes(bead.id) || 
      bead.description.includes(kb.title)
    );
    return { ...bead, kbReferences: relatedKB.length };
  })
  .filter(b => b.kbReferences > 0);

console.log(`Found ${enriched.length} beads with KB references`);
console.log(enriched.slice(0, 3));
```

### Batch Operations with Progress Tracking

Process many items without flooding context:

```bash
const beads = await beads.query({ status: 'open', priority: 1 });

let processed = 0;
let errors = [];

for (const bead of beads) {
  try {
    await beads.update(bead.id, { priority: 2 });
    processed++;
  } catch (e) {
    errors.push({ id: bead.id, error: e.message });
  }
  
  if (processed % 10 === 0) {
    console.log(`Processed ${processed}/${beads.length}`);
  }
}

console.log(`Complete: ${processed} updated, ${errors.length} errors`);
if (errors.length > 0) {
  console.log('Errors:', errors.slice(0, 5));
}
```

### Complex Filtering with Multiple Criteria

Apply complex business logic in subprocess:

```bash
const allBeads = await beads.list();

const candidates = allBeads.filter(bead => {
  // Multiple conditions
  const isHighPriority = bead.priority <= 2;
  const isStale = new Date() - new Date(bead.updated_at) > 7 * 24 * 60 * 60 * 1000;
  const hasNoAssignee = !bead.assignee;
  const isNotBlocked = !bead.blockers || bead.blockers.length === 0;
  
  return isHighPriority && isStale && hasNoAssignee && isNotBlocked;
});

// Group by priority
const grouped = candidates.reduce((acc, bead) => {
  acc[bead.priority] = acc[bead.priority] || [];
  acc[bead.priority].push(bead);
  return acc;
}, {});

console.log('Stale unassigned beads by priority:');
for (const [priority, beads] of Object.entries(grouped)) {
  console.log(`  Priority ${priority}: ${beads.length} beads`);
}
```
