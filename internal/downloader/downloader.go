package downloader

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Client struct {
	HTTP *http.Client
}

func (c Client) Download(url string, file string) error {
	info, err := c.fetchText(url)
	if err != nil || strings.TrimSpace(info) == "" {
		info, err = c.fetchText(WithOutputText(url))
	}
	if err != nil {
		return err
	}

	parts := strings.Split(info, ",")
	if len(parts) < 2 {
		return fmt.Errorf("invalid plugin info")
	}
	pluginURL := strings.TrimSpace(parts[0])
	pluginMD5 := strings.TrimSpace(parts[1])
	if pluginURL == "" || pluginMD5 == "" {
		return fmt.Errorf("invalid plugin url or md5")
	}

	if err := c.downloadFile(pluginURL, file); err != nil {
		_ = os.Remove(file)
		return err
	}

	sum, err := FileMD5(file)
	if err != nil {
		_ = os.Remove(file)
		return err
	}
	if !strings.EqualFold(sum, pluginMD5) {
		_ = os.Remove(file)
		return fmt.Errorf("md5 mismatch: got %s, want %s", sum, pluginMD5)
	}
	return nil
}

func (c Client) fetchText(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GET %s returned %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c Client) downloadFile(url string, file string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s returned %s", url, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp := file + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, file)
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func FileMD5(file string) (string, error) {
	in, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer in.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, in); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func WithOutputText(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&output=text"
	}
	return rawURL + "?output=text"
}
