package verify

import "testing"

func TestRun_AllPass(t *testing.T) {
	v, ok := Run(t.TempDir(), "sh", "task", []string{"true", "true"})
	if !ok || len(v.Checks) != 2 || !v.Passed() {
		t.Fatalf("expected all pass: ok=%v checks=%d passed=%v", ok, len(v.Checks), v.Passed())
	}
}

func TestRun_OneFails(t *testing.T) {
	v, ok := Run(t.TempDir(), "sh", "task", []string{"true", "false"})
	if ok || v.Passed() {
		t.Fatalf("expected failure: ok=%v passed=%v", ok, v.Passed())
	}
	if v.Checks[1].Result != "failed" || *v.Checks[1].ExitCode == 0 {
		t.Fatalf("failed check not recorded: %+v", v.Checks[1])
	}
}

func TestRun_EmptyNotVacuouslyTrue(t *testing.T) {
	v, ok := Run(t.TempDir(), "", "task", nil)
	if ok || v.Passed() {
		t.Fatal("empty requirement set must not pass (N23)")
	}
}
