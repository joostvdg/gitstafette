package context

import (
	"context"
	gitstafette_v1 "github.com/joostvdg/gitstafette/api/v1"
	"github.com/labstack/echo/v4"
)

type GitstafetteContext struct {
	echo.Context
	Relay *gitstafette_v1.RelayConfig
}

type ServiceContext struct {
	context.Context
	Relay *gitstafette_v1.RelayConfig
}

type Service func(*ServiceContext)
