package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/baiirun/prog/internal/db"
	"github.com/baiirun/prog/internal/model"
	"github.com/baiirun/prog/internal/tasks"
)

// progEnv runs the real cobra command tree against an isolated database.
type progEnv struct {
	t      *testing.T
	dbPath string
}

// newProg points PROG_DB and HOME at a fresh temp dir (so backups written by
// mutating commands never reach the real ~/.prog) and runs `prog init`.
func newProg(t *testing.T) *progEnv {
	t.Helper()
	dir := t.TempDir()
	p := &progEnv{t: t, dbPath: filepath.Join(dir, "prog.db")}
	t.Setenv("PROG_DB", p.dbPath)
	t.Setenv("HOME", dir)
	p.ok("init")
	return p
}

// run executes `prog args...` and returns what it wrote to stdout.
func (p *progEnv) run(args ...string) (string, error) {
	p.t.Helper()
	resetFlags(p.t, rootCmd)
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	rootCmd.SetArgs(args)

	var err error
	out := captureStdout(p.t, func() { err = rootCmd.Execute() })
	return out, err
}

// ok runs a command that must succeed and returns its trimmed stdout.
func (p *progEnv) ok(args ...string) string {
	p.t.Helper()
	out, err := p.run(args...)
	if err != nil {
		p.t.Fatalf("prog %s: %v\nstdout: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(out)
}

// fails runs a command that must return an error.
func (p *progEnv) fails(args ...string) {
	p.t.Helper()
	if out, err := p.run(args...); err == nil {
		p.t.Fatalf("prog %s succeeded, want error\nstdout: %s", strings.Join(args, " "), out)
	}
}

// show returns `prog show <id> --json` decoded into generic JSON values, so
// assertions check the wire format rather than the production structs.
func (p *progEnv) show(id string) map[string]any {
	p.t.Helper()
	var obj map[string]any
	decodeJSON(p.t, p.ok("show", id, "--json"), &obj)
	return obj
}

// jsonArray runs a command that prints a JSON array of objects.
func (p *progEnv) jsonArray(args ...string) []map[string]any {
	p.t.Helper()
	out := p.ok(args...)
	var arr []map[string]any
	decodeJSON(p.t, out, &arr)
	if arr == nil {
		p.t.Fatalf("prog %s printed %q, want a JSON array (not null)", strings.Join(args, " "), out)
	}
	return arr
}

// setStatus writes a stored status no CLI command can reach (e.g. blocked).
func (p *progEnv) setStatus(id string, status model.Status) {
	p.t.Helper()
	database, err := db.Open(p.dbPath)
	if err != nil {
		p.t.Fatalf("open db: %v", err)
	}
	defer func() { _ = database.Close() }()
	if err := tasks.NewService(database).SetStatus(context.Background(), id, status); err != nil {
		p.t.Fatalf("set status %s=%s: %v", id, status, err)
	}
}

func decodeJSON(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, s)
	}
}

// ids returns the "id" field of each object, in order.
func ids(arr []map[string]any) []string {
	out := make([]string, 0, len(arr))
	for _, o := range arr {
		id, _ := o["id"].(string)
		out = append(out, id)
	}
	return out
}

// assertUTCTimestamp checks field is present and in the jq-compatible form
// "2006-01-02T15:04:05Z".
func assertUTCTimestamp(t *testing.T, obj map[string]any, field string) {
	t.Helper()
	s, ok := obj[field].(string)
	if !ok {
		t.Errorf("%s = %#v, want RFC3339 string", field, obj[field])
		return
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil || !strings.HasSuffix(s, "Z") || strings.Contains(s, ".") {
		t.Errorf("%s = %q, want UTC RFC3339 with whole seconds", field, s)
		return
	}
	if parsed.IsZero() {
		t.Errorf("%s is the zero time", field)
	}
}

// assertEmptyArray checks field is [] (not null, not missing).
func assertEmptyArray(t *testing.T, obj map[string]any, field string) {
	t.Helper()
	arr, ok := obj[field].([]any)
	if !ok || len(arr) != 0 {
		t.Errorf("%s = %#v, want []", field, obj[field])
	}
}

// resetFlags returns every flag in the command tree to its default. Flags are
// package globals that persist across Execute calls.
func resetFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	reset := func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			var def []string
			if d := strings.Trim(f.DefValue, "[]"); d != "" {
				def = strings.Split(d, ",")
			}
			if err := sv.Replace(def); err != nil {
				t.Fatalf("reset --%s: %v", f.Name, err)
			}
		} else if err := f.Value.Set(f.DefValue); err != nil {
			t.Fatalf("reset --%s: %v", f.Name, err)
		}
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		resetFlags(t, sub)
	}
}

// captureStdout redirects os.Stdout while f runs. The pipe is drained
// concurrently so large output cannot deadlock the writer.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	f()
	_ = w.Close()
	os.Stdout = old
	return <-done
}
