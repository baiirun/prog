package tasks_test

import "testing"

// Service tests run against the real SQLite store (no mocks). In step 2 the
// ready/filter/report tests from internal/db/queries_test.go and
// deps_test.go move here, rewritten against Service and Graph.

// Protects the epic write rule now that it lives in Service, not SQL.
func TestSetStatus_EpicOnlyAcceptsTerminal(t *testing.T) {
	t.Skip("pending: skeleton")
}

func TestSetParent_RejectsNonEpicParent(t *testing.T) {
	t.Skip("pending: skeleton")
}

// Protects backend-assigned IDs and project normalization at the service boundary.
func TestCreateItem_AssignsPrefixedIDAndNormalizesProject(t *testing.T) {
	t.Skip("pending: skeleton")
}
