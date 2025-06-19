package nfschecker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestData_Write(t *testing.T) {
	t.Run("successful write with all fields", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test-data.json")

		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		err := data.Write(filePath)
		assert.NoError(t, err)

		// Verify file was created and contains correct JSON
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var readData Data
		err = json.Unmarshal(content, &readData)
		require.NoError(t, err)

		assert.Equal(t, data.VolumeName, readData.VolumeName)
		assert.Equal(t, data.VolumeMountPath, readData.VolumeMountPath)
		assert.Equal(t, data.FileContents, readData.FileContents)
	})

	t.Run("successful write with empty fields", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "empty-data.json")

		data := Data{
			VolumeName:      "",
			VolumeMountPath: "",
			FileContents:    "",
		}

		err := data.Write(filePath)
		assert.NoError(t, err)

		// Verify file was created and contains correct JSON
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var readData Data
		err = json.Unmarshal(content, &readData)
		require.NoError(t, err)

		assert.Equal(t, data, readData)
	})

	t.Run("successful write with special characters", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "special-chars.json")

		data := Data{
			VolumeName:      "volume-with-special_chars.123",
			VolumeMountPath: "/mnt/path with spaces/and-special_chars",
			FileContents:    "content with\nnewlines\tand\ttabs and \"quotes\" and 🚀 emoji",
		}

		err := data.Write(filePath)
		assert.NoError(t, err)

		// Verify file was created and contains correct JSON
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		var readData Data
		err = json.Unmarshal(content, &readData)
		require.NoError(t, err)

		assert.Equal(t, data, readData)
	})

	t.Run("write to non-existent directory", func(t *testing.T) {
		nonExistentPath := filepath.Join("/non-existent-dir", "test-data.json")

		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		err := data.Write(nonExistentPath)
		assert.Error(t, err)
	})

	t.Run("write to read-only directory", func(t *testing.T) {
		tempDir := t.TempDir()

		// Make directory read-only
		err := os.Chmod(tempDir, 0444)
		require.NoError(t, err)
		defer func() {
			_ = os.Chmod(tempDir, 0755) // Restore permissions for cleanup
		}()

		filePath := filepath.Join(tempDir, "readonly-test.json")

		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		err = data.Write(filePath)
		assert.Error(t, err)
	})

	t.Run("write large data", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "large-data.json")

		// Create large content (1MB)
		largeContent := string(make([]byte, 1024*1024))

		data := Data{
			VolumeName:      "large-volume",
			VolumeMountPath: "/mnt/large",
			FileContents:    largeContent,
		}

		err := data.Write(filePath)
		assert.NoError(t, err)

		// Verify the large file was written correctly
		readData, err := ReadDataFromFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, data, readData)
	})

	t.Run("write with JSON marshal error", func(t *testing.T) {
		// This is a bit tricky since Data struct has only string fields that can be marshaled
		// We need to use reflection to create an unmarshalable struct
		// For now, we'll test this indirectly by creating a data structure that would fail
		// In practice, the marshal error is very rare for Data struct, but let's test file system errors instead
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test.json")

		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		// Make the directory read-only to trigger a write error
		err := os.Chmod(tempDir, 0444)
		require.NoError(t, err)
		defer func() {
			_ = os.Chmod(tempDir, 0755) // Restore permissions for cleanup
		}()

		err = data.Write(filePath)
		assert.Error(t, err)
	})
}

func TestReadDataFromFile(t *testing.T) {
	t.Run("read new JSON format", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "json-format.json")

		originalData := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}

		// Write using the Write method
		err := originalData.Write(filePath)
		require.NoError(t, err)

		// Read back using ReadDataFromFile
		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, originalData, readData)
	})

	t.Run("read old plain text format", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "plain-text.txt")

		plainContent := "plain text content without JSON structure"
		err := os.WriteFile(filePath, []byte(plainContent), 0644)
		require.NoError(t, err)

		// Read back using ReadDataFromFile
		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)

		// Should have empty volume fields but correct file contents
		expected := Data{
			VolumeName:      "",
			VolumeMountPath: "",
			FileContents:    plainContent,
		}
		assert.Equal(t, expected, readData)
	})

	t.Run("read empty file", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "empty.txt")

		err := os.WriteFile(filePath, []byte(""), 0644)
		require.NoError(t, err)

		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)

		expected := Data{
			VolumeName:      "",
			VolumeMountPath: "",
			FileContents:    "",
		}
		assert.Equal(t, expected, readData)
	})

	t.Run("read file with invalid JSON but valid text", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "invalid-json.txt")

		invalidJSON := `{"incomplete": json without closing brace`
		err := os.WriteFile(filePath, []byte(invalidJSON), 0644)
		require.NoError(t, err)

		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)

		// Should fall back to treating it as plain text
		expected := Data{
			VolumeName:      "",
			VolumeMountPath: "",
			FileContents:    invalidJSON,
		}
		assert.Equal(t, expected, readData)
	})

	t.Run("read file with partial JSON fields", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "partial-json.json")

		partialData := Data{
			VolumeName:   "test-volume",
			FileContents: "content",
			// VolumeMountPath is missing
		}

		err := partialData.Write(filePath)
		require.NoError(t, err)

		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, partialData, readData)
	})

	t.Run("read file with extra JSON fields", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "extra-fields.json")

		// Create JSON with extra fields
		jsonWithExtra := `{
			"volume_name": "test-volume",
			"volume_mount_path": "/mnt/test",
			"file_contents": "test-content",
			"extra_field": "should be ignored",
			"another_extra": 123
		}`

		err := os.WriteFile(filePath, []byte(jsonWithExtra), 0644)
		require.NoError(t, err)

		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)

		expected := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "test-content",
		}
		assert.Equal(t, expected, readData)
	})

	t.Run("read non-existent file", func(t *testing.T) {
		nonExistentPath := "/non-existent-file.json"

		_, err := ReadDataFromFile(nonExistentPath)
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("read file with unicode and special characters", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "unicode.json")

		data := Data{
			VolumeName:      "测试卷-🚀",
			VolumeMountPath: "/mnt/тест/путь",
			FileContents:    "Content with émojis 🎉 and spëcial çhars",
		}

		err := data.Write(filePath)
		require.NoError(t, err)

		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, data, readData)
	})

	t.Run("read file with newlines and formatting in JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "formatted.json")

		data := Data{
			VolumeName:      "test-volume",
			VolumeMountPath: "/mnt/test",
			FileContents:    "line1\nline2\r\nline3\ttabbed content",
		}

		err := data.Write(filePath)
		require.NoError(t, err)

		readData, err := ReadDataFromFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, data, readData)
	})

	t.Run("read directory instead of file", func(t *testing.T) {
		tempDir := t.TempDir()

		_, err := ReadDataFromFile(tempDir)
		assert.Error(t, err)
	})
}

func TestData_RoundTrip(t *testing.T) {
	t.Run("write and read consistency", func(t *testing.T) {
		testCases := []Data{
			{
				VolumeName:      "simple-volume",
				VolumeMountPath: "/mnt/simple",
				FileContents:    "simple content",
			},
			{
				VolumeName:      "",
				VolumeMountPath: "",
				FileContents:    "",
			},
			{
				VolumeName:      "complex-volume_name.123",
				VolumeMountPath: "/mnt/complex path/with spaces",
				FileContents:    "Complex content with\nmultiple\nlines and special chars: !@#$%^&*()",
			},
			{
				VolumeName:      "unicode-测试",
				VolumeMountPath: "/mnt/юникод/путь",
				FileContents:    "Unicode content: 🌟 Testing émojis and spëcial characters",
			},
		}

		for i, originalData := range testCases {
			t.Run(fmt.Sprintf("test_case_%d", i), func(t *testing.T) {
				tempDir := t.TempDir()
				filePath := filepath.Join(tempDir, fmt.Sprintf("roundtrip_%d.json", i))

				// Write data
				err := originalData.Write(filePath)
				require.NoError(t, err)

				// Read data back
				readData, err := ReadDataFromFile(filePath)
				require.NoError(t, err)

				// Verify they match exactly
				assert.Equal(t, originalData, readData)
			})
		}
	})
}
