/*
Copyright SecureKey Technologies Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package operation

import (
	"encoding/json"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/trustbloc/trustbloc-did-method/pkg/internal/common/support"
)

// API endpoints.
const (
	healthCheckEndpoint = "/healthcheck"
)

type healthCheckResp struct {
	Status      string    `json:"status"`
	CurrentTime time.Time `json:"currentTime"`
}

// Handler http handler for each controller API endpoint.
type Handler interface {
	Path() string
	Method() string
	Handle() http.HandlerFunc
}

// New returns CreateCredential instance.
func New() *Operation {
	return &Operation{}
}

// Operation defines handlers for rp operations.
type Operation struct {
}

// GetRESTHandlers get all controller API handler available for this service.
func (o *Operation) GetRESTHandlers() []Handler {
	return []Handler{
		support.NewHTTPHandler(healthCheckEndpoint, http.MethodGet, o.healthCheckHandler),
	}
}

func (o *Operation) healthCheckHandler(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusOK)

	err := json.NewEncoder(rw).Encode(&healthCheckResp{
		Status:      "success",
		CurrentTime: time.Now(),
	})
	if err != nil {
		log.Errorf("healthcheck response failure, %s", err)
	}
}
