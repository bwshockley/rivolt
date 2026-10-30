package videos

import (
	"testing"
	"time"
)

func TestParseFilename(t *testing.T) {
	got, err := ParseFilename("10_30_26_142501_frontCenter_002.mp4", "America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	if got.Camera != "frontCenter" || got.Sequence != 2 {
		t.Fatalf("camera/sequence = %s/%d", got.Camera, got.Sequence)
	}
	if want := time.Date(2026, 10, 30, 14, 25, 1, 0, time.FixedZone("PDT", -7*60*60)); !got.RecordedAt.Equal(want) {
		t.Fatalf("recorded at = %s, want %s", got.RecordedAt, want)
	}
}

func TestParseFilenameRejectsInvalidInputs(t *testing.T) {
	for _, name := range []string{
		"10_30_26_142501_frontCenter_000.mp4",
		"10_30_26_142501_frontCenter_001.MP4",
		"10_30_26_256001_frontCenter_001.mp4",
		"../10_30_26_142501_frontCenter_001.mp4",
	} {
		if _, err := ParseFilename(name, "UTC"); err == nil {
			t.Errorf("ParseFilename(%q) unexpectedly succeeded", name)
		}
	}
	if _, err := ParseFilename("10_30_26_142501_frontCenter_001.mp4", "not/a/timezone"); err == nil {
		t.Fatal("invalid timezone unexpectedly succeeded")
	}
}