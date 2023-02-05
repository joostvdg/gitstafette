package v1

import (
	"fmt"
	"github.com/getsentry/sentry-go"
	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/joostvdg/gitstafette/internal/cache"
	gcontext "github.com/joostvdg/gitstafette/internal/context"
	"github.com/labstack/echo/v4"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

const (
	EventHeader      = "X-Github-InternalEvent"
	TargetIdHeader   = "X-Github-Hook-Installation-Target-Id"
	TargetTypeHeader = "X-Github-Hook-Installation-Target-Type"

	SignatureHeader = "X-Hub-Signature-256"
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

	// TODO validate message via webhook token & sha256 hash
	webContext := ctx.(*gcontext.GitstafetteContext)
	if webContext.WebhookHMAC != "" {
		digestHeader := ""
		if len(headers[SignatureHeader]) > 0 && headers[SignatureHeader][0] != "" {
			digestHeader = headers[SignatureHeader][0]
		}
		err := ValidateMessage(webContext.WebhookHMAC, digestHeader, messagePayload)
		if err != nil {
			message := fmt.Sprintf("Ran into an error validating the message digest: %v\n", err)
			log.Printf(message)
			if hub := sentryecho.GetHubFromContext(ctx); hub != nil {
				hub.WithScope(func(scope *sentry.Scope) {
					scope.SetExtra("SignatureHeader", digestHeader)
					scope.SetExtra("targetType", targetType)
					scope.SetExtra("RequestURI", ctx.Request().RequestURI)
					hub.CaptureMessage(message)
					hub.Flush(time.Second * 5)
				})
			}
			return ctx.String(http.StatusBadRequest, message)
		}
	} else {
		log.Printf("Warning: No HMAC set, ignoring digest")
	}

	targetRepositoryID := headers[TargetIdHeader][0]
	isStored := false
	if cache.Repositories.RepositoryIsWatched(targetRepositoryID) {
		// TODO handle error
		isStored, _ = cache.InternalEvent(targetRepositoryID, messagePayload, headers)
	} else {
		message := fmt.Sprintf("Target %v is not a watched repository", targetRepositoryID)
		log.Printf(message)
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
