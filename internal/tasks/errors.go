package tasks

import "fmt"

// Record names the kind of record a NotFoundError refers to.
type Record string

const (
	RecordItem  Record = "item"
	RecordLabel Record = "label"
)

// NotFoundError reports that a record does not exist. It is caused by the
// caller's input, so retrying cannot help. Store implementations return it
// for every missing record so callers can tell it apart from I/O failures
// with errors.As instead of matching message text.
type NotFoundError struct {
	Record Record
	ID     string // item ID, or label name
}

func (e *NotFoundError) Error() string {
	if e.Record == RecordItem {
		return fmt.Sprintf("item not found: %s (use 'prog list' to see available items)", e.ID)
	}
	return fmt.Sprintf("%s not found: %s", e.Record, e.ID)
}
