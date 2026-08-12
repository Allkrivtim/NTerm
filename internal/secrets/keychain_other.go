//go:build !darwin

package secrets

import "errors"

var errUnavailable = errors.New("secure password storage is not available on this platform")

func Set(string, string) error   { return errUnavailable }
func Delete(string) error        { return errUnavailable }
func Has(string) bool            { return false }
func Get(string) (string, error) { return "", errUnavailable }
