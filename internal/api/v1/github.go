package v1

import (
	"fmt"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/labstack/echo/v4"
	"io/ioutil"
	"net/http"
)

const (
	EventHeader      = "X-Github-InternalEvent"
	TargetIdHeader   = "X-Github-Hook-Installation-Target-Id"
	TargetTypeHeader = "X-Github-Hook-Installation-Target-Type"
)

func HandleGitHubPost(ctx echo.Context) error {
	body := ctx.Request().Body
	defer body.Close()
	messagePayload, err := ioutil.ReadAll(body)
	if err != nil {
		fmt.Printf("Ran into an error parsing content (Assumed GitHub Post): %v\n", err)
	}

	headers := ctx.Request().Header
	targetType := headers[TargetTypeHeader]
	if len(headers) <= 0 || len(headers[TargetIdHeader]) <= 0 || targetType[0] != "repository" {
		return ctx.String(http.StatusNotAcceptable, "InternalEvent is not for a repository")
	}

	targetRepositoryID := headers[TargetIdHeader][0]
	if cache.Repositories.RepositoryIsWatched(targetRepositoryID) {
		cache.InternalEvent(targetRepositoryID, messagePayload, headers)
	} else {
		fmt.Printf("Target %v is not a watched repository", targetRepositoryID)
		return ctx.String(http.StatusNotAcceptable, "InternalEvent does not contain a watched repository")
	}
	return ctx.String(http.StatusOK, "Repository event cached")
}
