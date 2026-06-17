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
	ErrInvalidVodID = errors.New("invalid vod id")
	vodIDRe         = regexp.MustCompile(`^\d{5,20}$`)
)

// VodSeekPad is subtracted from offset_seconds before ffmpeg -ss so the relay
// starts slightly before the analytics minute.
const VodSeekPad = 30

// VodRegistryKey is the orchestrator registry session key for a VOD relay.
// Convention: vod:{numericVodId} (separate from live channel sessions).
func VodRegistryKey(vodID string) string { return "vod:" + vodID }

// VodMediaKey is the MediaMTX RTMP/HLS path segment under /live/.
// Caddy proxies /live/* to MediaMTX; VOD relays publish to live/vod_{id}.
func VodMediaKey(vodID string) string { return "vod_" + vodID }

func ValidVodID(s string) bool { return vodIDRe.MatchString(strings.TrimSpace(s)) }

func VodPageURL(vodID string) string {
	return "https://www.twitch.tv/videos/" + vodID
}

// FormatVodTimeOffset formats seconds for Twitch ?t= and streamlink --hls-start-offset.
func FormatVodTimeOffset(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// VodPageURLWithOffset returns a Twitch VOD URL with ?t= seek for streamlink.
func VodPageURLWithOffset(vodID string, offsetSeconds int) string {
	seekSec := VodSeekSeconds(offsetSeconds)
	if seekSec <= 0 {
		return VodPageURL(vodID)
	}
	return VodPageURL(vodID) + "?t=" + FormatVodTimeOffset(seekSec)
}

func VodSeekSeconds(offsetSeconds int) int {
	seek := offsetSeconds - VodSeekPad
	if seek < 0 {
		return 0
	}
	return seek
}

func StartVod(vodID, quality string, offsetSeconds int, rtmpURL string, logw io.Writer, twitchOAuth string) (*Worker, error) {
	if !ValidVodID(vodID) {
		return nil, ErrInvalidVodID
	}
	if quality == "" {
		quality = "best"
	}
	seekSec := VodSeekSeconds(offsetSeconds)
	mediaKey := VodMediaKey(vodID)
	pageURL := VodPageURLWithOffset(vodID, offsetSeconds)

	slArgs := []string{
		"streamlink",
		"--twitch-disable-ads",
		"--twitch-supported-codecs=h264,h265,av1",
		"--retry-streams", "2",
		"--retry-max", "3",
		"--retry-open", "3",
		"--stream-segment-attempts", "3",
		"--stdout",
	}
	if seekSec > 0 {
		slArgs = append(slArgs, "--hls-start-offset", FormatVodTimeOffset(seekSec))
	}
	if strings.TrimSpace(twitchOAuth) != "" {
		slArgs = append(slArgs, "--twitch-oauth-token", strings.TrimSpace(twitchOAuth))
	}
	slArgs = append(slArgs, pageURL, quality)
	sl := exec.Command(slArgs[0], slArgs[1:]...)
	// Streamlink must seek via ?t= / --hls-start-offset; -ss before pipe:0 cannot skip
	// ahead in a live stdout stream and would decode from the VOD start (multi-minute delay).
	ffArgs := append(ffmpegPipeInputFlags(), "-i", "pipe:0", "-c", "copy", "-f", "flv", rtmpURL)
	ff := exec.Command("ffmpeg", ffArgs...)

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
		return nil, fmt.Errorf("start streamlink vod: %w", err)
	}
	ff.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: sl.Process.Pid}
	if err := ff.Start(); err != nil {
		pr.Close()
		pw.Close()
		_ = syscall.Kill(-sl.Process.Pid, syscall.SIGKILL)
		_ = sl.Wait()
		return nil, fmt.Errorf("start ffmpeg vod: %w", err)
	}
	pw.Close()
	pr.Close()
	return &Worker{Channel: mediaKey, PGID: sl.Process.Pid, sl: sl, ff: ff}, nil
}

func StartVodDirectHLS(vodID, sourceURL string, offsetSeconds int, rtmpURL string, logw io.Writer) (*Worker, error) {
	if !ValidVodID(vodID) {
		return nil, ErrInvalidVodID
	}
	u, err := url.Parse(sourceURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid hls source url")
	}
	seekSec := VodSeekSeconds(offsetSeconds)
	mediaKey := VodMediaKey(vodID)
	args := append([]string{"-hide_banner", "-loglevel", "error"}, ffmpegReconnectFlags()...)
	if seekSec > 0 {
		args = append(args, "-ss", strconv.Itoa(seekSec))
	}
	args = append(args, "-i", sourceURL, "-c", "copy", "-f", "flv", rtmpURL)
	ff := exec.Command("ffmpeg", args...)
	ff.Stderr = logw
	ff.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := ff.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg vod direct hls: %w", err)
	}
	return &Worker{Channel: mediaKey, PGID: ff.Process.Pid, ff: ff}, nil
}
