package netease

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

func readLocalArtwork(raw string) ([]byte, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return nil, fmt.Errorf("不支持的本地封面地址: %s", parsed.Host)
	}
	return os.ReadFile(filepath.FromSlash(parsed.Path))
}
