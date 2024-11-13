package tail

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

var dedupMapPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]struct{}, 200)
	},
}

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

	// use regular buffers for chunk and line reading
	chunkBuf := make([]byte, 4096)
	lineBuf := make([]byte, 0, 256)

	// read backwards from the end of the file
	scannedLines := 0
	matchedLines := 0

	var dedupedLines map[string]struct{}
	if op.dedup {
		// only use sync.Pool for dedup map
		dedupedLines = dedupMapPool.Get().(map[string]struct{})
		defer func() {
			// clear the map before returning it to pool
			for k := range dedupedLines {
				delete(dedupedLines, k)
			}
			dedupMapPool.Put(dedupedLines)
		}()
	}

	processLine := func(buf []byte) error {
		reverse(buf)
		scannedLines++

		if op.perLineFunc != nil {
			op.perLineFunc(buf)
		}

		shouldInclude, matchedFilter, err := op.applyFilter(buf)
		if err != nil {
			return err
		}
		if !shouldInclude {
			return nil
		}

		if op.dedup {
			if _, ok := dedupedLines[string(buf)]; ok {
				// skip duplicate
				return nil
			}

			dedupedLines[string(buf)] = struct{}{}
		}

		matchedLines++
		parsedTime, err := op.parseTime(buf)
		if err != nil {
			return err
		}
		op.processMatched(buf, parsedTime, matchedFilter)

		return nil
	}

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
			if scannedLines == op.linesToTail {
				return matchedLines, nil
			}

			// still processing a line
			if chunkBuf[i] != '\n' {
				lineBuf = append(lineBuf, chunkBuf[i])
				continue
			}

			// end of a line but no content
			if len(lineBuf) == 0 {
				continue
			}

			if err := processLine(lineBuf); err != nil {
				return 0, err
			}

			lineBuf = lineBuf[:0]
		}
	}

	if len(lineBuf) > 0 && scannedLines < op.linesToTail {
		if err := processLine(lineBuf); err != nil {
			return 0, err
		}
	}

	return matchedLines, nil
}

func reverse(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
