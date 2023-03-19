package cache

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var sublogger zerolog.Logger

func init() {
	sublogger = log.With().Str("component", "cache").Logger()
}
