package redact

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"slices"
	"strings"
	"sync"
)

type (

	// Redactor masks known secret values in text. The zero value and a nil
	// Redactor mask nothing.
	Redactor struct {
		replacer *strings.Replacer
	}
	redactedError struct {
		err     error
		message string
	}
	nopWriteCloser  struct{ io.Writer }
	redactingWriter struct {
		w          io.Writer
		r          *Redactor
		buf        []byte
		mu         sync.Mutex
		discarding bool
	}
)

const (

	// RedactedMarker replaces secret values in output and messages.
	RedactedMarker = "[REDACTED]"
	// Empty values are excluded from replacement.
	minRedactLength = 1
	// maxPendingLine bounds how much of an unterminated line is held back.
	maxPendingLine = 64 * 1024
)

var htmlEscaper = strings.NewReplacer("<", `\u003c`, ">", `\u003e`, "&", `\u0026`, "\u2028", `\u2028`, "\u2029", `\u2029`)

// NewRedactor builds a redactor for values. Each value is also matched as it
// appears inside a JSON string, because deploy progress is streamed as JSON.
func NewRedactor(values map[string]string) *Redactor {
	seen := make(map[string]struct{})
	var needles []string
	add := func(needle string) {
		if len(needle) < minRedactLength {
			return
		}
		if _, ok := seen[needle]; ok {
			return
		}
		seen[needle] = struct{}{}
		needles = append(needles, needle)
	}
	for _, value := range values {
		add(value)
		if encoded, err := json.Marshal(value); err == nil && len(encoded) >= 2 {
			inner := string(encoded[1 : len(encoded)-1])
			add(inner)
			// encoding/json (v1), which Docker uses, also escapes <, >, and &.
			add(htmlEscaper.Replace(inner))
		}
		// Multi-line values (keys, certificates) can surface one line at a time.
		if strings.Contains(value, "\n") {
			for line := range strings.SplitSeq(value, "\n") {
				add(strings.TrimSpace(line))
			}
		}
	}
	if len(needles) == 0 {
		return nil
	}
	// Longest first, so a value that contains another is masked whole.
	slices.SortFunc(needles, func(a, b string) int { return len(b) - len(a) })
	pairs := make([]string, 0, len(needles)*2)
	for _, needle := range needles {
		pairs = append(pairs, needle, RedactedMarker)
	}
	return &Redactor{replacer: strings.NewReplacer(pairs...)}
}

// String masks every known value in text.
func (r *Redactor) String(text string) string {
	if r == nil || text == "" {
		return text
	}
	return r.replacer.Replace(text)
}

// Error returns err with its message masked. errors.Is and errors.As still
// see the original error.
func (r *Redactor) Error(err error) error {
	if r == nil || err == nil {
		return err
	}
	message := err.Error()
	masked := r.String(message)
	if masked == message {
		return err
	}
	return &redactedError{err: err, message: masked}
}

func (e *redactedError) Error() string { return e.message }

// Unwrap keeps the error's identity. Code that prints an unwrapped error
// could see the original text, so callers should print the outer error.
func (e *redactedError) Unwrap() error { return e.err }

// Writer masks values in everything written to w. Output is passed on one
// line at a time so a value split across writes is still caught; Close and
// Flush pass on a final partial line. A nil Redactor returns w unchanged.
func (r *Redactor) Writer(w io.Writer) io.WriteCloser {
	if w == nil {
		return nil
	}
	if r == nil {
		return nopWriteCloser{w}
	}
	return &redactingWriter{r: r, w: w}
}

func (nopWriteCloser) Close() error { return nil }

func (rw *redactingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	remaining := p
	for len(remaining) > 0 {
		end := bytes.IndexByte(remaining, '\n')
		segment := remaining
		if end >= 0 {
			segment = remaining[:end+1]
		}
		if !rw.discarding {
			if len(rw.buf)+len(segment) > maxPendingLine {
				rw.buf = rw.buf[:0]
				rw.discarding = true
			} else {
				rw.buf = append(rw.buf, segment...)
			}
		}
		if end < 0 {
			break
		}
		output := RedactedMarker + "\n"
		if !rw.discarding {
			output = rw.r.String(string(rw.buf))
		}
		if _, err := io.WriteString(rw.w, output); err != nil {
			return 0, err
		}
		rw.buf = rw.buf[:0]
		rw.discarding = false
		remaining = remaining[end+1:]
	}
	return len(p), nil
}

// Flush passes on any held-back partial line, then flushes w when it can.
func (rw *redactingWriter) Flush() {
	_ = rw.flushInternal()
	if flusher, ok := rw.w.(interface{ Flush() }); ok {
		flusher.Flush()
	}
}

func (rw *redactingWriter) flushInternal() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.discarding {
		rw.discarding = false
		_, err := io.WriteString(rw.w, RedactedMarker)
		return err
	}
	if len(rw.buf) == 0 {
		return nil
	}
	_, err := io.WriteString(rw.w, rw.r.String(string(rw.buf)))
	rw.buf = rw.buf[:0]
	return err
}

func (rw *redactingWriter) Close() error { return rw.flushInternal() }
