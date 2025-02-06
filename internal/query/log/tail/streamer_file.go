package tail

import (
	"context"
	"time"

	query_log_common "github.com/leptonai/gpud/internal/query/log/common"
	"github.com/leptonai/gpud/log"

	"github.com/nxadm/tail"
)

func NewFromFile(ctx context.Context, file string, seek *tail.SeekInfo, opts ...OpOption) (Streamer, error) {
	op := &Op{
		file: file,
	}
	if err := op.ApplyOpts(opts); err != nil {
		return nil, err
	}

	f, err := tail.TailFile(
		file,
		tail.Config{
			Location: seek,

			Follow:    true,
			ReOpen:    true,
			MustExist: false,

			// we don't need real-time logs
			// using polling for reliability (vs. fsnotify)
			Poll: true,

			Logger: tail.DefaultLogger,
		},
	)
	if err != nil {
		return nil, err
	}

	sr := &fileStreamer{
		ctx:           ctx,
		op:            op,
		file:          f,
		lineC:         make(chan Line, 1000),
		dedupEnabled:  op.dedup,
		extractTime:   op.extractTime,
		skipEmptyLine: op.skipEmptyLine,
	}
	if op.dedup {
		sr.dedup = seenPool.Get().(*streamDeduper)
	}

	go sr.pollLoops()

	return sr, nil
}

var _ Streamer = (*fileStreamer)(nil)

type fileStreamer struct {
	ctx           context.Context
	op            *Op
	file          *tail.Tail
	lineC         chan Line
	dedupEnabled  bool
	dedup         *streamDeduper
	extractTime   query_log_common.ExtractTimeFunc
	skipEmptyLine bool
}

func (sr *fileStreamer) File() string {
	return sr.file.Filename
}

func (sr *fileStreamer) Commands() [][]string {
	return nil
}

func (sr *fileStreamer) Line() <-chan Line {
	return sr.lineC
}

func (sr *fileStreamer) pollLoops() {
	prevTime := time.Time{}
	for line := range sr.file.Lines {
		shouldInclude, matchedFilter, err := sr.op.applyFilter(line.Text)
		if err != nil {
			log.Logger.Warnw("error applying filter", "error", err)
			continue
		}
		if !shouldInclude {
			continue
		}

		txt := line.Text

		if len(txt) == 0 && sr.skipEmptyLine {
			continue
		}

		if sr.dedupEnabled {
			sr.dedup.mu.Lock()
			_, exists := sr.dedup.seen[txt]
			if exists {
				sr.dedup.mu.Unlock()
				continue
			}
			sr.dedup.seen[txt] = struct{}{}
			sr.dedup.mu.Unlock()
		}

		if sr.extractTime != nil {
			parsedTime, _, err := sr.extractTime([]byte(txt))
			if err == nil {
				line.Time = parsedTime
			} else {
				log.Logger.Warnw("error extracting time", "error", err)
			}

			if line.Time.IsZero() && !prevTime.IsZero() {
				line.Time = prevTime
			}

			if err == nil {
				prevTime = parsedTime
			}
		}

		if sr.op.ProcessMatched != nil {
			sr.op.ProcessMatched(line.Time, []byte(line.Text), matchedFilter)
		}

		lineToSend := Line{
			Line:          line,
			MatchedFilter: matchedFilter,
		}

		select {
		case <-sr.ctx.Done():
			sr.file.Done()

			if sr.dedupEnabled {
				sr.dedup.mu.Lock()
				for k := range sr.dedup.seen {
					delete(sr.dedup.seen, k)
				}
				sr.dedup.mu.Unlock()
				seenPool.Put(sr.dedup)
			}
			return

		case sr.lineC <- lineToSend:

		default:
			log.Logger.Warnw("channel is full -- dropped output", "file", sr.file.Filename)
		}
	}
}
