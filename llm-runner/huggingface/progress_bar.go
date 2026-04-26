package huggingface

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const progressBarWidth = 28

func terminalProgressLine(label string, written, total int64) string {
	label = strings.TrimSpace(label)
	if len(label) > 22 {
		label = label[:19] + "..."
	}

	var pct float64
	if total > 0 {
		pct = 100 * float64(written) / float64(total)
		if pct > 100 {
			pct = 100
		}
	}

	filled := int(float64(progressBarWidth) * pct / 100)
	if total > 0 && filled == 0 && written > 0 {
		filled = 1
	}

	var bar strings.Builder
	bar.Grow(progressBarWidth + 2)
	bar.WriteByte('[')
	for i := range progressBarWidth {
		if i < filled {
			bar.WriteByte('=')
		} else {
			bar.WriteByte('.')
		}
	}

	bar.WriteByte(']')

	var sizePart string
	if total > 0 {
		sizePart = fmt.Sprintf("%s / %s", formatBytes(written), formatBytes(total))
	} else {
		sizePart = formatBytes(written)
	}

	line := fmt.Sprintf("%-22s %s %5.1f%%  %s", label, bar.String(), pct, sizePart)

	const maxLen = 120
	if len(line) > maxLen {
		line = line[:maxLen]
	}

	return line
}

type downloadProgress struct {
	out   io.Writer
	label string
	mu    *sync.Mutex
}

func newDownloadProgress(out io.Writer, filename string, mu *sync.Mutex) *downloadProgress {
	if out == nil {
		out = os.Stdout
	}

	return &downloadProgress{out: out, label: filename, mu: mu}
}

func (d *downloadProgress) report(written, total int64) {
	line := "\r" + terminalProgressLine(d.label, written, total) + "\x1b[K"
	if d.mu != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
	}

	_, _ = io.WriteString(d.out, line)
	if f, ok := d.out.(*os.File); ok {
		_ = f.Sync()
	}
}

func (d *downloadProgress) finish(written, total int64) {
	d.report(written, total)
	if d.mu != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
	}

	_, _ = fmt.Fprintln(d.out)
	if f, ok := d.out.(*os.File); ok {
		_ = f.Sync()
	}
}
