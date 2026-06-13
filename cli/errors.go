package cli

import (
	"errors"

	"github.com/tamnd/npmjs-cli/npmjs"
)

func isNotFound(err error) bool {
	return errors.Is(err, npmjs.ErrNotFound)
}
