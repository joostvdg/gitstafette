package v1

import (
	"github.com/joostvdg/gitstafette/internal/cache"
	"github.com/labstack/echo/v4"
	"net/http"
)

// WatchedRepositoryList simple type for returning proper JSON
type WatchedRepositoryList struct {
	GitHubRepositoryIDs []string
}

// TODO add other CRUD methods

// HandleWatchListGet handles API call for listing wich repositories are currently watched
func HandleWatchListGet(ctx echo.Context) error {

	repoIds := cache.Repositories.Repositories
	list := WatchedRepositoryList{repoIds}

	return ctx.JSON(http.StatusOK, list)
}
