package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMock_Normal(t *testing.T) {
	m := Normal([]byte(`{"job_id":"J-1","status":"completed"}`), 1234)
	r, err := m.Run(context.Background(), Request{JobID: "J-1"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 || !r.FinalJSONOK || r.ReportedTokens == nil || *r.ReportedTokens != 1234 {
		t.Fatalf("unexpected result: %+v", r)
	}
	if len(m.Calls) != 1 || m.Calls[0].JobID != "J-1" {
		t.Fatalf("call not recorded: %+v", m.Calls)
	}
}

func TestMock_NonZeroExit(t *testing.T) {
	r, _ := NonZeroExit(40).Run(context.Background(), Request{})
	if r.ExitCode != 40 {
		t.Fatalf("exit code = %d want 40", r.ExitCode)
	}
}

func TestMock_TornFinalJSON(t *testing.T) {
	r, _ := TornFinalJSON().Run(context.Background(), Request{})
	if r.FinalJSONOK {
		t.Fatal("torn final.json must report FinalJSONOK=false")
	}
}

func TestMock_NoUsage(t *testing.T) {
	r, _ := NoUsage([]byte(`{}`)).Run(context.Background(), Request{})
	if r.ReportedTokens != nil {
		t.Fatal("NoUsage must leave ReportedTokens nil")
	}
}

func TestMock_Zombie_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	r, _ := Zombie().Run(ctx, Request{})
	if !r.KilledByWatchdog {
		t.Fatal("zombie should report KilledByWatchdog after ctx cancel")
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Fatal("zombie returned before context deadline")
	}
}

func TestMock_ScopeViolation_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	m := ScopeViolation([]byte(`{}`), map[string]string{"package.json": "{}", "infra/x.sh": "rm"})
	if _, err := m.Run(context.Background(), Request{Workdir: dir}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"package.json", "infra/x.sh"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected %s written: %v", rel, err)
		}
	}
}

// Compile-time check that Mock satisfies Runtime.
var _ Runtime = (*Mock)(nil)
