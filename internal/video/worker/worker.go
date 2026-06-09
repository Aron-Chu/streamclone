package worker

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

var (
	ErrInvalidChannel = errors.New("invalid channel name")
	channelRe         = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,24}$`)
)

func ValidChannel(s string) bool { return channelRe.MatchString(s) }

type Worker struct {
	Channel string
	PGID    int
	sl      *exec.Cmd
	ff      *exec.Cmd
}

func Start(channel, quality, rtmpURL string, logw io.Writer) (*Worker, error) {
	if !ValidChannel(channel) {
		return nil, ErrInvalidChannel
	}
	if quality == "" {
		quality = "best"
	}
	liveEdge := strings.TrimSpace(os.Getenv("STREAMLINK_HLS_LIVE_EDGE"))
	if liveEdge == "" {
		liveEdge = "2"
	}
	sl := exec.Command(
		"streamlink",
		"--twitch-disable-ads",
		"--twitch-supported-codecs=h264,h265,av1",
		"--twitch-low-latency",
		"--hls-live-edge", liveEdge,
		"--retry-streams", "2",
		"--retry-max", "3",
		"--retry-open", "3",
		"--stream-segment-attempts", "3",
		"--stdout",
		"twitch.tv/"+channel,
		quality,
	)
	ff := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-c", "copy", "-f", "flv", rtmpURL)

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	sl.Stdout = pw
	sl.Stderr = logw
	sl.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	ff.Stdin = pr
	ff.Stderr = logw

	if err := sl.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("start streamlink: %w", err)
	}
	ff.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: sl.Process.Pid}
	if err := ff.Start(); err != nil {
		pr.Close()
		pw.Close()
		_ = syscall.Kill(-sl.Process.Pid, syscall.SIGKILL)
		_ = sl.Wait()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	pw.Close()
	pr.Close()
	return &Worker{Channel: channel, PGID: sl.Process.Pid, sl: sl, ff: ff}, nil
}

func (w *Worker) Wait() error {
	var err error
	if w.ff != nil {
		err = w.ff.Wait()
	}
	w.Kill()
	if w.sl != nil {
		_ = w.sl.Wait()
	}
	return err
}

func (w *Worker) Kill() {
	if w.PGID > 0 {
		_ = syscall.Kill(-w.PGID, syscall.SIGKILL)
	}
}

func StartDirectHLS(channel, sourceURL, rtmpURL string, logw io.Writer) (*Worker, error) {
	if !ValidChannel(channel) {
		return nil, ErrInvalidChannel
	}
	u, err := url.Parse(sourceURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid hls source url")
	}
	ff := exec.Command(
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-reconnect", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "2",
		"-i", sourceURL,
		"-c", "copy",
		"-f", "flv",
		rtmpURL,
	)
	ff.Stderr = logw
	ff.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := ff.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg direct hls: %w", err)
	}
	return &Worker{Channel: channel, PGID: ff.Process.Pid, ff: ff}, nil
}

func Reconcile(match string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	killed := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(data), "\x00", " ")
		if strings.Contains(cmd, "streamlink") || (match != "" && strings.Contains(cmd, match)) {
			if syscall.Kill(-pid, syscall.SIGKILL) == nil {
				killed++
			}
		}
	}
	return killed
}
