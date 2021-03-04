package tracing

import (
	"bytes"
	"io"
	"strconv"
	"strings"
)

var header = "(?<host>\\S*) (?<clock>{.*})\\n(?<event>.*)"

type shivizLogger struct {
	w io.Writer
}

func newShivizLogger(w io.Writer) (*shivizLogger, error) {
	if _, err := w.Write([]byte(header + "\n\n")); err != nil {
		return nil, err
	}
	return &shivizLogger{w: w}, nil
}

func (s *shivizLogger) log(tRecord TraceRecord) error {
	var buffer bytes.Buffer
	line1 := []string{tRecord.TracerIdentity, tRecord.VectorClock.ReturnVCString()}
	if _, err := buffer.WriteString(strings.Join(line1, " ") + "\n"); err != nil {
		return err
	}

	line2 := []string{strconv.FormatUint(tRecord.TraceID, 10), tRecord.Tag, string(tRecord.Body)}
	if _, err := buffer.WriteString(strings.Join(line2, " ") + "\n"); err != nil {
		return err
	}

	if _, err := s.w.Write(buffer.Bytes()); err != nil {
		return err
	}
	return nil
}
