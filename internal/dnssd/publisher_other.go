//go:build !linux

package dnssd

import (
	"context"
	"errors"

	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

type Config struct {
	Identity  frprinter.Identity
	Instance  string
	Interface string
}

func Run(context.Context, Config) error {
	return errors.New("FolioRelay AirPrint DNS-SD publisher is supported only on Linux")
}
