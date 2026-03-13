package xid

const maxIntValue = int(^uint(0) >> 1)

func intFromUint64(v uint64) (int, bool) {
	if v > uint64(maxIntValue) {
		return 0, false
	}
	return int(v), true
}

func uint64FromInt(v int) (uint64, bool) {
	if v < 0 {
		return 0, false
	}
	return uint64(v), true
}
