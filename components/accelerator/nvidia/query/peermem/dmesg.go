// Package peermem contains the implementation of the peermem query for NVIDIA GPUs.
package peermem

import "regexp"

// repeated messages may indicate more persistent issue on the inter-GPU communication
// e.g.,
// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
// [Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing
const RegexInvalidContext = `.*ERROR detected invalid context, skipping further processing`

var (
	compiledInvalidContext = regexp.MustCompile(RegexInvalidContext)
)

func HasInvalidContext(line string) bool {
	if match := compiledInvalidContext.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}
