// Package common provides the common log components.
package common

import (
	"time"
)

type ParseTimeFunc func([]byte) (time.Time, error)

type ProcessMatchedFunc func(line []byte, parsedTime time.Time, filter *Filter)
