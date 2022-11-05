package cache

import (
	"fmt"
	api "github.com/joostvdg/gitstafette/api/v1"
	"log"
	"strings"
)

const delimiter = ","

var RepoWatcher *RepositoryWatcher

func init() {
	RepoWatcher = CreateRepositoryWatcher()
}

type RepositoryWatcher struct {
	Repositories api.Repositories
}

func CreateRepositoryWatcher() *RepositoryWatcher {
	return &RepositoryWatcher{
		Repositories: make(api.Repositories),
	}
}

func (r *RepositoryWatcher) AddRepository(repository *api.Repository) {
	r.Repositories[repository.ID] = repository
}

func (r *RepositoryWatcher) GetRepository(repositoryID string) *api.Repository {
	return r.Repositories[repositoryID]
}

func (r *RepositoryWatcher) RepositoryIsWatched(repositoryID string) bool {
	return r.Repositories[repositoryID] != nil
}

func (r *RepositoryWatcher) ReportWatchedRepositories() {
	fmt.Println("Watching repos:")
	for _, repo := range r.Repositories {
		fmt.Printf("- %v\n", repo.ID)
	}
}

func (r *RepositoryWatcher) WatchedRepositories() []string {
	keys := make([]string, len(r.Repositories))
	i := 0
	for key := range r.Repositories {
		keys[i] = key
		i++
	}
	return keys
}

func (r *RepositoryWatcher) Init(repositoryIDs string) {
	if repositoryIDs == "" || len(repositoryIDs) <= 1 {
		log.Fatal("Did not receive any RepositoryID to watch")
	} else if strings.Contains(repositoryIDs, delimiter) {
		repoIds := strings.Split(repositoryIDs, delimiter)
		for _, repoID := range repoIds {
			repository := api.Repository{ID: repoID}
			r.AddRepository(&repository)
		}
	} else {
		repository := api.Repository{ID: repositoryIDs}
		r.AddRepository(&repository)
	}
}
