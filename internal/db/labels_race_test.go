package db

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/baiirun/prog/internal/tasks"
)

// Protects AddLabelToItem's idempotency under concurrent agents: two items
// getting the same new label at once must both succeed and share one label.
func TestAddLabelToItem_ConcurrentFirstUse(t *testing.T) {
	ctx := context.Background()
	for round := range 20 {
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

// Protects transactions under concurrency: a transaction that reads before it
// writes (DeleteLabel) must wait for other writers rather than fail with
// SQLITE_BUSY_SNAPSHOT, and attaching a label must not fail because another
// agent deleted that label between creating and attaching it. Bursts stay
// small so the test measures these races, not lock starvation.
func TestLabels_ConcurrentDeleteAndAdd(t *testing.T) {
	ctx := context.Background()
	d := setupTestDB(t)
	item := createTestItem(t, d, "item")
	const bursts, pairs = 15, 8
	for burst := range bursts {
		var wg sync.WaitGroup
		errs := make(chan error, 2*pairs)
		for range pairs {
			wg.Add(2)
			go func() {
				defer wg.Done()
				if err := d.AddLabelToItem(ctx, item.ID, "test", "churn"); err != nil {
					errs <- fmt.Errorf("AddLabelToItem: %w", err)
				}
			}()
			go func() {
				defer wg.Done()
				var nf *tasks.NotFoundError
				if err := d.DeleteLabel(ctx, "test", "churn"); err != nil && !errors.As(err, &nf) {
					errs <- fmt.Errorf("DeleteLabel: %w", err)
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("burst %d: %v", burst, err)
		}
	}
}
