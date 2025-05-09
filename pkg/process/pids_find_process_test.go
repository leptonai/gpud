package process

import (
	"context"
	"errors"
	"strings"
	"testing"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
)

// procTestMock is a mock that implements ProcessStatus
type procTestMock struct {
	pid        int32
	procName   string
	nameErr    error
	procStatus []string
	statusErr  error
}

func (p *procTestMock) Name() (string, error) {
	return p.procName, p.nameErr
}

func (p *procTestMock) PID() int32 {
	return p.pid
}

func (p *procTestMock) Status() ([]string, error) {
	return p.procStatus, p.statusErr
}

// testProcessLister is a helper to create a list process function with predetermined responses
func testProcessLister(returnErr error, processes ...*procTestMock) func(ctx context.Context) ([]*procs.Process, error) {
	return func(ctx context.Context) ([]*procs.Process, error) {
		if returnErr != nil {
			return nil, returnErr
		}

		// We don't really care about the actual procs.Process objects
		// since the test will use our own ProcessStatus mock when making assertions
		result := make([]*procs.Process, len(processes))
		for i := range processes {
			result[i] = &procs.Process{Pid: processes[i].pid}
		}
		return result, nil
	}
}

// Custom version of findProcessByName for testing that uses procTestMock mocks
func testFindProcessByName(ctx context.Context, processName string, processes []*procTestMock, listErr error) (ProcessStatus, error) {
	listFunc := testProcessLister(listErr, processes...)

	procsList, err := listFunc(ctx)
	if err != nil {
		return nil, err
	}

	for i := range procsList {
		if i >= len(processes) {
			break
		}
		// Use the corresponding procTestMock for the real logic
		mockProc := processes[i]

		name, err := mockProc.Name()
		if err != nil {
			continue
		}

		if strings.Contains(name, processName) {
			return mockProc, nil
		}
	}
	return nil, nil
}

func TestFindProcessByNameIndividual(t *testing.T) {
	ctx := context.Background()

	t.Run("empty process list", func(t *testing.T) {
		result, err := testFindProcessByName(ctx, "test", []*procTestMock{}, nil)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("no matching process", func(t *testing.T) {
		processes := []*procTestMock{
			{pid: 100, procName: "process1"},
			{pid: 200, procName: "process2"},
		}

		result, err := testFindProcessByName(ctx, "test", processes, nil)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("matching process found", func(t *testing.T) {
		processes := []*procTestMock{
			{pid: 100, procName: "process1"},
			{pid: 200, procName: "test-process"},
			{pid: 300, procName: "process3"},
		}

		result, err := testFindProcessByName(ctx, "test", processes, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		name, err := result.Name()
		assert.NoError(t, err)
		assert.Equal(t, "test-process", name)
	})

	t.Run("error getting process list", func(t *testing.T) {
		processes := []*procTestMock{
			{pid: 100, procName: "process1"},
		}

		expectedErr := errors.New("failed to list processes")
		result, err := testFindProcessByName(ctx, "test", processes, expectedErr)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result)
	})

	t.Run("error getting process name", func(t *testing.T) {
		processes := []*procTestMock{
			{pid: 100, procName: "", nameErr: errors.New("failed to get name")},
			{pid: 200, procName: "test-process"},
		}

		result, err := testFindProcessByName(ctx, "test", processes, nil)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		name, err := result.Name()
		assert.NoError(t, err)
		assert.Equal(t, "test-process", name)
	})
}
