//go:build darwin && mygo_local_keychain

package netease

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// The helper's executable stays identical across local app rebuilds, so
// the file-based Keychain keeps its authorization. It authenticates the
// calling process before accepting a command; cookies only travel in pipes.
func localSessionCommand(op string, value []byte) ([]byte, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	helper := filepath.Join(filepath.Dir(executable), "..", "Helpers", "MyGo Keychain")
	cmd := exec.Command(helper, op)
	cmd.Stdin = bytes.NewReader(value)
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	out, err := cmd.Output()
	if err != nil {
		if strings.TrimSpace(diagnostic.String()) == "keychain status: -25300" {
			return nil, mygo.ErrSecretNotFound
		}
		return nil, fmt.Errorf("local keychain %s: %w (%s)", op, err, strings.TrimSpace(diagnostic.String()))
	}
	return out, nil
}

func getStoredSession() ([]byte, error) { return localSessionCommand("read", nil) }
func setStoredSession(b []byte) error {
	if len(b) > mygo.MaxSecretSize {
		return fmt.Errorf("session exceeds %d bytes", mygo.MaxSecretSize)
	}
	_, err := localSessionCommand("write", b)
	return err
}
func deleteStoredSession() error {
	_, err := localSessionCommand("delete", nil)
	return err
}
