/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	mockconfig "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/config"
	mockdiscovery "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/discovery"
	mockmodels "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/models"
	mockselection "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/selection"
	"github.com/trustbloc/trustbloc-did-method/pkg/vdri/trustbloc/config/httpconfig"
	"github.com/trustbloc/trustbloc-did-method/pkg/vdri/trustbloc/discovery/staticdiscovery"
	"github.com/trustbloc/trustbloc-did-method/pkg/vdri/trustbloc/models"
	"github.com/trustbloc/trustbloc-did-method/pkg/vdri/trustbloc/selection/staticselection"
)

func TestEndpointService_GetEndpoints(t *testing.T) {
	t.Run("success: get endpoints using static services", func(t *testing.T) {
		shFile1, err := mockmodels.DummyStakeholderJSON("bar.baz", []string{
			"https://bar.baz/webapi/123456", "https://bar.baz/webapi/654321"})
		require.NoError(t, err)

		stakeholderServ1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, shFile1)
		}))
		defer stakeholderServ1.Close()

		shFile2, err := mockmodels.DummyStakeholderJSON("baz.qux", []string{
			"https://baz.qux/iyoubhlkn/", "https://baz.foo/ukjhjtfyw/"})
		require.NoError(t, err)

		stakeholderServ2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, shFile2)
		}))
		defer stakeholderServ2.Close()

		consortiumFile, err := mockmodels.DummyConsortiumJSON("foo.bar", []*models.StakeholderListElement{
			{
				Domain: stakeholderServ1.URL,
			},
			{
				Domain: stakeholderServ2.URL,
			},
		})
		require.NoError(t, err)

		serv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, consortiumFile)
		}))
		defer serv.Close()

		configService := httpconfig.NewService(httpconfig.WithTLSConfig(&tls.Config{}))
		endpointService := NewService(staticdiscovery.NewService(configService), staticselection.NewService(configService))

		endpoints, err := endpointService.GetEndpoints(serv.URL)
		require.NoError(t, err)
		require.Len(t, endpoints, 2)
	})

	t.Run("failure: config service stakeholder", func(t *testing.T) {
		configService := &mockconfig.MockConfigService{
			GetConsortiumFunc: func(s string, s2 string) (*models.ConsortiumFileData, error) {
				return &models.ConsortiumFileData{
					Config: &models.Consortium{
						Members: []*models.StakeholderListElement{{Domain: "foo"}},
					},
				}, nil
			},
			GetStakeholderFunc: func(s string, s2 string) (*models.StakeholderFileData, error) {
				return nil, fmt.Errorf("stakeholder error")
			}}

		endpointService := NewService(staticdiscovery.NewService(configService), &mockselection.MockSelectionService{})

		endpoints, err := endpointService.GetEndpoints("")
		require.Error(t, err)
		require.Nil(t, endpoints)
		require.Contains(t, err.Error(), "stakeholder error")
	})

	t.Run("failure: discovery error", func(t *testing.T) {
		endpointService := NewService(&mockdiscovery.MockDiscoveryService{
			GetEndpointsFunc: func(domain string) ([]*models.Endpoint, error) {
				return nil, fmt.Errorf("discovery error")
			},
		}, &mockselection.MockSelectionService{})

		endpoints, err := endpointService.GetEndpoints("")
		require.Error(t, err)
		require.Nil(t, endpoints)
		require.Contains(t, err.Error(), "discovery error")
	})

	t.Run("failure: selection", func(t *testing.T) {
		endpointService := NewService(&mockdiscovery.MockDiscoveryService{}, &mockselection.MockSelectionService{
			SelectEndpointsFunc: func(domain string, endpoints []*models.Endpoint) ([]*models.Endpoint, error) {
				return nil, fmt.Errorf("selection error")
			}})

		endpoints, err := endpointService.GetEndpoints("")
		require.Error(t, err)
		require.Nil(t, endpoints)
		require.Contains(t, err.Error(), "selection error")
	})
}
