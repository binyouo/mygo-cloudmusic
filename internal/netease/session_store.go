//go:build !darwin || !mygo_local_keychain

package netease

import "github.com/egoist/mygo"

func getStoredSession() ([]byte, error) { return mygo.Secrets.Get(sessionSecret) }
func setStoredSession(b []byte) error   { return mygo.Secrets.Set(sessionSecret, b) }
func deleteStoredSession() error        { return mygo.Secrets.Delete(sessionSecret) }
