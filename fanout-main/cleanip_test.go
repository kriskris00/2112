package main

import "testing"

func TestCleanIPManual(t *testing.T) {
	scores, err := parseCleanIPManualScores("1.1.1.1 72\n8.8.8.8=90\n# comment")
	if err != nil {
		t.Fatal(err)
	}
	s, err := checkCleanIPScore("8.8.8.8", scores)
	if err != nil || s != 90 {
		t.Fatalf("%d %v", s, err)
	}
	if _, err = checkCleanIPScore("9.9.9.9", scores); err == nil {
		t.Fatal("unknown allowed")
	}
	for _, bad := range []string{"8.8.8.8 101", "not-ip 80", "1.1.1.1", "1.1.1.1 abc"} {
		if _, err := parseCleanIPManualScores(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
