package dms

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/dms/dlna"
)

type safeFilePathTestCase struct {
	root, given, expected string
}

func TestSafeFilePath(t *testing.T) {
	var cases []safeFilePathTestCase
	if runtime.GOOS == "windows" {
		cases = []safeFilePathTestCase{
			{"c:", "/", "c:."},
			{"c:", "/test", "c:test"},
			{"c:\\", "/", "c:\\"},
			{"c:\\", "/test", "c:\\test"},
			{"c:\\hello", "../windows", "c:\\hello\\windows"},
			{"c:\\hello", "/../windows", "c:\\hello\\windows"},
			{"c:\\hello", "/", "c:\\hello"},
			{"c:\\hello", "./world", "c:\\hello\\world"},
			{"c:\\hello", "/", "c:\\hello"},
			// These two ones are invalid but, as this actually prevents to serve them, it is fine
			{"c:\\foo", "c:/windows/", "c:\\foo\\c:\\windows"},
			{"c:\\foo", "e:/", "c:\\foo\\e:"},
		}
	} else {
		cases = []safeFilePathTestCase{
			{"/", "..", "/"},
			{"/hello", "..//", "/hello"},
			{"", "/precious", "precious"},
			{".", "///precious", "precious"},
		}
	}
	t.Logf("running %d test cases", len(cases))
	for _, _case := range cases {
		a := safeFilePath(_case.root, _case.given)
		if a != _case.expected {
			t.Errorf("expected %q from %q and %q but got %q", _case.expected, _case.root, _case.given, a)
		}
	}
}

func TestRequest(t *testing.T) {
	resp, err := http.NewRequest("NOTIFY", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	buf := bytes.NewBuffer(nil)
	resp.Write(buf)
	t.Logf("%q", buf.String())
}

func TestResponse(t *testing.T) {
	var resp http.Response
	resp.StatusCode = http.StatusOK
	resp.Header = make(http.Header)
	resp.Header["SID"] = []string{"uuid:1337"}
	var buf bytes.Buffer
	resp.Write(&buf)
	t.Logf("%q", buf.String())
}

func TestParseDLNARangeHeaderOpenEnded(t *testing.T) {
	r, err := parseDLNARangeHeader("npt=00:01:23.000-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start != 83*time.Second {
		t.Fatalf("unexpected start: %v", r.Start)
	}
	if r.End != -1 {
		t.Fatalf("unexpected end: %v", r.End)
	}
}

func TestParseDLNARangeHeaderWithDurationSuffix(t *testing.T) {
	r, err := parseDLNARangeHeader("npt=00:01:23.000-00:02:00.000/00:10:00.000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start != 83*time.Second {
		t.Fatalf("unexpected start: %v", r.Start)
	}
	if r.End != 120*time.Second {
		t.Fatalf("unexpected end: %v", r.End)
	}
}

func TestParseByteRangeHeaderOpenEnded(t *testing.T) {
	r, err := parseByteRangeHeader("bytes=100-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start != 100 {
		t.Fatalf("unexpected start: %d", r.Start)
	}
	if r.HasEnd {
		t.Fatalf("unexpected end flag")
	}
}

func TestParseByteRangeHeaderBounded(t *testing.T) {
	r, err := parseByteRangeHeader("bytes=100-200")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Start != 100 {
		t.Fatalf("unexpected start: %d", r.Start)
	}
	if !r.HasEnd {
		t.Fatalf("expected end flag")
	}
	if r.End != 200 {
		t.Fatalf("unexpected end: %d", r.End)
	}
}

func TestParseByteRangeHeaderRejectsUnsupported(t *testing.T) {
	if _, err := parseByteRangeHeader("bytes=-200"); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := parseByteRangeHeader("bytes=0-10,20-30"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleHTTPByteRangeIgnoresNonBytesUnit(t *testing.T) {
	w := httptest.NewRecorder()
	h := make(http.Header)
	h.Set("Range", "npt=00:01:23.000-")
	_, partial, ok := handleHTTPByteRange(w, h, false)
	if !ok {
		t.Fatalf("expected ok")
	}
	if partial {
		t.Fatalf("unexpected partial response")
	}
}

func TestHandleDLNARangeFromHTTPRangeNPT(t *testing.T) {
	w := httptest.NewRecorder()
	h := make(http.Header)
	h.Set("Range", "npt=00:01:23.000-")
	r, partial, ok := handleDLNARange(w, h, false)
	if !ok {
		t.Fatalf("expected ok")
	}
	if !partial {
		t.Fatalf("expected partial response")
	}
	if r.Start != 83*time.Second {
		t.Fatalf("unexpected start: %v", r.Start)
	}
	if r.End != -1 {
		t.Fatalf("unexpected end: %v", r.End)
	}
	if got := w.Header().Get(dlna.TimeSeekRangeDomain); got != "npt=00:01:23.000-/*" {
		t.Fatalf("unexpected time seek response header: %q", got)
	}
}

func TestHLSSegmentURL(t *testing.T) {
	r := &http.Request{
		URL: &url.URL{
			Path: "/res",
			RawQuery: url.Values{
				"path":      {"/movie.mkv"},
				"transcode": {"mpeg4"},
			}.Encode(),
		},
	}
	got := hlsSegmentURL(r, 7)
	if !strings.Contains(got, "segment=7") {
		t.Fatalf("missing segment in url: %q", got)
	}
	if !strings.Contains(got, "transcode=mpeg4") {
		t.Fatalf("missing transcode in url: %q", got)
	}
}

func TestBuildHLSPlaylist(t *testing.T) {
	r := &http.Request{
		URL: &url.URL{
			Path: "/res",
			RawQuery: url.Values{
				"path":      {"/movie.mkv"},
				"transcode": {"mpeg4"},
			}.Encode(),
		},
	}
	playlist := buildHLSPlaylist(r, 2, time.Second)
	if !strings.Contains(playlist, "#EXTM3U") {
		t.Fatalf("missing EXTM3U header: %q", playlist)
	}
	if !strings.Contains(playlist, "#EXT-X-PLAYLIST-TYPE:VOD") {
		t.Fatalf("missing playlist type: %q", playlist)
	}
	if !strings.Contains(playlist, "#EXT-X-INDEPENDENT-SEGMENTS") {
		t.Fatalf("missing independent segments tag: %q", playlist)
	}
	if !strings.Contains(playlist, "#EXT-X-ENDLIST") {
		t.Fatalf("missing ENDLIST: %q", playlist)
	}
	if strings.Count(playlist, "#EXTINF:1.000,") != 2 {
		t.Fatalf("unexpected EXTINF count: %q", playlist)
	}
	if strings.Count(playlist, "#EXT-X-DISCONTINUITY") != 0 {
		t.Fatalf("unexpected discontinuity tag: %q", playlist)
	}
	if strings.Count(playlist, "#EXT-X-PROGRAM-DATE-TIME:") != 2 {
		t.Fatalf("unexpected program-date-time count: %q", playlist)
	}
	if !strings.Contains(playlist, "#EXT-X-PROGRAM-DATE-TIME:1970-01-01T00:00:00Z") {
		t.Fatalf("missing first program-date-time: %q", playlist)
	}
	if !strings.Contains(playlist, "#EXT-X-PROGRAM-DATE-TIME:1970-01-01T00:00:01Z") {
		t.Fatalf("missing second program-date-time: %q", playlist)
	}
	if !strings.Contains(playlist, "segment=0") || !strings.Contains(playlist, "segment=1") {
		t.Fatalf("missing segment urls: %q", playlist)
	}
}
