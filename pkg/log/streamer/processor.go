package streamer

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

type Processor interface {
	Events(ctx context.Context, since time.Time) (apiv1.Events, error)
	Close()
}

type processorImpl struct {
	ctx         context.Context
	streamer    cmdStreamer
	matchFunc   MatchFunc
	eventBucket eventstore.Bucket
}

type MatchFunc func(line string) (eventName string, message string)

type ParseLogFunc func(line string) LogLine

func New(
	ctx context.Context,
	cmds [][]string,
	matchFunc MatchFunc,
	parseLogFunc ParseLogFunc,
	eventBucket eventstore.Bucket,
) (Processor, error) {
	streamer, err := newCmdStreamer(cmds, parseLogFunc)
	if err != nil {
		return nil, err
	}

	llp := &processorImpl{
		ctx:         ctx,
		streamer:    streamer,
		matchFunc:   matchFunc,
		eventBucket: eventBucket,
	}
	go llp.watch()

	return llp, nil
}

func (llp *processorImpl) watch() {
	ch := llp.streamer.watch()
	for {
		select {
		case <-llp.ctx.Done():
			return
		case line, open := <-ch:
			if !open {
				return
			}

			ev := eventstore.Event{
				Time: line.Time.UTC(),
				Type: string(apiv1.EventTypeWarning),
			}
			ev.Name, ev.Message = llp.matchFunc(line.Content)
			if ev.Name == "" {
				continue
			}

			// lookup to prevent duplicate event insertions
			cctx, ccancel := context.WithTimeout(llp.ctx, 15*time.Second)
			found, err := llp.eventBucket.Find(cctx, ev)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to find event", "eventName", ev.Name, "eventType", ev.Type, "error", err)
			}
			if found != nil {
				continue
			}

			// insert event
			cctx, ccancel = context.WithTimeout(llp.ctx, 15*time.Second)
			err = llp.eventBucket.Insert(cctx, ev)
			ccancel()
			if err != nil {
				log.Logger.Warnw("failed to insert event", "error", err)
			} else {
				log.Logger.Infow("successfully inserted event", "event", ev.Name)
			}
		}
	}
}

func (llp *processorImpl) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	evs, err := llp.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return evs.Events(), nil
}

func (llp *processorImpl) Close() {
	llp.streamer.close()
}
