package diagnose

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
)

func copyFile(sourceFile, targetFile string) error {
	srcFile, err := os.Open(sourceFile)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}

	return nil
}

func tarDirectory(sourceDir, targetFile string) error {
	tarFile, err := os.Create(targetFile)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	tarWriter := tar.NewWriter(tarFile)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// create a new header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// update the name to correctly reflect the path inside the archive
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)

		// set mode to a default if it's zero (which can happen on some systems)
		if header.Mode == 0 {
			if info.IsDir() {
				header.Mode = 0755
			} else {
				header.Mode = 0644
			}
		}

		// write the header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// if not a regular file, we're done
		if !info.Mode().IsRegular() {
			return nil
		}

		// open and copy the file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})
}

func truncateKeepEnd(filename string, size int64) error {
	f, err := os.OpenFile(filename, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	fInfo, err := f.Stat()
	if err != nil {
		return err
	}

	if fInfo.Size() <= size {
		return nil // File is already smaller than or equal to the desired size
	}

	// Create a temporary buffer to store the last 'size' bytes
	buf := make([]byte, size)
	_, err = f.ReadAt(buf, fInfo.Size()-size)
	if err != nil {
		return err
	}

	// Truncate the file to zero length
	err = f.Truncate(0)
	if err != nil {
		return err
	}

	// Write the last 'size' bytes back to the file
	_, err = f.Write(buf)
	if err != nil {
		return err
	}

	return nil
}
