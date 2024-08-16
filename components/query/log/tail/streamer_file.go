package tail

import (
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/nxadm/tail"
)

func NewFromFile(file string, seek *tail.SeekInfo, opts ...OpOption) (Streamer, error) {
	op := &Op{
		file: file,
	}
	if err := op.applyOpts(opts); err != nil {
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
		op:    op,
		file:  f,
		lineC: make(chan Line, 100),
	}
	go sr.pollLoops()

	return sr, nil
}

var _ Streamer = (*fileStreamer)(nil)

type fileStreamer struct {
	op    *Op
	file  *tail.Tail
	lineC chan Line
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

		if line.Time.IsZero() {
			line.Time = time.Now().UTC()
		}

		sr.lineC <- Line{
			Line:          line,
			MatchedFilter: matchedFilter,
		}
	}
}
