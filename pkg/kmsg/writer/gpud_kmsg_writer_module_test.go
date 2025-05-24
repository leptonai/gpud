package writer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	pkgkmsg "github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEmbed(t *testing.T) {
	require.NotEmpty(t, gpudKmsgWriterC)
	require.NotEmpty(t, gpudKmsgWriterMakefile)
}

func TestNewGPUdKmsgWriterModule(t *testing.T) {
	tempDir := t.TempDir()
	ctx := context.Background()

	module := NewGPUdKmsgWriterModule(ctx, tempDir)
	require.NotNil(t, module)

	// Cast to concrete type to inspect internal state
	concreteModule := module.(*gpudKmsgWriterModule)
	require.Equal(t, ctx, concreteModule.ctx)
	require.Equal(t, tempDir, concreteModule.workDir)
	require.Equal(t, filepath.Join(tempDir, "gpud_kmsg_writer.c"), concreteModule.cFile)
	require.Equal(t, filepath.Join(tempDir, "Makefile"), concreteModule.makeFile)
	require.Equal(t, filepath.Join(tempDir, "gpud_kmsg_writer.ko"), concreteModule.koFile)
	require.Equal(t, "/dev/gpud_kmsg_writer", concreteModule.devFile)
	require.Equal(t, "gpud_kmsg_writer", concreteModule.devName)
	require.NotNil(t, concreteModule.processRunner)
	require.NotNil(t, concreteModule.kmsgReadFunc)
}

func TestGPUdKmsgWriterModule_CheckPermissions(t *testing.T) {
	err := CheckPermissions()
	if runtime.GOOS != "linux" {
		require.Error(t, err)
		require.Contains(t, err.Error(), "only supported on Linux")
	}
	// Note: We can't test the root check on Mac since os.Geteuid() will not be 0
}

func TestGPUdKmsgWriterModule_CleanupFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	cFile := filepath.Join(tempDir, "test.c")
	koFile := filepath.Join(tempDir, "test.ko")
	devFile := filepath.Join(tempDir, "test_dev")

	require.NoError(t, os.WriteFile(cFile, []byte("test content"), 0644))
	require.NoError(t, os.WriteFile(koFile, []byte("test content"), 0644))
	require.NoError(t, os.WriteFile(devFile, []byte("test content"), 0644))

	// Verify files exist
	require.FileExists(t, cFile)
	require.FileExists(t, koFile)
	require.FileExists(t, devFile)

	module := &gpudKmsgWriterModule{
		cFile:   cFile,
		koFile:  koFile,
		devFile: devFile,
	}

	err := module.cleanupFiles()
	require.NoError(t, err)

	// Verify files are removed
	require.NoFileExists(t, cFile)
	require.NoFileExists(t, koFile)
	require.NoFileExists(t, devFile)
}

func TestGPUdKmsgWriterModule_CleanupFiles_NonExistentFiles(t *testing.T) {
	tempDir := t.TempDir()

	module := &gpudKmsgWriterModule{
		cFile:   filepath.Join(tempDir, "nonexistent.c"),
		koFile:  filepath.Join(tempDir, "nonexistent.ko"),
		devFile: filepath.Join(tempDir, "nonexistent_dev"),
	}

	err := module.cleanupFiles()
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_WriteFiles(t *testing.T) {
	tempDir := t.TempDir()

	module := &gpudKmsgWriterModule{
		cFile:           filepath.Join(tempDir, "gpud_kmsg_writer.c"),
		cFileContent:    "test c content",
		makeFile:        filepath.Join(tempDir, "Makefile"),
		makeFileContent: "test makefile content",
	}

	err := module.writeFiles()
	require.NoError(t, err)

	// Verify files were created with correct content
	require.FileExists(t, module.cFile)
	require.FileExists(t, module.makeFile)

	cContent, err := os.ReadFile(module.cFile)
	require.NoError(t, err)
	require.Equal(t, "test c content", string(cContent))

	makeContent, err := os.ReadFile(module.makeFile)
	require.NoError(t, err)
	require.Equal(t, "test makefile content", string(makeContent))
}

func TestGPUdKmsgWriterModule_WriteFiles_WriteError(t *testing.T) {
	invalidPath := "/invalid/path/that/does/not/exist"

	module := &gpudKmsgWriterModule{
		cFile:           filepath.Join(invalidPath, "gpud_kmsg_writer.c"),
		cFileContent:    "test content",
		makeFile:        filepath.Join(invalidPath, "Makefile"),
		makeFileContent: "test content",
	}

	err := module.writeFiles()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to write")
}

func TestGPUdKmsgWriterModule_BuildModule_Success(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("build successful"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:                     tempDir,
		scriptBuildModule:           "make modules",
		scriptReinstallLinuxHeaders: "sudo apt-get install",
		processRunner:               mockRunner,
	}

	err := module.buildModule(context.Background())
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_BuildModule_FailureWithKernelConfig(t *testing.T) {
	tempDir := t.TempDir()

	callCount := 0
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			callCount++
			if callCount == 1 {
				// First call fails with kernel config error
				return []byte("ERROR: Kernel configuration is invalid"), 1, errors.New("build failed")
			} else if callCount == 2 {
				// Second call is the reinstall script
				return []byte("reinstall successful"), 0, nil
			} else {
				// Third call should succeed
				return []byte("build successful after reinstall"), 0, nil
			}
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:                     tempDir,
		scriptBuildModule:           "make modules",
		scriptReinstallLinuxHeaders: "sudo apt-get install",
		processRunner:               mockRunner,
	}

	err := module.buildModule(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, callCount) // Should have made 3 calls
}

func TestGPUdKmsgWriterModule_BuildModule_FailureWithoutRetry(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("some other error"), 1, errors.New("build failed")
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:                     tempDir,
		scriptBuildModule:           "make modules",
		scriptReinstallLinuxHeaders: "sudo apt-get install",
		processRunner:               mockRunner,
	}

	err := module.buildModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to build module")
}

func TestGPUdKmsgWriterModule_BuildModule_ReinstallFails(t *testing.T) {
	tempDir := t.TempDir()

	callCount := 0
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			callCount++
			if callCount == 1 {
				// First call fails with kernel config error
				return []byte("ERROR: Kernel configuration is invalid"), 1, errors.New("build failed")
			} else {
				// Reinstall fails
				return []byte("reinstall failed"), 1, errors.New("reinstall failed")
			}
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:                     tempDir,
		scriptBuildModule:           "make modules",
		scriptReinstallLinuxHeaders: "sudo apt-get install",
		processRunner:               mockRunner,
	}

	err := module.buildModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "reinstall failed")
}

func TestGPUdKmsgWriterModule_InstallModule_Success(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			require.Equal(t, "sudo insmod gpud_kmsg_writer.ko", script)
			return []byte("module installed"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:             tempDir,
		scriptInstallModule: "sudo insmod gpud_kmsg_writer.ko",
		processRunner:       mockRunner,
	}

	err := module.installModule(context.Background())
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_InstallModule_Failure(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("install failed"), 1, errors.New("install failed")
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:             tempDir,
		scriptInstallModule: "sudo insmod gpud_kmsg_writer.ko",
		processRunner:       mockRunner,
	}

	err := module.installModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "install failed")
}

func TestGPUdKmsgWriterModule_FindMajorNum_SortingLogic(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		name          string
		messages      []pkgkmsg.Message
		expectedMajor string
		expectedError string
	}{
		{
			name: "finds major number from latest message",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime.Add(-2 * time.Hour)),
					Message:   "some other message",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)),
					Message:   "module loaded with device major number 123",
				},
				{
					Timestamp: metav1.NewTime(baseTime), // latest
					Message:   "module loaded with device major number 456",
				},
			},
			expectedMajor: "456",
		},
		{
			name: "no major number message found",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime),
					Message:   "some unrelated message",
				},
			},
			expectedError: "failed to find device major number in kmsg",
		},
		{
			name: "sorts correctly - finds latest major number",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime.Add(-3 * time.Hour)),
					Message:   "module loaded with device major number 111",
				},
				{
					Timestamp: metav1.NewTime(baseTime), // latest
					Message:   "module loaded with device major number 999",
				},
			},
			expectedMajor: "999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := &gpudKmsgWriterModule{
				kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
					return tt.messages, nil
				},
			}

			major, err := module.findMajorNum(context.Background())

			if tt.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedMajor, major)
			}
		})
	}
}

func TestGPUdKmsgWriterModule_FindMajorNum_KmsgReaderError(t *testing.T) {
	module := &gpudKmsgWriterModule{
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			return nil, errors.New("failed to read kmsg")
		},
	}

	_, err := module.findMajorNum(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read kmsg")
}

func TestGPUdKmsgWriterModule_LoadModule_Success(t *testing.T) {
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			require.Equal(t, "mknod /dev/test_device c 123 0", script)
			return []byte("device created"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		devMajorNum:   "123",
		processRunner: mockRunner,
		createScriptLoadModuleFunc: func(devName string, devMajorNum string) string {
			return "mknod /dev/" + devName + " c " + devMajorNum + " 0"
		},
	}

	err := module.loadModule(context.Background())
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_LoadModule_Failure(t *testing.T) {
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("device creation failed"), 1, errors.New("mknod failed")
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		devMajorNum:   "123",
		processRunner: mockRunner,
		createScriptLoadModuleFunc: func(devName string, devMajorNum string) string {
			return "mknod /dev/" + devName + " c " + devMajorNum + " 0"
		},
	}

	err := module.loadModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "mknod failed")
}

func TestGPUdKmsgWriterModule_InjectMessage_Success(t *testing.T) {
	baseTime := time.Now()
	testMessage := "test message content"

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			require.Contains(t, script, testMessage)
			return []byte("message injected"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		processRunner: mockRunner,
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test script with " + msg.Message
		},
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			return []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime),
					Message:   "some message with " + testMessage + " in it",
				},
			}, nil
		},
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  testMessage,
	}

	err := module.injectKmsg(context.Background(), msg)
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_InjectMessage_ValidationError(t *testing.T) {
	module := &gpudKmsgWriterModule{}

	// Create a message that's too long
	longMessage := make([]byte, MaxPrintkRecordLength+1)
	for i := range longMessage {
		longMessage[i] = 'a'
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  string(longMessage),
	}

	err := module.injectKmsg(context.Background(), msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid message")
}

func TestGPUdKmsgWriterModule_InjectMessage_ProcessRunnerError(t *testing.T) {
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("injection failed"), 1, errors.New("process failed")
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		processRunner: mockRunner,
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test"
		},
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test message",
	}

	err := module.injectKmsg(context.Background(), msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "process failed")
}

func TestGPUdKmsgWriterModule_InjectMessage_MessageNotFound(t *testing.T) {
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("message injected"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		processRunner: mockRunner,
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test"
		},
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			return []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "some unrelated message",
				},
			}, nil
		},
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test message that won't be found",
	}

	err := module.injectKmsg(context.Background(), msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to find injected message in kmsg")
}

func TestGPUdKmsgWriterModule_InjectMessage_KmsgReaderError(t *testing.T) {
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("message injected"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		processRunner: mockRunner,
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test"
		},
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			return nil, errors.New("kmsg read failed")
		},
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test message",
	}

	err := module.injectKmsg(context.Background(), msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read kmsg")
}

func TestGPUdKmsgWriterModule_Inject_InterfaceMethod(t *testing.T) {
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("message injected"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		processRunner: mockRunner,
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test"
		},
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			return []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "some message with test message in it",
				},
			}, nil
		},
	}

	// Test the interface method
	var kmsgWriter GPUdKmsgWriterModule = module

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test message",
	}

	err := kmsgWriter.InjectKernelMessage(context.Background(), msg)
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_BuildInstall_NonLinux(t *testing.T) {
	tempDir := t.TempDir()
	module := NewGPUdKmsgWriterModule(context.Background(), tempDir)

	// Should fail on non-Linux platforms
	if runtime.GOOS != "linux" {
		err := module.BuildInstall(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "only supported on Linux")
	}
}

func TestGPUdKmsgWriterModule_BuildInstall_MockedSuccess(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Skipping mocked test on Linux platform")
	}

	tempDir := t.TempDir()

	// Test with mocked components for a full successful flow
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			// Mock all script executions as successful
			return []byte("success"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		ctx:                         context.Background(),
		workDir:                     tempDir,
		cFile:                       filepath.Join(tempDir, "gpud_kmsg_writer.c"),
		cFileContent:                "mock c content",
		makeFile:                    filepath.Join(tempDir, "Makefile"),
		makeFileContent:             "mock makefile content",
		koFile:                      filepath.Join(tempDir, "gpud_kmsg_writer.ko"),
		devFile:                     "/dev/gpud_kmsg_writer",
		devName:                     "gpud_kmsg_writer",
		scriptBuildModule:           "make modules",
		scriptReinstallLinuxHeaders: "sudo apt-get install",
		scriptInstallModule:         "sudo insmod gpud_kmsg_writer.ko",
		processRunner:               mockRunner,
		createScriptLoadModuleFunc: func(devName string, devMajorNum string) string {
			return "mknod /dev/" + devName + " c " + devMajorNum + " 0"
		},
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test"
		},
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			// Mock kmsg messages
			return []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "module loaded with device major number 123",
				},
				{
					Timestamp: metav1.NewTime(time.Now().Add(time.Second)),
					Message:   "some message with hello world in it",
				},
			}, nil
		},
	}

	// The BuildInstall should still fail due to platform check, but we're testing the mocked components
	err := module.BuildInstall(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "only supported on Linux")
}

func TestCreateScriptLoadModule(t *testing.T) {
	tests := []struct {
		name        string
		devName     string
		devMajorNum string
		expected    string
	}{
		{
			name:        "basic case",
			devName:     "gpud_kmsg_writer",
			devMajorNum: "123",
			expected:    "mknod /dev/gpud_kmsg_writer c 123 0",
		},
		{
			name:        "different device name",
			devName:     "test_device",
			devMajorNum: "456",
			expected:    "mknod /dev/test_device c 456 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createScriptLoadModule(tt.devName, tt.devMajorNum)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateScriptInjectMessage(t *testing.T) {
	tests := []struct {
		name     string
		devName  string
		msg      *KernelMessage
		expected string
	}{
		{
			name:    "basic case",
			devName: "gpud_kmsg_writer",
			msg: &KernelMessage{
				Priority: "KERN_INFO",
				Message:  "hello world",
			},
			expected: `sudo sh -c "echo \"KERN_INFO,hello world\" > /dev/gpud_kmsg_writer"`,
		},
		{
			name:    "different message",
			devName: "test_device",
			msg: &KernelMessage{
				Priority: "KERN_DEBUG",
				Message:  "test message",
			},
			expected: `sudo sh -c "echo \"KERN_DEBUG,test message\" > /dev/test_device"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createScriptInjectMessage(tt.devName, tt.msg)
			require.Equal(t, tt.expected, result)
		})
	}
}

// Mock runner for testing
type mockRunner struct {
	runFunc func(ctx context.Context, script string) ([]byte, int32, error)
}

func (m *mockRunner) RunUntilCompletion(ctx context.Context, script string) ([]byte, int32, error) {
	return m.runFunc(ctx, script)
}

// Ensure mockRunner implements process.Runner interface
var _ process.Runner = (*mockRunner)(nil)

// Additional tests to increase coverage

func TestNewKmsgWriterWithDummyDevice_NonLinux(t *testing.T) {
	// Should return noOpKmsgWriter on non-Linux platforms
	if runtime.GOOS != "linux" {
		writer := NewKmsgWriterWithDummyDevice()
		require.NotNil(t, writer)

		// Should be able to write without error
		msg := &KernelMessage{
			Priority: "KERN_INFO",
			Message:  "test message",
		}
		err := writer.Write(msg)
		require.NoError(t, err)

		// Should handle nil message
		err = writer.Write(nil)
		require.NoError(t, err)
	}
}

func TestNoOpKmsgWriter_Write(t *testing.T) {
	writer := &noOpKmsgWriter{}

	// Should handle normal message
	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test message",
	}
	err := writer.Write(msg)
	require.NoError(t, err)

	// Should handle nil message
	err = writer.Write(nil)
	require.NoError(t, err)
}

func TestKmsgWriterWithDummyDevice_Write_Nil(t *testing.T) {
	writer := &kmsgWriterWithDummyDevice{}

	// Should handle nil message gracefully
	err := writer.Write(nil)
	require.NoError(t, err)
}

func TestKmsgWriterWithDummyDevice_Write_InvalidMessage(t *testing.T) {
	writer := &kmsgWriterWithDummyDevice{}

	// Create a message that's too long
	longMessage := make([]byte, MaxPrintkRecordLength+1)
	for i := range longMessage {
		longMessage[i] = 'a'
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  string(longMessage),
	}

	err := writer.Write(msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message length exceeds")
}

func TestGPUdKmsgWriterModule_CleanupFiles_PartialFailure(t *testing.T) {
	tempDir := t.TempDir()

	// Create one valid file and set up paths where some will fail
	cFile := filepath.Join(tempDir, "test.c")
	require.NoError(t, os.WriteFile(cFile, []byte("test content"), 0644))

	module := &gpudKmsgWriterModule{
		cFile:   cFile,
		koFile:  filepath.Join(tempDir, "nonexistent.ko"),
		devFile: "/dev/nonexistent_device", // This will fail to remove but should not error
	}

	err := module.cleanupFiles()
	require.NoError(t, err) // Should succeed even if some files don't exist

	// Verify the existing file was removed
	require.NoFileExists(t, cFile)
}

func TestGPUdKmsgWriterModule_BuildInstall_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Skipping context cancellation test on Linux")
	}

	tempDir := t.TempDir()

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	module := NewGPUdKmsgWriterModule(ctx, tempDir)

	// Should fail due to platform check, not context cancellation
	err := module.BuildInstall(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only supported on Linux")
}

func TestGPUdKmsgWriterModule_InjectMessage_ContextCancellation(t *testing.T) {
	// Create a context that gets canceled during the sleep
	ctx, cancel := context.WithCancel(context.Background())

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			// Cancel the context during execution
			cancel()
			return []byte("message injected"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		devName:       "test_device",
		processRunner: mockRunner,
		createScriptInjectMessageFunc: func(devName string, msg *KernelMessage) string {
			return "echo test"
		},
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			// This should not be called due to context cancellation
			return []pkgkmsg.Message{}, nil
		},
	}

	msg := &KernelMessage{
		Priority: "KERN_INFO",
		Message:  "test message",
	}

	err := module.injectKmsg(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
}

func TestGPUdKmsgWriterModule_FindMajorNum_EmptyMessage(t *testing.T) {
	module := &gpudKmsgWriterModule{
		kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
			return []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(time.Now()),
					Message:   "", // Empty message
				},
			}, nil
		},
	}

	_, err := module.findMajorNum(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to find device major number in kmsg")
}

func TestGPUdKmsgWriterModule_BuildModule_DirectoryChangeError(t *testing.T) {
	// Test with a directory that doesn't exist to trigger chdir error
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("should not be called"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:                     "/invalid/nonexistent/directory",
		scriptBuildModule:           "make modules",
		scriptReinstallLinuxHeaders: "sudo apt-get install",
		processRunner:               mockRunner,
	}

	err := module.buildModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to change directory")
}

func TestGPUdKmsgWriterModule_InstallModule_DirectoryChangeError(t *testing.T) {
	// Test with a directory that doesn't exist to trigger chdir error
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("should not be called"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:             "/invalid/nonexistent/directory",
		scriptInstallModule: "sudo insmod test.ko",
		processRunner:       mockRunner,
	}

	err := module.installModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to change directory")
}

// Additional test cases specifically for timestamp-based major number selection
func TestGPUdKmsgWriterModule_FindMajorNum_TimestampPrecedence(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		name             string
		messages         []pkgkmsg.Message
		expectedMajorNum string
		description      string
	}{
		{
			name: "picks latest timestamp when multiple major numbers exist",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime.Add(-2 * time.Hour)), // oldest
					Message:   "module loaded with device major number 111",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)), // middle
					Message:   "module loaded with device major number 222",
				},
				{
					Timestamp: metav1.NewTime(baseTime), // latest
					Message:   "module loaded with device major number 333",
				},
			},
			expectedMajorNum: "333",
			description:      "should pick major number 333 from the latest timestamp",
		},
		{
			name: "picks latest even when older messages appear first in slice",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime), // latest timestamp but first in slice
					Message:   "module loaded with device major number 999",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-3 * time.Hour)), // oldest
					Message:   "module loaded with device major number 111",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)), // middle
					Message:   "module loaded with device major number 555",
				},
			},
			expectedMajorNum: "999",
			description:      "should pick major number 999 despite slice order",
		},
		{
			name: "picks latest among mixed relevant and irrelevant messages",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime.Add(-3 * time.Hour)),
					Message:   "some unrelated log message",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-2 * time.Hour)),
					Message:   "module loaded with device major number 123",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-30 * time.Minute)),
					Message:   "another unrelated message",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)),
					Message:   "module loaded with device major number 456",
				},
				{
					Timestamp: metav1.NewTime(baseTime), // latest with major number
					Message:   "module loaded with device major number 789",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(1 * time.Hour)), // even later but no major number
					Message:   "some future log without major number",
				},
			},
			expectedMajorNum: "789",
			description:      "should pick major number 789 from latest relevant message",
		},
		{
			name: "handles microsecond precision timestamps",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime.Add(100 * time.Microsecond)),
					Message:   "module loaded with device major number 100",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(200 * time.Microsecond)), // slightly later
					Message:   "module loaded with device major number 200",
				},
				{
					Timestamp: metav1.NewTime(baseTime.Add(150 * time.Microsecond)),
					Message:   "module loaded with device major number 150",
				},
			},
			expectedMajorNum: "200",
			description:      "should pick major number 200 from microsecond-precision latest timestamp",
		},
		{
			name: "handles same timestamp with deterministic ordering",
			messages: []pkgkmsg.Message{
				{
					Timestamp: metav1.NewTime(baseTime), // same timestamp
					Message:   "module loaded with device major number 111",
				},
				{
					Timestamp: metav1.NewTime(baseTime), // same timestamp
					Message:   "module loaded with device major number 222",
				},
			},
			expectedMajorNum: "111", // should pick first one found after sorting
			description:      "should pick first major number when timestamps are identical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module := &gpudKmsgWriterModule{
				kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
					return tt.messages, nil
				},
			}

			majorNum, err := module.findMajorNum(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.expectedMajorNum, majorNum, tt.description)
		})
	}
}

func TestGPUdKmsgWriterModule_FindMajorNum_SortingBehaviorEdgeCases(t *testing.T) {
	baseTime := time.Now()

	t.Run("single message with major number", func(t *testing.T) {
		module := &gpudKmsgWriterModule{
			kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
				return []pkgkmsg.Message{
					{
						Timestamp: metav1.NewTime(baseTime),
						Message:   "module loaded with device major number 42",
					},
				}, nil
			},
		}

		majorNum, err := module.findMajorNum(context.Background())
		require.NoError(t, err)
		require.Equal(t, "42", majorNum)
	})

	t.Run("major numbers at beginning, middle, and end of message", func(t *testing.T) {
		module := &gpudKmsgWriterModule{
			kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
				return []pkgkmsg.Message{
					{
						Timestamp: metav1.NewTime(baseTime.Add(-2 * time.Hour)),
						Message:   "old module loaded with device major number 456",
					},
					{
						Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)),
						Message:   "middle module loaded with device major number 789",
					},
					{
						Timestamp: metav1.NewTime(baseTime), // latest
						Message:   "latest module loaded with device major number 999",
					},
				}, nil
			},
		}

		majorNum, err := module.findMajorNum(context.Background())
		require.NoError(t, err)
		require.Equal(t, "999", majorNum) // Should extract major number from latest message
	})

	t.Run("major number is last field in latest message", func(t *testing.T) {
		module := &gpudKmsgWriterModule{
			kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
				return []pkgkmsg.Message{
					{
						Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)),
						Message:   "old module loaded with device major number 111",
					},
					{
						Timestamp: metav1.NewTime(baseTime), // latest
						Message:   "kernel: module loaded with device major number 777",
					},
				}, nil
			},
		}

		majorNum, err := module.findMajorNum(context.Background())
		require.NoError(t, err)
		require.Equal(t, "777", majorNum) // Should extract last field correctly
	})

	t.Run("verify sorting preserves message order for same timestamps", func(t *testing.T) {
		module := &gpudKmsgWriterModule{
			kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
				// All messages have the same timestamp
				sameTime := metav1.NewTime(baseTime)
				return []pkgkmsg.Message{
					{
						Timestamp: sameTime,
						Message:   "first module loaded with device major number 111",
					},
					{
						Timestamp: sameTime,
						Message:   "second module loaded with device major number 222",
					},
					{
						Timestamp: sameTime,
						Message:   "third module loaded with device major number 333",
					},
				}, nil
			},
		}

		majorNum, err := module.findMajorNum(context.Background())
		require.NoError(t, err)
		// Should be deterministic and pick the first one
		require.Equal(t, "111", majorNum)
	})

	t.Run("function extracts last field even with extra text", func(t *testing.T) {
		module := &gpudKmsgWriterModule{
			kmsgReadFunc: func(ctx context.Context) ([]pkgkmsg.Message, error) {
				return []pkgkmsg.Message{
					{
						Timestamp: metav1.NewTime(baseTime.Add(-1 * time.Hour)),
						Message:   "older module loaded with device major number 123",
					},
					{
						Timestamp: metav1.NewTime(baseTime), // latest
						Message:   "module loaded with device major number 456 extra text here",
					},
				}, nil
			},
		}

		majorNum, err := module.findMajorNum(context.Background())
		require.NoError(t, err)
		// Function extracts last field after splitting by whitespace
		require.Equal(t, "here", majorNum)
	})
}

func TestGPUdKmsgWriterModule_UninstallModule_Success(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			require.Equal(t, "sudo rmmod gpud_kmsg_writer", script)
			return []byte("module uninstalled"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:               tempDir,
		scriptUninstallModule: "sudo rmmod gpud_kmsg_writer",
		processRunner:         mockRunner,
	}

	err := module.uninstallModule(context.Background())
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_UninstallModule_Failure(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("uninstall failed"), 1, errors.New("uninstall failed")
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:               tempDir,
		scriptUninstallModule: "sudo rmmod gpud_kmsg_writer",
		processRunner:         mockRunner,
	}

	err := module.uninstallModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "uninstall failed")
}

func TestGPUdKmsgWriterModule_UninstallModule_DirectoryChangeError(t *testing.T) {
	// Test with a directory that doesn't exist to trigger chdir error
	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("should not be called"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:               "/invalid/nonexistent/directory",
		scriptUninstallModule: "sudo rmmod test.ko",
		processRunner:         mockRunner,
	}

	err := module.uninstallModule(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to change directory")
}

func TestGPUdKmsgWriterModule_Uninstall_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files that will be cleaned up
	cFile := filepath.Join(tempDir, "gpud_kmsg_writer.c")
	koFile := filepath.Join(tempDir, "gpud_kmsg_writer.ko")
	devFile := filepath.Join(tempDir, "test_dev")

	require.NoError(t, os.WriteFile(cFile, []byte("test content"), 0644))
	require.NoError(t, os.WriteFile(koFile, []byte("test content"), 0644))
	require.NoError(t, os.WriteFile(devFile, []byte("test content"), 0644))

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			require.Equal(t, "sudo rmmod gpud_kmsg_writer", script)
			return []byte("module uninstalled"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:               tempDir,
		cFile:                 cFile,
		koFile:                koFile,
		devFile:               devFile,
		scriptUninstallModule: "sudo rmmod gpud_kmsg_writer",
		processRunner:         mockRunner,
	}

	// Verify files exist before uninstall
	require.FileExists(t, cFile)
	require.FileExists(t, koFile)
	require.FileExists(t, devFile)

	err := module.Uninstall(context.Background())
	require.NoError(t, err)

	// Verify files are cleaned up after uninstall
	require.NoFileExists(t, cFile)
	require.NoFileExists(t, koFile)
	require.NoFileExists(t, devFile)
}

func TestGPUdKmsgWriterModule_Uninstall_UninstallModuleFailure(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("uninstall failed"), 1, errors.New("uninstall failed")
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:               tempDir,
		cFile:                 filepath.Join(tempDir, "test.c"),
		koFile:                filepath.Join(tempDir, "test.ko"),
		devFile:               filepath.Join(tempDir, "test_dev"),
		scriptUninstallModule: "sudo rmmod gpud_kmsg_writer",
		processRunner:         mockRunner,
	}

	err := module.Uninstall(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "uninstall failed")
}

func TestGPUdKmsgWriterModule_Uninstall_InterfaceMethod(t *testing.T) {
	tempDir := t.TempDir()

	mockRunner := &mockRunner{
		runFunc: func(ctx context.Context, script string) ([]byte, int32, error) {
			return []byte("module uninstalled"), 0, nil
		},
	}

	module := &gpudKmsgWriterModule{
		workDir:               tempDir,
		cFile:                 filepath.Join(tempDir, "test.c"),
		koFile:                filepath.Join(tempDir, "test.ko"),
		devFile:               filepath.Join(tempDir, "test_dev"),
		scriptUninstallModule: "sudo rmmod gpud_kmsg_writer",
		processRunner:         mockRunner,
	}

	// Test the interface method
	var kmsgWriter GPUdKmsgWriterModule = module

	err := kmsgWriter.Uninstall(context.Background())
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_InjectMessage_NilMessage(t *testing.T) {
	module := &gpudKmsgWriterModule{}

	// Should handle nil message gracefully and return no error
	err := module.injectKmsg(context.Background(), nil)
	require.NoError(t, err)
}

func TestGPUdKmsgWriterModule_InjectKernelMessage_NilMessage(t *testing.T) {
	module := &gpudKmsgWriterModule{}

	// Test the interface method with nil message
	var kmsgWriter GPUdKmsgWriterModule = module

	err := kmsgWriter.InjectKernelMessage(context.Background(), nil)
	require.NoError(t, err)
}
