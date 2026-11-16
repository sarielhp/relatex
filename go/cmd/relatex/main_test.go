package main

import "testing"

func TestProgramNameDropsTheExeSuffix(t *testing.T) {
	for argument, want := range map[string]string{"/usr/bin/rlx": "rlx", "rlx.exe": "rlx", "RLX.EXE": "rlx", "relatex": "relatex", "a.b": "a.b"} {
		if got := programName(argument); got != want {
			t.Errorf("programName(%q) = %q, want %q", argument, got, want)
		}
	}
}
