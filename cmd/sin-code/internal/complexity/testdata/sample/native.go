package sample

import "github.com/pkg/errors"

func boom() error {
	return errors.New("boom")
}
