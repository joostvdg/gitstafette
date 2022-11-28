package v1

import (
	"fmt"
	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/labstack/echo/v4"
	"io/ioutil"
	"net/http"
	"time"
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
	isStored := false
	if cache.Repositories.RepositoryIsWatched(targetRepositoryID) {
		// TODO handle error
		isStored, _ = cache.InternalEvent(targetRepositoryID, messagePayload, headers)
	} else {
		message := fmt.Sprintf("Target %v is not a watched repository", targetRepositoryID)
		fmt.Printf(message)
		if hub := sentryecho.GetHubFromContext(ctx); hub != nil {
			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetExtra("targetRepositoryID", targetRepositoryID)
				scope.SetExtra("targetType", targetType)
				scope.SetExtra("RequestURI", ctx.Request().RequestURI)
				hub.CaptureMessage(message)
				hub.Flush(time.Second * 5)
			})
		}
		return ctx.String(http.StatusNotAcceptable, message)
	}
	if isStored {
		return ctx.String(http.StatusCreated, "Repository event cached")
	}
	return ctx.String(http.StatusNoContent, "Repository event accepted but is already cached")

}
