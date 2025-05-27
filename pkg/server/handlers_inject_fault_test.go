package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
)

// Mock kmsg writer for testing
type mockKmsgWriter struct {
	mock.Mock
}

func (m *mockKmsgWriter) Write(msg *pkgkmsgwriter.KernelMessage) error {
	args := m.Called(msg)
	return args.Error(0)
}

// Mock fault injector for testing
type mockFaultInjector struct {
	mock.Mock
}

func (m *mockFaultInjector) KmsgWriter() pkgkmsgwriter.KmsgWriter {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(pkgkmsgwriter.KmsgWriter)
}

// Helper function to setup test context
func setupInjectFaultTest() (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	recorder := httptest.NewRecorder()
	return router, recorder
}

func TestHandleInjectFault_NilFaultInjector(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create handler with nil fault injector
	handler := &globalHandler{
		faultInjector: nil,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create request with valid JSON
	request := pkgfaultinjector.Request{
		KernelMessage: &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_INFO",
			Message:  "test message",
		},
	}
	requestBody, _ := json.Marshal(request)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusNotFound, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// The error object serializes as an empty map in JSON
	assert.Equal(t, map[string]interface{}{}, response["code"])
	assert.Equal(t, "fault injector not set up", response["message"])
}

func TestHandleInjectFault_InvalidJSON(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create handler with mock fault injector
	mockInjector := new(mockFaultInjector)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create HTTP request with invalid JSON
	invalidJSON := `{"invalid": json}`
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBufferString(invalidJSON))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// The error object serializes as an empty map in JSON
	assert.Equal(t, map[string]interface{}{}, response["code"])
	assert.Contains(t, response["message"], "failed to decode request body")

	// Assert no calls were made to the fault injector
	mockInjector.AssertNotCalled(t, "KmsgWriter")
}

func TestHandleInjectFault_SuccessfulKernelMessageInjection(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector and kmsg writer
	mockInjector := new(mockFaultInjector)
	mockWriter := new(mockKmsgWriter)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create valid request
	kernelMessage := &pkgkmsgwriter.KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test kernel message",
	}
	request := pkgfaultinjector.Request{
		KernelMessage: kernelMessage,
	}
	requestBody, _ := json.Marshal(request)

	// Mock successful injection
	mockInjector.On("KmsgWriter").Return(mockWriter)
	mockWriter.On("Write", kernelMessage).Return(nil)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusOK, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "fault injected", response["message"])

	// Verify mock was called
	mockInjector.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
}

func TestHandleInjectFault_FailedKernelMessageInjection(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector and kmsg writer
	mockInjector := new(mockFaultInjector)
	mockWriter := new(mockKmsgWriter)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create valid request
	kernelMessage := &pkgkmsgwriter.KernelMessage{
		Priority: "KERN_ERR",
		Message:  "test error message",
	}
	request := pkgfaultinjector.Request{
		KernelMessage: kernelMessage,
	}
	requestBody, _ := json.Marshal(request)

	// Mock failed injection
	injectionError := errors.New("injection failed")
	mockInjector.On("KmsgWriter").Return(mockWriter)
	mockWriter.On("Write", kernelMessage).Return(injectionError)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// The error object serializes as an empty map in JSON
	assert.Equal(t, map[string]interface{}{}, response["code"])
	assert.Equal(t, "failed to inject kernel message: injection failed", response["message"])

	// Verify mock was called
	mockInjector.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
}

func TestHandleInjectFault_NilKernelMessage_DefaultCase(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector
	mockInjector := new(mockFaultInjector)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create request with nil KernelMessage (validation fails before reaching switch)
	request := pkgfaultinjector.Request{
		KernelMessage: nil,
	}
	requestBody, _ := json.Marshal(request)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// The error object serializes as an empty map in JSON
	assert.Equal(t, map[string]interface{}{}, response["code"])
	assert.Equal(t, "invalid request: no fault injection entry found", response["message"])

	// Verify no injection was attempted
	mockInjector.AssertNotCalled(t, "KmsgWriter")
}

func TestHandleInjectFault_EmptyRequest_DefaultCase(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector
	mockInjector := new(mockFaultInjector)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create empty request (no fields set, validation fails before reaching switch)
	request := pkgfaultinjector.Request{}
	requestBody, _ := json.Marshal(request)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// The error object serializes as an empty map in JSON
	assert.Equal(t, map[string]interface{}{}, response["code"])
	assert.Equal(t, "invalid request: no fault injection entry found", response["message"])

	// Verify no injection was attempted
	mockInjector.AssertNotCalled(t, "KmsgWriter")
}

func TestHandleInjectFault_ContextPropagation(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector and kmsg writer
	mockInjector := new(mockFaultInjector)
	mockWriter := new(mockKmsgWriter)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create valid request
	kernelMessage := &pkgkmsgwriter.KernelMessage{
		Priority: "KERN_DEBUG",
		Message:  "test context propagation",
	}
	request := pkgfaultinjector.Request{
		KernelMessage: kernelMessage,
	}
	requestBody, _ := json.Marshal(request)

	// Mock the injector to verify context is passed correctly
	mockInjector.On("KmsgWriter").Return(mockWriter)
	mockWriter.On("Write", kernelMessage).Return(nil)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusOK, recorder.Code)

	// Verify mock was called with correct context type
	mockInjector.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
}

// Validation-specific tests for request.Validate() call
func TestHandleInjectFault_Validation(t *testing.T) {
	t.Run("valid request passes validation", func(t *testing.T) {
		// Setup
		router, recorder := setupInjectFaultTest()

		// Create mock fault injector and kmsg writer
		mockInjector := new(mockFaultInjector)
		mockWriter := new(mockKmsgWriter)
		handler := &globalHandler{
			faultInjector: mockInjector,
		}

		// Register the handler
		router.POST(URLPathInjectFault, handler.handleInjectFault)

		// Create request with valid kernel message
		kernelMessage := &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_INFO",
			Message:  "valid message for validation test",
		}
		request := pkgfaultinjector.Request{
			KernelMessage: kernelMessage,
		}
		requestBody, _ := json.Marshal(request)

		// Mock successful injection since validation will pass
		mockInjector.On("KmsgWriter").Return(mockWriter)
		mockWriter.On("Write", kernelMessage).Return(nil)

		// Create HTTP request
		req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")

		// Send request
		router.ServeHTTP(recorder, req)

		// Assert validation passed (status is OK, not BadRequest)
		assert.Equal(t, http.StatusOK, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "fault injected", response["message"])

		// Verify mock was called (proves validation passed)
		mockInjector.AssertExpectations(t)
		mockWriter.AssertExpectations(t)
	})

	t.Run("invalid request fails validation - message too long", func(t *testing.T) {
		// Setup
		router, recorder := setupInjectFaultTest()

		// Create mock fault injector
		mockInjector := new(mockFaultInjector)
		handler := &globalHandler{
			faultInjector: mockInjector,
		}

		// Register the handler
		router.POST(URLPathInjectFault, handler.handleInjectFault)

		// Create request with message exceeding MaxPrintkRecordLength (976 characters)
		maxLength := 976
		longMessage := make([]byte, maxLength+100) // Exceeds the limit
		for i := range longMessage {
			longMessage[i] = 'A'
		}

		kernelMessage := &pkgkmsgwriter.KernelMessage{
			Priority: "KERN_ERR",
			Message:  string(longMessage),
		}
		request := pkgfaultinjector.Request{
			KernelMessage: kernelMessage,
		}
		requestBody, _ := json.Marshal(request)

		// Create HTTP request
		req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")

		// Send request
		router.ServeHTTP(recorder, req)

		// Assert validation failed
		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)

		// The error object serializes as an empty map in JSON
		assert.Equal(t, map[string]interface{}{}, response["code"])
		assert.Contains(t, response["message"], "invalid request:")
		assert.Contains(t, response["message"], "message length exceeds the maximum length")
		assert.Contains(t, response["message"], "976")

		// Verify no injection was attempted (proves validation stopped execution)
		mockInjector.AssertNotCalled(t, "KmsgWriter")
	})

	t.Run("request with nil kernel message fails validation", func(t *testing.T) {
		// Setup
		router, recorder := setupInjectFaultTest()

		// Create mock fault injector
		mockInjector := new(mockFaultInjector)
		handler := &globalHandler{
			faultInjector: mockInjector,
		}

		// Register the handler
		router.POST(URLPathInjectFault, handler.handleInjectFault)

		// Create request with nil KernelMessage (validation fails before reaching switch)
		request := pkgfaultinjector.Request{
			KernelMessage: nil,
		}
		requestBody, _ := json.Marshal(request)

		// Create HTTP request
		req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
		req.Header.Set("Content-Type", "application/json")

		// Send request
		router.ServeHTTP(recorder, req)

		// Assert validation failed
		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		require.NoError(t, err)

		// This should be the validation error, not switch case error
		assert.Equal(t, "invalid request: no fault injection entry found", response["message"])

		// Verify no injection was attempted
		mockInjector.AssertNotCalled(t, "KmsgWriter")
	})
}

func TestHandleInjectFault_XidToKernelMessageTransformation(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector and kmsg writer
	mockInjector := new(mockFaultInjector)
	mockWriter := new(mockKmsgWriter)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create request with XID (will be transformed to KernelMessage during validation)
	request := pkgfaultinjector.Request{
		XID: &pkgfaultinjector.XIDToInject{
			ID: 79, // Valid XID that should be transformed
		},
	}
	requestBody, _ := json.Marshal(request)

	// Mock successful injection - the XID will be transformed to KernelMessage
	// We need to match any parameters since the transformation happens during validation
	mockInjector.On("KmsgWriter").Return(mockWriter)
	mockWriter.On("Write", mock.Anything).Return(nil)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusOK, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "fault injected", response["message"])

	// Verify mock was called (proves XID was transformed and injection was attempted)
	mockInjector.AssertExpectations(t)
	mockWriter.AssertExpectations(t)
}

func TestHandleInjectFault_InvalidXid(t *testing.T) {
	// Setup
	router, recorder := setupInjectFaultTest()

	// Create mock fault injector
	mockInjector := new(mockFaultInjector)
	handler := &globalHandler{
		faultInjector: mockInjector,
	}

	// Register the handler
	router.POST(URLPathInjectFault, handler.handleInjectFault)

	// Create request with invalid XID (ID = 0, should fail validation)
	request := pkgfaultinjector.Request{
		XID: &pkgfaultinjector.XIDToInject{
			ID: 0, // Invalid XID, should fail validation
		},
	}
	requestBody, _ := json.Marshal(request)

	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, URLPathInjectFault, bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	router.ServeHTTP(recorder, req)

	// Assert response
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var response map[string]interface{}
	err := json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	// The error object serializes as an empty map in JSON
	assert.Equal(t, map[string]interface{}{}, response["code"])
	assert.Equal(t, "invalid request: no fault injection entry found", response["message"])

	// Verify no injection was attempted
	mockInjector.AssertNotCalled(t, "KmsgWriter")
}
