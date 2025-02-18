package peermem

import (
	"regexp"
)

const (
	// repeated messages may indicate more persistent issue on the inter-GPU communication
	// e.g.,
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
	eventPeermemInvalidContext   = "nvidia_peermem_invalid_context"
	regexPeermemInvalidContext   = `.*ERROR detected invalid context, skipping further processing`
	messagePeermemInvalidContext = `peermem error detected (possible GPU communication issue)`
)

var (
	compiledPeermemInvalidContext = regexp.MustCompile(regexPeermemInvalidContext)
)

func HasPeermemInvalidContext(line string) bool {
	if match := compiledPeermemInvalidContext.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func Match(line string) (eventName string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.message
		}
	}
	return "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasPeermemInvalidContext, eventName: eventPeermemInvalidContext, regex: regexPeermemInvalidContext, message: messagePeermemInvalidContext},
	}
}
