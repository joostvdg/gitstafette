package v1

import (
	"fmt"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/joostvdg/gitstafette/internal/context"
	"github.com/labstack/echo/v4"
	"io/ioutil"
	"net/http"
)

const (
	EventHeader      = "X-Github-Event"
	TargetIdHeader   = "X-Github-Hook-Installation-Target-Id"
	TargetTypeHeader = "X-Github-Hook-Installation-Target-Type"
)

func HandleGitHubPost(ctx echo.Context) error {
	gitstatefetteContext := ctx.(*context.GitstafetteContext)
	body := ctx.Request().Body
	defer body.Close()
	messagePayload, err := ioutil.ReadAll(body)
	if err != nil {
		fmt.Printf("Ran into an error parsing content (Assumed GitHub Post): %v\n", err)
	}

	headers := ctx.Request().Header
	targetType := headers[TargetTypeHeader]
	if len(headers) <= 0 || len(headers[TargetIdHeader]) <= 0 || targetType[0] != "repository" {
		return ctx.String(http.StatusNotAcceptable, "Event is not for a repository")
	}

	targetRepositoryID := headers[TargetIdHeader][0]
	if cache.RepoWatcher.RepositoryIsWatched(targetRepositoryID) {
		cache.Event(targetRepositoryID, messagePayload, headers, gitstatefetteContext.RelayEndpoint)
	} else {
		fmt.Printf("Target %v is not a watched repository", targetRepositoryID)
		return ctx.String(http.StatusNotAcceptable, "Event does not contain a watched repository")
	}
	return ctx.String(http.StatusOK, "Repository event cached")

	//X-Github-Hook-Installation-Target-Id:[537845873]
	//X-Github-Hook-Installation-Target-Type:[repository]

	// TODO: re-enable with debug logging
	//if len(headers) > 0 {
	//	fmt.Printf("Headers: %v\n", headers)
	//} else {
	//	fmt.Println("Did not find any headers")
	//}
	// TODO: re-enable with debug logging
	//for header := range headers {
	//	fmt.Sprintf("- %v", header)
	//}
	//
	//fmt.Printf("Received message: %v\n", messagePayload)

}
