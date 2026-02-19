package transcode

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTranscodePipeCloseStopsProcess(t *testing.T) {
	r, err := transcodePipe([]string{"sh", "-c", "while :; do printf x; done"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("close took too long: %s", time.Since(start))
	}
}

func TestSegmentTranscodeUsesTwoStageSeek(t *testing.T) {
	tmpDir := t.TempDir()
	ffmpegPath := filepath.Join(tmpDir, "fake-ffmpeg.sh")
	argsPath := filepath.Join(tmpDir, "args.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsPath + "\"\nprintf x\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FFMPEG_PATH", ffmpegPath)

	r, err := SegmentTranscode("/tmp/video.mkv", 10*time.Second, time.Second, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	argsRaw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(argsRaw)), "\n")

	ssIndexes := make([]int, 0, 2)
	iIndex := -1
	for i, arg := range args {
		if arg == "-ss" {
			ssIndexes = append(ssIndexes, i)
		}
		if arg == "-i" && iIndex == -1 {
			iIndex = i
		}
	}
	if len(ssIndexes) != 2 || iIndex == -1 {
		t.Fatalf("missing -ss or -i in args: %q", args)
	}
	if ssIndexes[0] > iIndex {
		t.Fatalf("expected input seek before input file, args: %q", args)
	}
	if ssIndexes[1] < iIndex {
		t.Fatalf("expected fine seek after input file, args: %q", args)
	}
	if !strings.Contains(string(argsRaw), "-f\nmpegts\n") {
		t.Fatalf("expected mpegts output, args: %q", args)
	}
	if strings.Contains(string(argsRaw), "-avoid_negative_ts\nmake_zero\n") {
		t.Fatalf("unexpected avoid_negative_ts make_zero in args: %q", args)
	}
}
