package v1

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

func ValidateEvent(hmac string, event *v1.WebhookEvent) bool {
	digestHeader := ""
	for _, header := range event.Headers {
		if header.Name == SignatureHeader {
			digestHeader = header.Values
		}
	}
	if digestHeader == "" {
		return true
	}
	validationError := ValidateMessage(hmac, digestHeader, event.Body)
	if validationError != nil {
		sublogger.Warn().Msgf("Could not validate received webhook event [%v]: %v", event.EventId, validationError)
	}
	return true
}

func ValidateMessage(token string, givenSha string, payload []byte) error {
	if givenSha == "" {
		return fmt.Errorf("no sha checksum provided")
	}

	h := hmac.New(sha256.New, []byte(token))
	_, err := h.Write(payload)
	if err != nil {
		return err
	}

	computedSha := "sha256=" + hex.EncodeToString(h.Sum(nil))
	sublogger.Debug().Msgf("GivenSha: %v, Calculated Sha: %v", givenSha, computedSha)

	if computedSha != givenSha {
		return fmt.Errorf("sha checksums did not match")
	}
	return nil
}
