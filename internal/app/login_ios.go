package app

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"rsc.io/qr"
)

func init() {
	shareLoginCode = shareQRImage
}

// shareQRImage lets the listener save the QR image to scan from the same phone.
func shareQRImage(code *qr.Code) error {
	temp, err := mygo.App.Path(mygo.PathTemp)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(temp, "cloudmusic-login-")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, "cloudmusic-login.png")
	if err := os.WriteFile(file, code.PNG(), 0600); err != nil {
		os.RemoveAll(dir)
		return err
	}
	// Share returns when the sheet opens. It still needs to read the PNG
	// when the listener chooses an action.
	if err := mygo.Share(mygo.ShareOptions{Files: []string{file}}); err != nil {
		os.RemoveAll(dir)
		return err
	}
	return nil
}
