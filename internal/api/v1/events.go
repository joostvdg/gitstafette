package v1

import (
	v1 "github.com/joostvdg/gitstafette/api/v1"
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/labstack/echo/v4"
	"net/http"
)

// WatchedRepositoryList simple type for returning proper JSON
type RepositoryEvents struct {
	Events []*v1.WebhookEventInternal
}

// TODO add other CRUD methods

// HandleRetrieveEventsForRepository handles API call for listing wich repositories are currently watched
func HandleRetrieveEventsForRepository(ctx echo.Context) error {
	repositoryID := ctx.Param("repo")

	if repositoryID == "" {
		return ctx.String(http.StatusBadRequest, "This request requires a valid RepositoryID")
	}

	events := cache.Store.RetrieveEventsForRepository(repositoryID)
	eventList := RepositoryEvents{events}
	return ctx.JSON(http.StatusOK, eventList)
}
