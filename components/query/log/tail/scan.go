package tail

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Scan scans the file or commands output from the end of the file
// and return the number of matched lines.
// It returns the lines in the reverse order that evaluates true
// for the "match" function.
// If the match function is nil, returns all.
func Scan(ctx context.Context, opts ...OpOption) (int, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return 0, err
	}

	file := op.file
	if file == "" {
		if len(op.commands) == 0 {
			return 0, errors.New("file or commands must be set")
		}

		f, err := os.CreateTemp(os.TempDir(), "tailscan*.txt")
		if err != nil {
			return 0, err
		}
		defer os.Remove(f.Name())
		file = f.Name()

		log.Logger.Debugw("writing commands to file to scan", "commands", op.commands)
		p, err := process.New(process.WithCommands(op.commands), process.WithRunAsBashScript(), process.WithOutputFile(f))
		if err != nil {
			return 0, err
		}
		if err := p.Start(ctx); err != nil {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case err := <-p.Wait():
			if err != nil {
				return 0, err
			}
		}
		if err := f.Sync(); err != nil {
			return 0, err
		}
		if err := p.Abort(ctx); err != nil {
			return 0, err
		}
	}

	f, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}
	fileSize := stat.Size()

	// pre-allocate buffers
	chunkBuf := make([]byte, 4096)
	lineBuf := make([]byte, 0, 256)

	// read backwards from the end of the file
	scannedLines := 0
	matchedLines := 0

	defer func() {
		log.Logger.Debugw("scanned lines", "lines", scannedLines, "matched", matchedLines)
	}()
	for offset := fileSize; offset > 0; {
		chunkSize := int64(len(chunkBuf))
		if offset < chunkSize {
			chunkSize = offset
		}
		offset -= chunkSize

		if _, serr := f.Seek(offset, io.SeekStart); serr != nil {
			return 0, serr
		}
		if _, rerr := f.Read(chunkBuf[:chunkSize]); rerr != nil {
			return 0, rerr
		}

		for i := chunkSize - 1; i >= 0; i-- {
			if chunkBuf[i] == '\n' {
				if len(lineBuf) > 0 {
					reverse(lineBuf)
					scannedLines++

					if op.perLineFunc != nil {
						op.perLineFunc(lineBuf)
					}

					shouldInclude, matchedFilter, err := op.applyFilter(lineBuf)
					if err != nil {
						return 0, err
					}
					if shouldInclude {
						matchedLines++

						parsedTime, err := op.parseTime(lineBuf)
						if err != nil {
							return 0, err
						}
						op.processMatched(lineBuf, parsedTime, matchedFilter)
					}

					lineBuf = lineBuf[:0]
				}
			} else {
				lineBuf = append(lineBuf, chunkBuf[i])
			}

			if scannedLines == op.linesToTail {
				return matchedLines, nil
			}
		}
	}

	if len(lineBuf) > 0 && scannedLines < op.linesToTail {
		reverse(lineBuf)

		if op.perLineFunc != nil {
			op.perLineFunc(lineBuf)
		}

		shouldInclude, matchedFilter, err := op.applyFilter(lineBuf)
		if err != nil {
			return 0, err
		}
		if shouldInclude {
			matchedLines++

			parsedTime, err := op.parseTime(lineBuf)
			if err != nil {
				return 0, err
			}
			op.processMatched(lineBuf, parsedTime, matchedFilter)
		}
	}

	return matchedLines, nil
}

func reverse(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
