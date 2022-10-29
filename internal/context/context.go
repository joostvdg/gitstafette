package context

import (
	"context"
	"github.com/labstack/echo/v4"
	"net/url"
)

type GitstafetteContext struct {
	echo.Context
	RelayEndpoint *url.URL
}

type ServiceContext struct {
	context.Context
	RelayEndpoint *url.URL
}

type Service func(*ServiceContext)
