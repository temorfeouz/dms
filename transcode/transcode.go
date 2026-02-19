// Package transcode implements routines for transcoding to various kinds of
// receiver.
package transcode

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/ffprobe"
	"github.com/anacrolix/log"

	. "github.com/anacrolix/dms/misc"
)

// Invokes an external command and returns a reader from its stdout. The
// command is waited on asynchronously.
func transcodePipe(args []string, stderr io.Writer) (r io.ReadCloser, err error) {
	log.Println("transcode command:", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	err = cmd.Start()
	if err != nil {
		return
	}
	waitCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		waitCh <- err
		if err != nil {
			log.Printf("command %s failed: %s", args, err)
		}
	}()
	r = &commandReadCloser{
		stdout:  stdout,
		cmd:     cmd,
		waitErr: waitCh,
	}
	return
}

type commandReadCloser struct {
	stdout  io.ReadCloser
	cmd     *exec.Cmd
	waitErr <-chan error
	once    sync.Once
}

func (me *commandReadCloser) Read(p []byte) (int, error) {
	return me.stdout.Read(p)
}

func (me *commandReadCloser) Close() (err error) {
	me.once.Do(func() {
		err = me.stdout.Close()
		if me.cmd.Process != nil {
			if killErr := me.cmd.Process.Kill(); killErr != nil && !errorsIsProcessDone(killErr) {
				err = killErr
			}
		}
		<-me.waitErr
	})
	return
}

func errorsIsProcessDone(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, os.ErrProcessDone)
}

// Return a series of ffmpeg arguments that pick specific codecs for specific
// streams. This requires use of the -map flag.
func streamArgs(s map[string]interface{}) (ret []string) {
	defer func() {
		if len(ret) != 0 {
			if _, ok := s["index"]; !ok {
				return
			}

			ret = append(ret, []string{
				"-map", "0:" + fmt.Sprintf("%v", s["index"]),
			}...)
		}
	}()
	switch s["codec_type"] {
	case "video":
		/*
			if s["codec_name"] == "h264" {
				if i, _ := strconv.ParseInt(s["is_avc"], 0, 0); i != 0 {
					return []string{"-vcodec", "copy", "-sameq", "-vbsf", "h264_mp4toannexb"}
				}
			}
		*/
		return []string{"-target", "pal-dvd"}
	case "audio":
		if s["codec_name"] == "dca" {
			return []string{"-acodec", "ac3", "-ab", "224k", "-ac", "2"}
		} else {
			return []string{"-acodec", "copy"}
		}
	case "subtitle":
		return []string{"-scodec", "copy"}
	}
	return
}
func ffmpegExecutable() string {
	if tmp := os.Getenv("FFMPEG_PATH"); tmp != "" {
		log.Println("use custom ffmpeg", tmp)

		return tmp
	}

	return "ffmpeg"
}

func quality() string {
	if tmp := os.Getenv("FFMPEG_QUALITY"); tmp != "" {
		log.Println("use quality", tmp)

		return tmp
	}

	return "4"
}

// Streams the desired file in the MPEG_PS_PAL DLNA profile.
func Transcode(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	args := []string{
		ffmpegExecutable(),
		"-threads", strconv.FormatInt(int64(runtime.NumCPU()), 10),
		"-async", "1",
		"-ss", FormatDurationSexagesimal(start),
	}
	if length >= 0 {
		args = append(args, []string{
			"-t", FormatDurationSexagesimal(length),
		}...)
	}
	args = append(args, []string{
		"-i", path,
	}...)
	info, err := ffprobe.Run(path)
	if err != nil {
		return
	}
	for _, s := range info.Streams {
		args = append(args, streamArgs(s)...)
	}
	args = append(args, []string{"-f", "mpegts", "pipe:"}...)
	return transcodePipe(args, stderr)
}

// Returns a stream of Chromecast supported VP8.
func VP8Transcode(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	args := []string{
		"avconv",
		"-threads", strconv.FormatInt(int64(runtime.NumCPU()), 10),
		"-async", "1",
		"-ss", FormatDurationSexagesimal(start),
	}
	if length > 0 {
		args = append(args, []string{
			"-t", FormatDurationSexagesimal(length),
		}...)
	}
	args = append(args, []string{
		"-i", path,
		// "-deadline", "good",
		// "-c:v", "libvpx", "-crf", "10",
		"-f", "webm",
		"pipe:",
	}...)
	return transcodePipe(args, stderr)
}

// Returns a stream of Chromecast supported matroska.
func ChromecastTranscode(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	args := []string{
		ffmpegExecutable(),
		"-ss", FormatDurationSexagesimal(start),
		"-i", path,
		"-c:v", "libx264", "-preset", "ultrafast", "-profile:v", "high", "-level", "5.0",
		"-movflags", "+faststart+frag_keyframe+empty_moov",
	}
	if length > 0 {
		args = append(args, []string{
			"-t", FormatDurationSexagesimal(length),
		}...)
	}
	args = append(args, []string{
		"-f", "mp4",
		"pipe:",
	}...)
	return transcodePipe(args, stderr)
}

// Returns a stream of mpeg4 video for Panasonic TV
func MPEG4Transcode(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	args := []string{
		ffmpegExecutable(),
		"-ss", FormatDurationSexagesimal(start),
		"-i", path,
		"-vcodec", "mpeg4",
		"-threads", "2",
		"-q:v", quality(),
		"-movflags", "+faststart+frag_keyframe+empty_moov",
	}
	if length > 0 {
		args = append(args, []string{
			"-t", FormatDurationSexagesimal(length),
		}...)
	}
	args = append(args, []string{
		"-f", "mp4",
		"pipe:",
	}...)
	return transcodePipe(args, stderr)
}

// Returns a stream of h264 video and mp3 audio
func WebTranscode(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	args := []string{
		ffmpegExecutable(),
		"-ss", FormatDurationSexagesimal(start),
		"-i", path,
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264", "-crf", "25",
		"-c:a", "mp3", "-ab", "128k", "-ar", "44100",
		"-preset", "ultrafast",
		"-movflags", "+faststart+frag_keyframe+empty_moov",
	}
	if length > 0 {
		args = append(args, []string{
			"-t", FormatDurationSexagesimal(length),
		}...)
	}
	args = append(args, []string{
		"-f", "mp4",
		"pipe:",
	}...)
	return transcodePipe(args, stderr)
}

// Returns a stable MPEG-TS segment suitable for HLS-style segmented playback.
func SegmentTranscode(path string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	coarseSeek := start
	fineSeek := time.Duration(0)
	const fineSeekWindow = 2 * time.Second
	if start > fineSeekWindow {
		coarseSeek = start - fineSeekWindow
		fineSeek = fineSeekWindow
	} else {
		coarseSeek = 0
		fineSeek = start
	}

	args := []string{
		ffmpegExecutable(),
		"-threads", strconv.FormatInt(int64(runtime.NumCPU()), 10),
		"-ss", FormatDurationSexagesimal(coarseSeek),
		"-i", path,
	}
	if fineSeek > 0 {
		args = append(args, []string{
			"-ss", FormatDurationSexagesimal(fineSeek),
		}...)
	}
	if length > 0 {
		args = append(args, []string{
			"-t", FormatDurationSexagesimal(length),
		}...)
	}
	args = append(args, []string{
		"-c:v", "mpeg2video",
		"-q:v", "5",
		"-bf", "0",
		"-pix_fmt", "yuv420p",
		"-c:a", "mp2",
		"-b:a", "192k",
		"-ac", "2",
		"-muxpreload", "0",
		"-muxdelay", "0",
		"-output_ts_offset", FormatDurationSexagesimal(start),
		"-mpegts_flags", "+resend_headers",
		"-f", "mpegts",
		"pipe:",
	}...)
	return transcodePipe(args, stderr)
}

// credit laurent @ https://stackoverflow.com/questions/34118732/parse-a-command-line-string-into-flags-and-arguments-in-golang
func parseCommandLine(command string) ([]string, error) {
	var args []string
	state := "start"
	current := ""
	quote := "\""
	escapeNext := true
	for i := 0; i < len(command); i++ {
		c := command[i]

		if state == "quotes" {
			if string(c) != quote {
				current += string(c)
			} else {
				args = append(args, current)
				current = ""
				state = "start"
			}
			continue
		}

		if escapeNext {
			current += string(c)
			escapeNext = false
			continue
		}

		if c == '\\' {
			escapeNext = true
			continue
		}

		if c == '"' || c == '\'' {
			state = "quotes"
			quote = string(c)
			continue
		}

		if state == "arg" {
			if c == ' ' || c == '\t' {
				args = append(args, current)
				current = ""
				state = "start"
			} else {
				current += string(c)
			}
			continue
		}

		if c != ' ' && c != '\t' {
			state = "arg"
			current += string(c)
		}
	}

	if state == "quotes" {
		return []string{}, fmt.Errorf("Unclosed quote in command line: %s", command)
	}

	if current != "" {
		args = append(args, current)
	}

	return args, nil
}

// Exec runs the cmd to generate the video to stream. It does not support seeking. Used by the dynamic stream feature.
func Exec(cmds string, start, length time.Duration, stderr io.Writer) (r io.ReadCloser, err error) {
	cmda, aerr := parseCommandLine(cmds)
	if aerr != nil {
		err = aerr
		return
	}
	return transcodePipe(cmda, stderr)
}
