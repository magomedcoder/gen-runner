package huggingface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ModelInfo struct {
	ModelID  string `json:"modelId"`
	Private  bool   `json:"private"`
	Gated    bool   `json:"gated"`
	Siblings []File `json:"siblings"`
}

type File struct {
	Rfilename string `json:"rfilename"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

func NewClient(token string) *Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DisableCompression:    true,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		baseURL: "https://huggingface.co",
		httpClient: &http.Client{
			Transport: tr,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("слишком много редиректов")
				}

				return nil
			},
		},
		token: strings.TrimSpace(token),
	}
}

func (c *Client) ModelInfo(ctx context.Context, repoID string) (*ModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	url := fmt.Sprintf("%s/api/models/%s", c.baseURL, repoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("ответ api со статусом %d: %s", resp.StatusCode, string(body))
	}

	var info ModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("разбор ответа: %w", err)
	}

	return &info, nil
}

func (info *ModelInfo) GGFFFilenames() []string {
	var out []string
	for _, s := range info.Siblings {
		if strings.ToLower(path.Ext(s.Rfilename)) == ".gguf" {
			out = append(out, s.Rfilename)
		}
	}

	return out
}

func (c *Client) Download(ctx context.Context, repoID, revision, filename string, w io.Writer, onProgress ProgressFunc) (written int64, contentLength int64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if revision == "" {
		revision = "main"
	}

	url := fmt.Sprintf("%s/%s/resolve/%s/%s", c.baseURL, repoID, revision, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

		return 0, 0, fmt.Errorf("загрузка вернула %d: %s", resp.StatusCode, string(body))
	}

	total := resp.ContentLength
	var body io.Reader = resp.Body
	if onProgress != nil {
		body = &progressReader{
			r:     resp.Body,
			base:  0,
			total: total,
			on:    onProgress,
		}
	}

	n, err := io.Copy(w, body)
	if err != nil {
		return n, total, err
	}

	return n, total, nil
}

func parseContentRange(s string) (start, end, total int64, ok bool) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	const pfx = "bytes "
	if !strings.HasPrefix(low, pfx) {
		return 0, 0, 0, false
	}

	s = s[len(pfx):]
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0, 0, false
	}

	rangePart := strings.TrimSpace(parts[0])
	totalPart := strings.TrimSpace(parts[1])

	if rangePart == "*" {
		if totalPart == "*" {
			return 0, 0, -1, true
		}

		t, err := strconv.ParseInt(totalPart, 10, 64)
		if err != nil || t < 0 {
			return 0, 0, 0, false
		}

		return 0, 0, t, true
	}

	se := strings.SplitN(rangePart, "-", 2)
	if len(se) != 2 {
		return 0, 0, 0, false
	}

	st, err1 := strconv.ParseInt(strings.TrimSpace(se[0]), 10, 64)
	en, err2 := strconv.ParseInt(strings.TrimSpace(se[1]), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, 0, false
	}

	var t int64
	if totalPart == "*" {
		t = -1
	} else {
		var err error
		t, err = strconv.ParseInt(totalPart, 10, 64)
		if err != nil || t < 0 {
			return 0, 0, 0, false
		}
	}

	return st, en, t, true
}

func retryableDownloadErr(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}

	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	return false
}

func sleepDownloadRetry(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) headContentLength(ctx context.Context, repoID, revision, filename string) (int64, error) {
	if revision == "" {
		revision = "main"
	}

	url := fmt.Sprintf("%s/%s/resolve/%s/%s", c.baseURL, repoID, revision, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return -1, err
	}

	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return -1, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("HEAD вернул %d", resp.StatusCode)
	}

	cl := resp.ContentLength
	if cl <= 0 {
		return -1, fmt.Errorf("HEAD без Content-Length")
	}

	return cl, nil
}

func (c *Client) DownloadToPath(ctx context.Context, repoID, revision, filename, dstPath string, onProgress ProgressFunc) (written int64, total int64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if revision == "" {
		revision = "main"
	}

	url := fmt.Sprintf("%s/%s/resolve/%s/%s", c.baseURL, repoID, revision, filename)

	const maxBackoff = 45 * time.Second
	backoff := time.Duration(0)

	for {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}

		var resume int64
		if st, e := os.Stat(dstPath); e == nil {
			resume = st.Size()
		} else if !os.IsNotExist(e) {
			return 0, 0, e
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, 0, err
		}

		c.setAuth(req)
		if resume > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resume))
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if retryableDownloadErr(err) {
				if backoff == 0 {
					backoff = time.Second
				} else {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}

				if err := sleepDownloadRetry(ctx, backoff); err != nil {
					return 0, 0, err
				}

				continue
			}

			return 0, 0, err
		}

		switch resp.StatusCode {
		case http.StatusOK:
			if resume > 0 {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				if err := os.Truncate(dstPath, 0); err != nil {
					return 0, 0, err
				}

				backoff = 0
				continue
			}

			total = resp.ContentLength
			f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				resp.Body.Close()

				return 0, 0, err
			}

			var body io.Reader = resp.Body
			if onProgress != nil {
				body = &progressReader{r: resp.Body, base: 0, total: total, on: onProgress}
			}

			sessWritten, copyErr := io.Copy(f, body)
			_ = f.Close()
			_ = resp.Body.Close()

			newSize := sessWritten
			if copyErr != nil {
				if retryableDownloadErr(copyErr) {
					if backoff == 0 {
						backoff = time.Second
					} else {
						backoff *= 2
						if backoff > maxBackoff {
							backoff = maxBackoff
						}
					}

					if err := sleepDownloadRetry(ctx, backoff); err != nil {
						return newSize, total, err
					}

					continue
				}

				return newSize, total, copyErr
			}

			if total > 0 && newSize != total {
				return newSize, total, fmt.Errorf("неполный файл: %d из %d байт", newSize, total)
			}

			backoff = 0

			return newSize, total, nil

		case http.StatusPartialContent:
			cr := resp.Header.Get("Content-Range")
			start, _, t, ok := parseContentRange(cr)
			if !ok {
				resp.Body.Close()

				return 0, 0, fmt.Errorf("некорректный Content-Range: %q", cr)
			}

			if start != resume {
				resp.Body.Close()

				return 0, 0, fmt.Errorf("несовпадение Range: сервер с %d, локально %d", start, resume)
			}

			total = t

			f, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				resp.Body.Close()

				return 0, 0, err
			}

			if _, err := f.Seek(resume, io.SeekStart); err != nil {
				resp.Body.Close()
				_ = f.Close()

				return 0, 0, err
			}

			var body io.Reader = resp.Body
			if onProgress != nil {
				body = &progressReader{r: resp.Body, base: resume, total: total, on: onProgress}
			}

			sessWritten, copyErr := io.Copy(f, body)
			_ = f.Close()
			_ = resp.Body.Close()

			newSize := resume + sessWritten
			if copyErr != nil {
				if retryableDownloadErr(copyErr) {
					if backoff == 0 {
						backoff = time.Second
					} else {
						backoff *= 2
						if backoff > maxBackoff {
							backoff = maxBackoff
						}
					}

					if err := sleepDownloadRetry(ctx, backoff); err != nil {
						return newSize, total, err
					}

					continue
				}

				return newSize, total, copyErr
			}

			if total > 0 && newSize != total {
				return newSize, total, fmt.Errorf("неполный файл: %d из %d байт", newSize, total)
			}

			backoff = 0

			return newSize, total, nil

		case http.StatusRequestedRangeNotSatisfiable:
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()

			st, stErr := os.Stat(dstPath)
			if stErr != nil {
				return 0, 0, stErr
			}

			cl, he := c.headContentLength(ctx, repoID, revision, filename)
			if he == nil && cl > 0 && st.Size() == cl {
				if onProgress != nil {
					onProgress(cl, cl)
				}

				backoff = 0

				return cl, cl, nil
			}

			return st.Size(), 0, fmt.Errorf("HTTP 416 (размер на диске %d байт)", st.Size())

		default:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()

			return 0, 0, fmt.Errorf("загрузка вернула %d: %s", resp.StatusCode, string(body))
		}
	}
}

func (c *Client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
