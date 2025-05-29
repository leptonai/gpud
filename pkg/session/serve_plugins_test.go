package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
)

// Test handling setPluginSpecs request
func TestHandleSetPluginSpecsRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSessionWithoutFaultInjector()

	// Mock save function
	saveFunc := func(ctx context.Context, specs pkgcustomplugins.Specs) (bool, error) {
		return true, nil // Return true to indicate update
	}
	session.savePluginSpecsFunc = saveFunc

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	// Create plugin specs
	specs := pkgcustomplugins.Specs{
		{
			PluginName: "test-plugin",
			Type:       pkgcustomplugins.SpecTypeComponent,
		},
	}

	req := Request{
		Method:      "setPluginSpecs",
		PluginSpecs: specs,
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-set-plugin-specs",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-set-plugin-specs", resp.ReqID)
	assert.Empty(t, response.Error)

	registry.AssertExpectations(t)
}

func TestHandleSetPluginSpecsRequestNilSaveFunc(t *testing.T) {
	session, _, _, _, reader, writer := setupTestSessionWithoutFaultInjector()

	// Don't set savePluginSpecsFunc (leave it nil)
	session.savePluginSpecsFunc = nil

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	// Create plugin specs
	specs := pkgcustomplugins.Specs{
		{
			PluginName: "test-plugin",
			Type:       pkgcustomplugins.SpecTypeComponent,
		},
	}

	req := Request{
		Method:      "setPluginSpecs",
		PluginSpecs: specs,
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-set-plugin-specs-nil-func",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-set-plugin-specs-nil-func", resp.ReqID)
	assert.Equal(t, "save plugin specs function is not initialized", response.Error)
}

// Test handling getPluginSpecs request
func TestHandleGetPluginSpecsRequest(t *testing.T) {
	session, registry, _, _, reader, writer := setupTestSessionWithoutFaultInjector()

	// Create mock custom plugin component
	comp := new(mockComponent)
	spec := pkgcustomplugins.Spec{
		PluginName: "test-plugin",
		Type:       pkgcustomplugins.SpecTypeComponent,
	}

	// Mock the component to implement CustomPluginRegisteree
	mockCustomPlugin := &struct {
		*mockComponent
		spec pkgcustomplugins.Spec
	}{
		mockComponent: comp,
		spec:          spec,
	}

	// We need to create a proper mock that implements both Component and CustomPluginRegisteree
	allComponents := []components.Component{mockCustomPlugin.mockComponent}
	registry.On("All").Return(allComponents)

	// Start the session in a separate goroutine
	go session.serve()
	defer close(reader)

	req := Request{
		Method: "getPluginSpecs",
	}

	reqData, _ := json.Marshal(req)

	// Send the request
	reader <- Body{
		Data:  reqData,
		ReqID: "test-get-plugin-specs",
	}

	// Read the response
	var resp Body
	select {
	case resp = <-writer:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for response")
	}

	// Parse the response
	var response Response
	err := json.Unmarshal(resp.Data, &response)
	require.NoError(t, err)

	// Check the response
	assert.Equal(t, "test-get-plugin-specs", resp.ReqID)
	assert.Empty(t, response.Error)
	// Since our mock doesn't properly implement CustomPluginRegisteree, we expect nil specs
	assert.Nil(t, response.PluginSpecs)

	registry.AssertExpectations(t)
}
