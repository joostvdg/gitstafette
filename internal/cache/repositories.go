package cache

import (
	"fmt"
	v1 "github.com/joostvdg/gitstafette/api/v1"
)

var RepoWatcher *RepositoryWatcher

func init() {
	RepoWatcher = CreateRepositoryWatcher()
}

type RepositoryWatcher struct {
	Repositories v1.Repositories
}

func CreateRepositoryWatcher() *RepositoryWatcher {
	return &RepositoryWatcher{
		Repositories: make(v1.Repositories),
	}
}

func (r *RepositoryWatcher) AddRepository(repository *v1.Repository) {
	r.Repositories[repository.ID] = repository
}

func (r *RepositoryWatcher) GetRepository(repositoryID string) *v1.Repository {
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
