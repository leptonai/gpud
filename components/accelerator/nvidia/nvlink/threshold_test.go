package nvlink

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaultExpectedLinkStates(t *testing.T) {
	original := GetDefaultExpectedLinkStates()
	t.Cleanup(func() { SetDefaultExpectedLinkStates(original) })

	SetDefaultExpectedLinkStates(ExpectedLinkStates{
		MaxInactiveNVLinks:   -1,
		MaxUnhealthyP2PPeers: -1,
	})
	assert.Equal(t, ExpectedLinkStates{}, GetDefaultExpectedLinkStates())

	want := ExpectedLinkStates{MaxInactiveNVLinks: 1, MaxUnhealthyP2PPeers: 2}
	SetDefaultExpectedLinkStates(want)
	assert.Equal(t, want, GetDefaultExpectedLinkStates())
}
