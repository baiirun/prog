package db

import (
	"context"
	"sync"
	"testing"
)

// Protects AddLabelToItem's idempotency under concurrent agents: two items
// getting the same new label at once must both succeed and share one label.
func TestAddLabelToItem_ConcurrentFirstUse(t *testing.T) {
	ctx := context.Background()
	for round := range 100 {
		d := setupTestDB(t)
		const n = 8
		ids := make([]string, n)
		for i := range ids {
			ids[i] = createTestItem(t, d, "item").ID
		}
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := range ids {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = d.AddLabelToItem(ctx, ids[i], "test", "shared")
			}()
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: AddLabelToItem(%s): %v", round, ids[i], err)
			}
		}
		labels, err := d.ListLabels(ctx, "test")
		if err != nil {
			t.Fatal(err)
		}
		if len(labels) != 1 {
			t.Fatalf("round %d: %d labels, want 1 shared label", round, len(labels))
		}
	}
}
