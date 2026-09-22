package bootstrapv4

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// loadedImage is the executable file object this process was started from,
// captured before protected code can run and held open for the process
// lifetime. It is deliberately never closed.
type loadedImage struct {
	file *os.File
	info os.FileInfo
	err  error
}

var startupImage = captureLoadedImage()

func captureLoadedImage() loadedImage {
	path := ""
	if runtime.GOOS == "linux" {
		// The kernel's own reference to the executing image, independent of
		// any pathname.
		path = "/proc/self/exe"
	} else {
		executable, err := os.Executable()
		if err != nil {
			return loadedImage{err: fmt.Errorf("resolve running executable: %w", err)}
		}
		resolved, err := filepath.EvalSymlinks(executable)
		if err != nil {
			return loadedImage{err: fmt.Errorf("resolve running executable symlinks: %w", err)}
		}
		path = resolved
	}
	file, err := os.Open(path)
	if err != nil {
		return loadedImage{err: fmt.Errorf("open running executable image: %w", err)}
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return loadedImage{err: fmt.Errorf("stat running executable image: %w", err)}
	}
	return loadedImage{file: file, info: info}
}

func processImage() (loadedImage, error) {
	if startupImage.err != nil {
		return loadedImage{}, startupImage.err
	}
	if startupImage.file == nil || startupImage.info == nil {
		return loadedImage{}, errors.New("running executable image was not captured at process start")
	}
	return startupImage, nil
}

// read returns the bytes of the captured file object with positional reads, so
// concurrent verifications never disturb one another. One read feeds both the
// manifest digest and the loaded-code binding, so the two always describe the
// same bytes. Nothing is cached: an identity that is reused cannot notice a
// later change.
func (i loadedImage) read() ([]byte, error) {
	data := make([]byte, i.info.Size())
	if _, err := io.ReadFull(io.NewSectionReader(i.file, 0, i.info.Size()), data); err != nil {
		return nil, fmt.Errorf("read running executable image: %w", err)
	}
	// A rewrite that changes length is visible in the size the descriptor now
	// reports. A same-length in-place rewrite is not, which is why the digest
	// alone never establishes what this process is executing (see
	// bindExecutingCode).
	current, err := i.file.Stat()
	if err != nil || current.Size() != i.info.Size() {
		return nil, errors.New("running executable image changed size after process start")
	}
	return data, nil
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
