package v1

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var sublogger zerolog.Logger

func init() {
	sublogger = log.With().Str("component", "api").Logger()
}
