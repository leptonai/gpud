// Package common provides the common log components.
package common

import (
	"time"
)

type ExtractTimeFunc func([]byte) (time.Time, []byte, error)

type ProcessMatchedFunc func(parsedTime time.Time, line []byte, filter *Filter)
