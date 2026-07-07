package assets

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Scale struct {
	Name   string
	Height int
}

var Scales = []Scale{
	{"1x", 32},
	{"2x", 64},
	{"3x", 96},
	{"4x", 128},
}

type Rendition struct {
	Scale string
	Data  []byte
}

func Render(srcPath string) ([]Rendition, error) {
	names := make([]string, 0, len(Scales))
	for _, sc := range Scales {
		names = append(names, sc.Name)
	}
	return RenderScales(srcPath, names)
}

func RenderScales(srcPath string, scaleNames []string) ([]Rendition, error) {
	dir, err := os.MkdirTemp("", "emote-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	var results []Rendition
	for _, name := range scaleNames {
		sc, ok := scaleByName(name)
		if !ok {
			return nil, fmt.Errorf("unsupported scale %s", name)
		}
		out := filepath.Join(dir, sc.Name+".webp")
		err := runVips(srcPath, out, sc.Height)
		if err != nil {
			return nil, fmt.Errorf("scale %s: %w", sc.Name, err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", sc.Name, err)
		}
		results = append(results, Rendition{Scale: sc.Name, Data: data})
	}
	return results, nil
}

func scaleByName(name string) (Scale, bool) {
	for _, sc := range Scales {
		if sc.Name == name {
			return sc, true
		}
	}
	return Scale{}, false
}

func runVips(src, dst string, height int) error {
	cmd := exec.Command("vips",
		"thumbnail",
		vipsSource(src),
		dst,
		"10000",
		"--height", fmt.Sprintf("%d", height),
		"--size", "down",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}

func vipsSource(src string) string {
	data, err := os.ReadFile(src)
	if err != nil {
		return src
	}
	contentType := http.DetectContentType(data)
	if strings.Contains(contentType, "webp") || strings.Contains(contentType, "gif") {
		return src + "[n=-1]"
	}
	return src
}
