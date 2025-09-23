package session

import (
	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) processGossip(resp *Response) {
	if s.createGossipRequestFunc == nil {
		return
	}

	gossipReq, err := s.createGossipRequestFunc(s.machineID, s.nvmlInstance)
	if err != nil {
		log.Logger.Errorw("failed to create gossip request", "error", err)
		resp.Error = err.Error()
		return
	}

	resp.GossipRequest = gossipReq
	log.Logger.Debugw("successfully set gossip request")
}
