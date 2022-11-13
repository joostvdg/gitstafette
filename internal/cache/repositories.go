package cache

import (
	"log"
)

type RepositoryWatcher struct {
	Repositories []string
}

func createRepositoryWatcher() RepositoryWatcher {
	return RepositoryWatcher{
		Repositories: make([]string, 0),
	}
}

func (r *RepositoryWatcher) AddRepository(repositoryId string) {
	r.Repositories = append(r.Repositories, repositoryId)
}

func (r *RepositoryWatcher) RepositoryIsWatched(repositoryID string) bool {
	for _, value := range r.Repositories {
		if value == repositoryID {
			return true
		}
	}
	return false
}

func (r *RepositoryWatcher) ReportWatchedRepositories() {
	log.Println("Watching repos:")
	for _, repoId := range r.Repositories {
		log.Printf(" - %v\n", repoId)
	}
}
