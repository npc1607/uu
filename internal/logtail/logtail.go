package logtail

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

type Options struct {
	File        string
	Lines       int
	Timeout     time.Duration
	WaitTimeout time.Duration
	Writer      io.Writer
}

func Follow(opts Options) error {
	if opts.File == "" {
		return errors.New("follow log file is empty")
	}
	if opts.Writer == nil {
		opts.Writer = io.Discard
	}
	if opts.WaitTimeout == 0 {
		opts.WaitTimeout = 10 * time.Second
	}

	fmt.Fprintf(opts.Writer, "Following log file: %s\n", opts.File)
	file, err := waitOpenFile(opts.File, opts.WaitTimeout)
	if err != nil {
		return err
	}
	defer file.Close()

	offset, err := printInitialLog(file, opts.Lines, opts.Writer)
	if err != nil {
		return err
	}

	deadline := time.Time{}
	if opts.Timeout > 0 {
		deadline = time.Now().Add(opts.Timeout)
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil
		}
		<-ticker.C

		stat, err := file.Stat()
		if err != nil {
			return err
		}
		if stat.Size() < offset {
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			offset = 0
		}
		if stat.Size() == offset {
			continue
		}

		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		written, err := io.Copy(opts.Writer, file)
		if err != nil {
			return err
		}
		offset += written
	}
}

func printInitialLog(file *os.File, lines int, writer io.Writer) (int64, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return 0, err
	}
	if lines > 0 {
		tail := LastLines(content, lines)
		if len(tail) > 0 {
			if _, err := writer.Write(tail); err != nil {
				return 0, err
			}
			if tail[len(tail)-1] != '\n' {
				if _, err := fmt.Fprintln(writer); err != nil {
					return 0, err
				}
			}
		}
	}
	return int64(len(content)), nil
}

func waitOpenFile(path string, timeout time.Duration) (*os.File, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		file, err := os.Open(path)
		if err == nil {
			return file, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func LastLines(content []byte, lines int) []byte {
	if lines <= 0 || len(content) == 0 {
		return nil
	}
	content = bytes.TrimRight(content, "\n")
	if len(content) == 0 {
		return nil
	}

	cut := len(content)
	for seen := 0; seen < lines && cut > 0; {
		cut = bytes.LastIndexByte(content[:cut], '\n')
		if cut == -1 {
			return content
		}
		seen++
	}
	return content[cut+1:]
}
