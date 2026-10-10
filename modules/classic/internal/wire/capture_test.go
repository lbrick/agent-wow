package wire

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCapturedPackets parses packets saved by cmd/capture when
// AGENT_WOW_CAPTURE points at a capture directory. Captures are not committed.
func TestCapturedPackets(t *testing.T) {
	dir := os.Getenv("AGENT_WOW_CAPTURE")
	if dir == "" {
		t.Skip("set AGENT_WOW_CAPTURE to a cmd/capture output directory")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.bin"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no captures in %s: %v", dir, err)
	}
	var updates, blocks, moves, players int
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.Contains(f, "MONSTER_MOVE"):
			if _, err := ParseMonsterMove(body); err != nil {
				t.Errorf("%s: %v", filepath.Base(f), err)
			}
			moves++
		default:
			if strings.Contains(f, "COMPRESSED") {
				if body, err = Inflate(body); err != nil {
					t.Errorf("%s: %v", filepath.Base(f), err)
					continue
				}
			}
			bs, err := ParseUpdate(body)
			if err != nil {
				t.Errorf("%s: %v", filepath.Base(f), err)
			}
			updates++
			blocks += len(bs)
			for _, b := range bs {
				if b.TypeID == TypePlayer {
					players++
				}
			}
		}
	}
	t.Logf("%d updates (%d blocks, %d player creates), %d monster moves", updates, blocks, players, moves)
}
