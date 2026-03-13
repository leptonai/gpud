package processes

import "math"

const maxUint32Value = uint64(^uint32(0))

func int32FromUint32(v uint32) (int32, bool) {
	if v > math.MaxInt32 {
		return 0, false
	}
	return int32(v), true
}

func uint32FromUint64(v uint64) (uint32, bool) {
	if v > maxUint32Value {
		return 0, false
	}
	return uint32(v), true
}
