package attach

import (
	"os"
	"testing"
)

func TestStageEmpty(t *testing.T) {
	got, err := Stage(nil, Limits{SizeMB: 200, MaxCount: 6}, 32<<20)
	if err != nil {
		t.Fatalf("Stage() = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Stage() = %d attachments, want 0", len(got))
	}
}

func TestStageDisabled(t *testing.T) {
	if _, err := Stage([]string{"a.bin"}, Limits{SizeMB: 0}, 32<<20); err == nil {
		t.Error("Stage(disabled) = nil, want error")
	}
}

func TestStageOverLimit(t *testing.T) {
	names := []string{"a", "b", "c"}
	if _, err := Stage(names, Limits{SizeMB: 200, MaxCount: 2}, 32<<20); err == nil {
		t.Error("Stage(over max) = nil, want error")
	}
}

func TestStageBadMemory(t *testing.T) {
	if _, err := Stage(nil, Limits{SizeMB: 200}, 0); err == nil {
		t.Error("Stage(memory 0) = nil, want error")
	}
}

func TestStageTempCleanup(t *testing.T) {
	a, err := StageTemp(t.TempDir(), "note.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("StageTemp() = %v", err)
	}
	if a.Path == "" || a.Name != "note.txt" {
		t.Errorf("StageTemp() = %+v, want path and name", a)
	}
	if _, err := os.Stat(a.Path); err != nil {
		t.Fatalf("Stat() = %v, want staged file", err)
	}
	a.Cleanup()
	if _, err := os.Stat(a.Path); !os.IsNotExist(err) {
		t.Errorf("Stat() after Cleanup = %v, want not-exist", err)
	}
	a.Cleanup() // idempotent
}

func TestStageTempBadName(t *testing.T) {
	if _, err := StageTemp(t.TempDir(), "", []byte("x")); err == nil {
		t.Error("StageTemp(blank) = nil, want error")
	}
	long := make([]byte, 251)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := StageTemp(t.TempDir(), string(long), []byte("x")); err == nil {
		t.Error("StageTemp(long) = nil, want error")
	}
}
