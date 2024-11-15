package tail

import (
	"context"
	"time"

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
		ctx:          ctx,
		op:           op,
		file:         f,
		lineC:        make(chan Line, 100),
		dedupEnabled: op.dedup,
	}
	if op.dedup {
		sr.dedup = seenPool.Get().(*streamDeduper)
	}

	go sr.pollLoops()

	return sr, nil
}

var _ Streamer = (*fileStreamer)(nil)

type fileStreamer struct {
	ctx          context.Context
	op           *Op
	file         *tail.Tail
	lineC        chan Line
	dedupEnabled bool
	dedup        *streamDeduper
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
	for line := range sr.file.Lines {
		shouldInclude, matchedFilter, err := sr.op.applyFilter(line.Text)
		if err != nil {
			log.Logger.Warnw("error applying filter", "error", err)
			continue
		}
		if !shouldInclude {
			continue
		}

		s := line.Text

		if sr.dedupEnabled {
			sr.dedup.mu.Lock()
			_, exists := sr.dedup.seen[s]
			if exists {
				sr.dedup.mu.Unlock()
				continue
			}
			sr.dedup.seen[s] = struct{}{}
			sr.dedup.mu.Unlock()
		}

		if line.Time.IsZero() {
			line.Time = time.Now().UTC()
		}

		if sr.op.ProcessMatched != nil {
			sr.op.ProcessMatched([]byte(line.Text), line.Time, matchedFilter)
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
			log.Logger.Warnw("channel is full -- dropped output")
		}
	}
}
