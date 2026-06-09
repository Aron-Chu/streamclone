package worker

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestValidChannel(t *testing.T) {
	cases := map[string]bool{
		"ninja":            true,
		"a_b_c":            true,
		"user123":          true,
		"ab":               false,
		"_leading":         false,
		"UPPER":            false,
		"has-dash":         false,
		"with space":       false,
		"twitch.tv/inject": false,
		"":                 false,
	}
	for in, want := range cases {
		if got := ValidChannel(in); got != want {
			t.Errorf("ValidChannel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestProcessGroupKill(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Skipf("sleep unavailable: %v", err)
	}
	pgid := cmd.Process.Pid
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill process group: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected process to be killed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process not killed within timeout")
	}
}
