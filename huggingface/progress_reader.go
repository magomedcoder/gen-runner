package huggingface

import (
	"io"
	"time"
)

const progressReportMinInterval = 200 * time.Millisecond

type ProgressFunc func(written, total int64)

type progressReader struct {
	r         io.Reader
	base      int64
	written   int64
	total     int64
	on        ProgressFunc
	nextFlush time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.written += int64(n)
		p.maybeReport(err == io.EOF)
	}

	if err == io.EOF {
		p.maybeReport(true)
	}

	return n, err
}

func (p *progressReader) maybeReport(force bool) {
	if p.on == nil {
		return
	}

	now := time.Now()
	if !force {
		switch {
		case p.total > 0 && p.written < p.total && now.Before(p.nextFlush):
			return
		case p.total <= 0 && now.Before(p.nextFlush):
			return
		}
	}

	p.on(p.base+p.written, p.total)
	p.nextFlush = now.Add(progressReportMinInterval)
}
