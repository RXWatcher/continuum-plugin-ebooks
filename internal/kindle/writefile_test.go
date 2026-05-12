package kindle

import "os"

func writeFileImpl(path string, b []byte) error {
	return os.WriteFile(path, b, 0o600)
}
