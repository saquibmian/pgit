package logwriter

import (
	"bytes"
	"io"
	"log"
)

// LogWriter is an io.Writer that wraps a log.Logger
type LogWriter struct {
	Logger    *log.Logger
    buf       *bytes.Buffer
	readLines string
}

// NewLogWriter creates a new LogWriter that wraps a log.Logger
func NewLogWriter(logger *log.Logger) *LogWriter {
	writer := &LogWriter{
		Logger:     logger,
        buf:        bytes.NewBuffer([]byte("")),
	}
	return writer
}

func (l *LogWriter) Write(p []byte) (n int, err error) {
	if n, err = l.buf.Write(p); err != nil {
		return
	}

	err = l.Flush()
	return
}

func (l *LogWriter) Flush() (err error) {
	for {
		line, err := l.buf.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		l.readLines += line
		l.Logger.Print(line)
	}

	return nil
}
