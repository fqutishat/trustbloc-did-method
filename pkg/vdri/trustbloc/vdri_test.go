/*
Copyright SecureKey Technologies Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package trustbloc

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/aries-framework-go/pkg/doc/did"
	vdriapi "github.com/hyperledger/aries-framework-go/pkg/framework/aries/api/vdri"
	mockvdri "github.com/hyperledger/aries-framework-go/pkg/mock/vdri"
	"github.com/square/go-jose/v3"
	"github.com/stretchr/testify/require"

	mockconfig "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/config"
	mockdidconf "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/didconfiguration"
	mockendpoint "github.com/trustbloc/trustbloc-did-method/pkg/internal/mock/endpoint"
	"github.com/trustbloc/trustbloc-did-method/pkg/vdri/trustbloc/didconfiguration"
	"github.com/trustbloc/trustbloc-did-method/pkg/vdri/trustbloc/models"
)

func TestVDRI_Accept(t *testing.T) {
	t.Run("test success", func(t *testing.T) {
		v := New()
		require.True(t, v.Accept("trustbloc"))
	})

	t.Run("test return false", func(t *testing.T) {
		v := New()
		require.False(t, v.Accept("bloc1"))
	})
}

func TestVDRI_Store(t *testing.T) {
	t.Run("test error", func(t *testing.T) {
		v := New()
		err := v.Store(nil, nil)
		require.NoError(t, err)
	})
}

func TestVDRI_Build(t *testing.T) {
	t.Run("test error", func(t *testing.T) {
		v := New()
		_, err := v.Build(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "build method not supported for did bloc")
	})
}

func httpVdriFunc(doc *did.Doc, err error) func(url string) (v vdri, err error) {
	return func(url string) (v vdri, e error) {
		return &mockvdri.MockVDRI{
			ReadFunc: func(didID string, opts ...vdriapi.ResolveOpts) (*did.Doc, error) {
				return doc, err
			}}, nil
	}
}

func ed25519SigningKey(t *testing.T, jsonKey string) *jose.SigningKey {
	var key jose.JSONWebKey
	err := key.UnmarshalJSON([]byte(jsonKey))
	require.NoError(t, err)

	return &jose.SigningKey{Key: key, Algorithm: jose.EdDSA}
}

func confSignature(configBytes []byte, keys []jose.SigningKey) (*jose.JSONWebSignature, error) {
	signer, err := jose.NewMultiSigner(keys, nil)
	if err != nil {
		return nil, err
	}

	return signer.Sign(configBytes)
}

func signConfig(config interface{}, keys []jose.SigningKey) (string, error) {
	configBytes, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	jws, err := confSignature(configBytes, keys)
	if err != nil {
		return "", err
	}

	return jws.FullSerialize(), nil
}

func dummyConsortium(consortiumDomain, stakeholderDomain string) *models.Consortium {
	return &models.Consortium{
		Domain: consortiumDomain,
		Policy: models.ConsortiumPolicy{
			NumQueries: 1,
		},
		Members: []*models.StakeholderListElement{{
			Domain: stakeholderDomain,
			DID:    "did:example:123456789abcdefghi",
			PublicKey: models.PublicKey{
				ID:  "did:example:123456789abcdefghi#key-1",
				JWK: []byte(pubKeyJSON),
			},
		}},
		Previous: "",
	}
}

func dummyStakeholder(stakeholderDomain string) *models.Stakeholder {
	return &models.Stakeholder{
		Domain:    stakeholderDomain,
		DID:       "did:example:foo",
		Policy:    models.StakeholderSettings{},
		Endpoints: []string{"foo"},
		Previous:  "",
	}
}

func signedConsortiumFileData(t *testing.T, consortium *models.Consortium, key *jose.SigningKey,
) *models.ConsortiumFileData {
	if key == nil {
		return &models.ConsortiumFileData{Config: consortium}
	}

	confData, err := json.Marshal(consortium)
	require.NoError(t, err)

	jws, err := confSignature(confData, []jose.SigningKey{*key})
	require.NoError(t, err)

	return &models.ConsortiumFileData{
		Config: consortium,
		JWS:    jws,
	}
}

func signedStakeholderFileData(t *testing.T, stakeholder *models.Stakeholder, key *jose.SigningKey,
) *models.StakeholderFileData {
	if key == nil {
		return &models.StakeholderFileData{Config: stakeholder}
	}

	confData, err := json.Marshal(stakeholder)
	require.NoError(t, err)

	jws, err := confSignature(confData, []jose.SigningKey{*key})
	require.NoError(t, err)

	return &models.StakeholderFileData{
		Config: stakeholder,
		JWS:    jws,
	}
}

func TestVDRI_Read(t *testing.T) {
	t.Run("test error from get http vdri for resolver url", func(t *testing.T) {
		v := New(WithResolverURL("url"))

		_, err := v.getHTTPVDRI("")
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty url")

		v.getHTTPVDRI = func(url string) (v vdri, err error) {
			return nil, fmt.Errorf("get http vdri error")
		}

		doc, err := v.Read("did")
		require.Error(t, err)
		require.Contains(t, err.Error(), "get http vdri error")
		require.Nil(t, doc)
	})

	t.Run("test error from http vdri build for resolver url", func(t *testing.T) {
		v := New(WithResolverURL("url"))

		v.getHTTPVDRI = httpVdriFunc(nil, fmt.Errorf("read error"))

		doc, err := v.Read("did")
		require.Error(t, err)
		require.Contains(t, err.Error(), "read error")
		require.Nil(t, doc)
	})

	t.Run("test success for resolver url", func(t *testing.T) {
		v := New(WithResolverURL("url"))

		v.getHTTPVDRI = httpVdriFunc(&did.Doc{ID: "did"}, nil)

		doc, err := v.Read("did")
		require.NoError(t, err)
		require.Equal(t, "did", doc.ID)
	})

	t.Run("test error parsing did", func(t *testing.T) {
		v := New()

		v.getHTTPVDRI = func(url string) (v vdri, err error) {
			return nil, nil
		}

		doc, err := v.Read("did:1223")
		require.Error(t, err)
		require.Contains(t, err.Error(), "wrong did did:1223")
		require.Nil(t, doc)
	})

	t.Run("test error from get endpoints", func(t *testing.T) {
		v := New()

		v.endpointService = &mockendpoint.MockEndpointService{
			GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
				return nil, fmt.Errorf("discover error")
			}}

		v.validatedConsortium["testnet"] = true

		doc, err := v.Read("did:trustbloc:testnet:123")
		require.Error(t, err)
		require.Contains(t, err.Error(), "discover error")
		require.Nil(t, doc)

		v.endpointService = &mockendpoint.MockEndpointService{
			GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
				return nil, fmt.Errorf("select error")
			}}

		doc, err = v.Read("did:trustbloc:testnet:123")
		require.Error(t, err)
		require.Contains(t, err.Error(), "select error")
		require.Nil(t, doc)

		v.endpointService = &mockendpoint.MockEndpointService{
			GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
				return nil, nil
			}}

		doc, err = v.Read("did:trustbloc:testnet:123")
		require.Error(t, err)
		require.Contains(t, err.Error(), "list of endpoints is empty")
		require.Nil(t, doc)
	})

	t.Run("test error from get http vdri", func(t *testing.T) {
		v := New()

		v.endpointService = &mockendpoint.MockEndpointService{
			GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
				return []*models.Endpoint{{URL: "url"}}, nil
			}}

		v.getHTTPVDRI = func(url string) (v vdri, err error) {
			return nil, fmt.Errorf("get http vdri error")
		}

		v.validatedConsortium["testnet"] = true

		doc, err := v.Read("did:trustbloc:testnet:123")
		require.Error(t, err)
		require.Contains(t, err.Error(), "get http vdri error")
		require.Nil(t, doc)
	})

	t.Run("test error from http vdri read", func(t *testing.T) {
		v := New()

		v.endpointService = &mockendpoint.MockEndpointService{
			GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
				return []*models.Endpoint{{URL: "url"}}, nil
			}}

		v.getHTTPVDRI = httpVdriFunc(nil, fmt.Errorf("read error"))

		v.validatedConsortium["testnet"] = true

		doc, err := v.Read("did:trustbloc:testnet:123")
		require.Error(t, err)
		require.Contains(t, err.Error(), "read error")
		require.Nil(t, doc)
	})

	//nolint:gocritic
	// t.Run("test error from mismatch", func(t *testing.T) {
	// 	v := New()
	//
	// 	v.endpointService = &mockendpoint.MockEndpointService{
	// 		GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
	// 			return []*models.Endpoint{{URL: "url"}, {URL: "url.2"}}, nil
	// 		}}
	//
	// 	counter := 0
	//
	// 	v.getHTTPVDRI = func(url string) (v vdri, err error) {
	// 		return &mockvdri.MockVDRI{
	// 			ReadFunc: func(didID string, opts ...vdriapi.ResolveOpts) (*did.Doc, error) {
	// 				counter++
	// 				return generateDIDDoc("test:" + string(counter)), nil
	// 			}}, nil
	// 	}
	//
	// 	_, err := v.Read("did:trustbloc:testnet:123")
	// 	require.Error(t, err)
	// 	require.Contains(t, err.Error(), "mismatch")
	// })

	t.Run("test success", func(t *testing.T) {
		v := New()

		v.endpointService = &mockendpoint.MockEndpointService{
			GetEndpointsFunc: func(domain string) (endpoints []*models.Endpoint, err error) {
				return []*models.Endpoint{{URL: "url"}, {URL: "url.2"}}, nil
			}}

		v.getHTTPVDRI = httpVdriFunc(&did.Doc{ID: "did:trustbloc:testnet:123"}, nil)

		sigKey := ed25519SigningKey(t, keyJSON)

		cfd := signedConsortiumFileData(t, &models.Consortium{
			Domain:   "testnet",
			Policy:   models.ConsortiumPolicy{},
			Members:  nil,
			Previous: "",
		}, sigKey)

		v.configService = &mockconfig.MockConfigService{
			GetConsortiumFunc: func(u string, d string) (*models.ConsortiumFileData, error) {
				return cfd, nil
			},
		}

		doc, err := v.Read("did:trustbloc:testnet:123")
		require.NoError(t, err)
		require.Equal(t, "did:trustbloc:testnet:123", doc.ID)
	})
}

const (
	keyJSON = `{
  "kty": "OKP",
  "kid": "key1",
  "d": "CSLczqR1ly2lpyBcWne9gFKnsjaKJw0dKfoSQu7lNvg",
  "crv": "Ed25519",
  "x": "bWRCy8DtNhRO3HdKTFB2eEG5Ac1J00D0DQPffOwtAD0"
}`

	pubKeyJSON = `{
  "kty": "OKP",
  "kid": "key1",
  "crv": "Ed25519",
  "x": "bWRCy8DtNhRO3HdKTFB2eEG5Ac1J00D0DQPffOwtAD0"
}`

	testDoc = `{
  "@context": ["https://w3id.org/did/v1"],
  "publicKey": [{
    "id": "did:example:123456789abcdefghi#key-2",
    "controller": "did:example:123456789abcdefghi",
    "publicKeyJwk":{
      "kty": "OKP",
      "crv": "Ed25519",
      "x": "8rfXFZNHZs9GYzGbQLYDasGUAm1brAgTLI0jrD4KheU"
    },
    "type":"JwsVerificationKey2020"
  }],
  "id": "did:example:123456789abcdefghi",
  "authentication": [
    {
      "id": "did:example:123456789abcdefghi#key-1",
      "controller": "did:example:123456789abcdefghi",
      "publicKeyJwk":{
		"kty": "OKP",
		"crv": "Ed25519",
	    "x": "bWRCy8DtNhRO3HdKTFB2eEG5Ac1J00D0DQPffOwtAD0"
	  },
      "type":"JwsVerificationKey2020"
    }
  ],
  "service": []
}`
)

func TestVDRI_ValidateConsortium(t *testing.T) {
	sigKey := ed25519SigningKey(t, keyJSON)

	t.Run("success - no stakeholders to verify", func(t *testing.T) {
		v := New()

		var confFile string

		consortiumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, confFile)
		}))
		defer consortiumServer.Close()

		conf := models.Consortium{
			Domain:   consortiumServer.URL,
			Policy:   models.ConsortiumPolicy{},
			Members:  nil,
			Previous: "",
		}

		var err error
		confFile, err = signConfig(conf, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		_, err = v.ValidateConsortium(consortiumServer.URL)
		require.NoError(t, err)
	})

	t.Run("failure - consortium invalid", func(t *testing.T) {
		v := New()

		confFile := `RU^&I*&*&OH`

		consortiumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, confFile)
		}))
		defer consortiumServer.Close()

		_, err := v.ValidateConsortium(consortiumServer.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "consortium invalid")
	})

	t.Run("failure - stakeholders don't sign consortium config", func(t *testing.T) {
		v := New()

		var consortiumFile, stakeholderFile, didConfFile string

		consortiumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, consortiumFile)
		}))
		defer consortiumServer.Close()

		stakeholderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.String(), "did-configuration") {
				fmt.Fprint(w, didConfFile)
			} else {
				fmt.Fprint(w, stakeholderFile)
			}
		}))
		defer stakeholderServer.Close()

		consortium := dummyConsortium(consortiumServer.URL, stakeholderServer.URL)

		var err error
		consortiumFile, err = signConfig(consortium, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		stakeholder := dummyStakeholder(stakeholderServer.URL)

		stakeholderFile, err = signConfig(stakeholder, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		v.configService = &mockconfig.MockConfigService{
			GetConsortiumFunc: func(u string, d string) (*models.ConsortiumFileData, error) {
				return &models.ConsortiumFileData{
					Config: consortium,
					JWS:    nil,
				}, nil
			},
			GetStakeholderFunc: func(u string, d string) (*models.StakeholderFileData, error) {
				return nil, fmt.Errorf("error stakeholder")
			},
		}

		v.getHTTPVDRI = httpVdriFunc(nil, nil)

		_, err = v.ValidateConsortium(consortiumServer.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to fetch stakeholders")
	})

	t.Run("success - verify with one stakeholder", func(t *testing.T) {
		v := New()

		var consortiumFile, stakeholderFile, didConfFile string

		consortiumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, consortiumFile)
		}))
		defer consortiumServer.Close()

		stakeholderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.String(), "did-configuration"):
				fmt.Fprint(w, didConfFile)
			case strings.Contains(r.URL.String(), consortiumServer.URL):
				fmt.Fprint(w, consortiumFile)
			default:
				fmt.Fprint(w, stakeholderFile)
			}
		}))
		defer stakeholderServer.Close()

		var err error

		consortium := dummyConsortium(consortiumServer.URL, stakeholderServer.URL)
		consortiumFile, err = signConfig(consortium, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		stakeholder := dummyStakeholder(stakeholderServer.URL)
		stakeholderFile, err = signConfig(stakeholder, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		didConf, err := didconfiguration.CreateDIDConfiguration(
			stakeholderServer.URL, "did:example:123456789abcdefghi", 0, sigKey)
		require.NoError(t, err)

		didConfBytes, err := json.Marshal(didConf)
		require.NoError(t, err)

		didConfFile = string(didConfBytes)

		mockDoc, err := did.ParseDocument([]byte(testDoc))
		require.NoError(t, err)

		v.getHTTPVDRI = httpVdriFunc(mockDoc, nil)

		_, err = v.ValidateConsortium(consortiumServer.URL)
		require.NoError(t, err)
	})

	t.Run("failure - can't resolve stakeholder DID", func(t *testing.T) {
		v := New()

		var consortiumFile, stakeholderFile, didConfFile string

		consortiumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, consortiumFile)
		}))
		defer consortiumServer.Close()

		stakeholderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.String(), "did-configuration") {
				fmt.Fprint(w, didConfFile)
			} else {
				fmt.Fprint(w, stakeholderFile)
			}
		}))
		defer stakeholderServer.Close()

		var err error

		consortium := dummyConsortium(consortiumServer.URL, stakeholderServer.URL)
		consortiumFile, err = signConfig(consortium, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		stakeholder := dummyStakeholder(stakeholderServer.URL)
		stakeholderFile, err = signConfig(stakeholder, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		didConf, err := didconfiguration.CreateDIDConfiguration(
			stakeholderServer.URL, "did:example:123456789abcdefghi", 0, sigKey)
		require.NoError(t, err)

		didConfBytes, err := json.Marshal(didConf)
		require.NoError(t, err)

		didConfFile = string(didConfBytes)

		v.configService = &mockconfig.MockConfigService{
			GetConsortiumFunc: func(u string, d string) (*models.ConsortiumFileData, error) {
				return &models.ConsortiumFileData{
					Config: consortium,
					JWS:    nil,
				}, nil
			},
			GetStakeholderFunc: func(u string, d string) (*models.StakeholderFileData, error) {
				return &models.StakeholderFileData{
					Config: stakeholder,
					JWS:    nil,
				}, nil
			},
		}

		v.getHTTPVDRI = func(url string) (v vdri, err error) {
			return nil, fmt.Errorf("foo")
		}

		_, err = v.ValidateConsortium(consortiumServer.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "can't resolve stakeholder DID")
	})

	t.Run("failure - verifying stakeholder", func(t *testing.T) {
		var consortiumFile, stakeholderFile, didConfFile string

		consortiumServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, consortiumFile)
		}))
		defer consortiumServer.Close()

		stakeholderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.String(), "did-configuration") {
				fmt.Fprint(w, didConfFile)
			} else {
				fmt.Fprint(w, stakeholderFile)
			}
		}))
		defer stakeholderServer.Close()

		var err error

		consortium := dummyConsortium(consortiumServer.URL, stakeholderServer.URL)
		consortiumFile, err = signConfig(consortium, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		stakeholder := dummyStakeholder(stakeholderServer.URL)
		stakeholderFile, err = signConfig(stakeholder, []jose.SigningKey{*sigKey})
		require.NoError(t, err)

		didConf, err := didconfiguration.CreateDIDConfiguration(
			stakeholderServer.URL, "did:example:123456789abcdefghi", 0, sigKey)
		require.NoError(t, err)

		didConfBytes, err := json.Marshal(didConf)
		require.NoError(t, err)

		didConfFile = string(didConfBytes)

		v := New()

		v.configService = &mockconfig.MockConfigService{
			GetConsortiumFunc: func(u string, d string) (*models.ConsortiumFileData, error) {
				return &models.ConsortiumFileData{
					Config: consortium,
					JWS:    nil,
				}, nil
			},
			GetStakeholderFunc: func(u string, d string) (*models.StakeholderFileData, error) {
				return &models.StakeholderFileData{
					Config: stakeholder,
					JWS:    nil,
				}, nil
			},
		}

		mockDoc, err := did.ParseDocument([]byte(testDoc))
		require.NoError(t, err)

		v.getHTTPVDRI = httpVdriFunc(mockDoc, nil)

		v.didConfigService = &mockdidconf.MockDIDConfigService{
			VerifyStakeholderFunc: func(domain string, doc *did.Doc) error {
				return fmt.Errorf("stakeholder error")
			}}

		_, err = v.ValidateConsortium(consortiumServer.URL)
		require.Error(t, err)
		require.Contains(t, err.Error(), "stakeholder error")
	})
}

func Test_verifyStakeholder(t *testing.T) {
	sigKey := ed25519SigningKey(t, keyJSON)

	mockDoc, err := did.ParseDocument([]byte(testDoc))
	require.NoError(t, err)

	alternateKey := ed25519SigningKey(t, `{
	"kty":"OKP",
	"crv":"Ed25519",
	"d":"nWGxne_9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A",
	"x":"11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
}`)

	tests := []struct {
		testName          string
		consortiumDomain  string
		stakeholderDomain string
		consortiumKey     *jose.SigningKey
		stakeholderKey    *jose.SigningKey
		isErr             bool
		errString         string
	}{
		{
			"success",
			"consortium.url",
			"stakeholder.url",
			sigKey,
			sigKey,
			false,
			"",
		}, {
			"failure - bad consortium signature",
			"consortium.url",
			"stakeholder.url",
			alternateKey,
			nil,
			true,
			"does not sign consortium",
		}, {
			"failure - bad stakeholder signature",
			"consortium.url",
			"stakeholder.url",
			sigKey,
			alternateKey,
			true,
			"does not sign itself",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.testName, func(t *testing.T) {
			cfd := signedConsortiumFileData(t, dummyConsortium(test.consortiumDomain, test.stakeholderDomain),
				test.consortiumKey)
			sfd := signedStakeholderFileData(t, dummyStakeholder(test.stakeholderDomain), test.stakeholderKey)

			v := New()

			v.getHTTPVDRI = httpVdriFunc(mockDoc, nil)

			v.didConfigService = &mockdidconf.MockDIDConfigService{
				VerifyStakeholderFunc: func(domain string, doc *did.Doc) error {
					return nil
				},
			}

			err = v.verifyStakeholder(cfd, sfd)

			if test.isErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.errString)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVDRI_Close(t *testing.T) {
	v := New()
	require.NoError(t, v.Close())
}

func Test_canonicalizeDoc(t *testing.T) {
	var docs = [][2]string{
		{`{
  "@context": ["https://w3id.org/did/v1"],
  "publicKey": [{
    "id": "did:example:123456789abcdefghi#keys-3",
    "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV",
    "type": "Secp256k1VerificationKey2018",
    "controller": "did:example:123456789abcdefghi"
  }],
  "id": "did:example:123456789abcdefghi",
  "authentication": [
    {
      "id": "did:example:123456789abcdefghi#keys-2",
      "type": "Ed25519VerificationKey2018",
      "controller": "did:example:123456789abcdefghi",
      "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV"
    },
    "did:example:123456789abcdefghi#keys-3"
  ],
  "service": [{
    "id": "did:example:123456789abcdefghi#oidc",
    "type": "OpenIdConnectVersion1.0Service",
    "serviceEndpoint": "https://openid.example.com/"
  }, {
    "id": "did:example:123456789abcdefghi#messaging",
    "type": "MessagingService",
    "serviceEndpoint": "https://example.com/messages/8377464"
  }, {
    "id": "did:example:123456789abcdefghi#vcStore",
    "type": "CredentialRepositoryService",
    "serviceEndpoint": "https://repository.example.com/service/8377464"
  }, {
    "id": "did:example:123456789abcdefghi#xdi",
    "serviceEndpoint": "https://xdi.example.com/8377464",
    "type": "XdiService"
  }, {
    "type": "HubService",
    "id": "did:example:123456789abcdefghi#hub",
    "serviceEndpoint": "https://hub.example.com/.identity/did:example:0123456789abcdef/"
  }, {
    "id": "did:example:123456789abcdefghi#inbox",
    "description": "My public social inbox",
    "type": "SocialWebInboxService",
    "serviceEndpoint": "https://social.example.com/83hfh37dj",
    "spamCost": {
      "amount": "0.50",
      "currency": "USD"
    }
  }]
}`,
			`{
  "@context": ["https://w3id.org/did/v1"],
  "publicKey": [{
    "id": "did:example:123456789abcdefghi#keys-3",
    "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV",
    "type": "Secp256k1VerificationKey2018",
    "controller": "did:example:123456789abcdefghi"
  }],
  "id": "did:example:123456789abcdefghi",
  "authentication": [
    {
      "id": "did:example:123456789abcdefghi#keys-2",
      "type": "Ed25519VerificationKey2018",
      "controller": "did:example:123456789abcdefghi",
      "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV"
    },
    "did:example:123456789abcdefghi#keys-3"
  ],
  "service": [{
    "id": "did:example:123456789abcdefghi#messaging",
    "type": "MessagingService",
    "serviceEndpoint": "https://example.com/messages/8377464"
  }, {
    "id": "did:example:123456789abcdefghi#oidc",
    "type": "OpenIdConnectVersion1.0Service",
    "serviceEndpoint": "https://openid.example.com/"
  }, {
    "id": "did:example:123456789abcdefghi#vcStore",
    "type": "CredentialRepositoryService",
    "serviceEndpoint": "https://repository.example.com/service/8377464"
  }, {
    "id": "did:example:123456789abcdefghi#xdi",
    "serviceEndpoint": "https://xdi.example.com/8377464",
    "type": "XdiService"
  }, {
    "type": "HubService",
    "id": "did:example:123456789abcdefghi#hub",
    "serviceEndpoint": "https://hub.example.com/.identity/did:example:0123456789abcdef/"
  }, {
    "id": "did:example:123456789abcdefghi#inbox",
    "description": "My public social inbox",
    "type": "SocialWebInboxService",
    "serviceEndpoint": "https://social.example.com/83hfh37dj",
    "spamCost": {
      "amount": "0.50",
      "currency": "USD"
    }
  }]
}`},
		{`{
  "@context": ["https://w3id.org/did/v1"],
  "publicKey": [{
    "id": "did:example:123456789abcdefghi#keys-3",
    "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV",
    "type": "Secp256k1VerificationKey2018",
    "controller": "did:example:123456789abcdefghi"
  }],
  "id": "did:example:123456789abcdefghi",
  "authentication": [
    {
      "id": "did:example:123456789abcdefghi#keys-2",
      "controller": "did:example:123456789abcdefghi",
      "publicKeyJwk":{
        "kty":"OKP",
        "crv":"Ed25519",
        "x":"60-uLNeLPAT-gaV_7_9_g330m0aLRlqk-LEnQvz2lv0"
      },
      "type":"JwsVerificationKey2020"
    },
    "did:example:123456789abcdefghi#keys-3"
  ],
  "service": [{
    "id": "did:example:123456789abcdefghi#oidc",
    "type": "OpenIdConnectVersion1.0Service",
    "serviceEndpoint": "https://openid.example.com/"
  }, {
    "id": "did:example:123456789abcdefghi#messaging",
    "type": "MessagingService",
    "serviceEndpoint": "https://example.com/messages/8377464"
  }]
}`,
			`{
  "service": [ {
    "type": "MessagingService",
    "serviceEndpoint": "https://example.com/messages/8377464",
    "id": "did:example:123456789abcdefghi#messaging"
  }, {
    "id": "did:example:123456789abcdefghi#oidc",
    "serviceEndpoint": "https://openid.example.com/",
    "type": "OpenIdConnectVersion1.0Service"
  }],
  "id": "did:example:123456789abcdefghi",
  "authentication": [
    {
      "id": "did:example:123456789abcdefghi#keys-2",
      "type":"JwsVerificationKey2020",
      "controller": "did:example:123456789abcdefghi",
      "publicKeyJwk":{
        "crv":"Ed25519",
        "x":"60-uLNeLPAT-gaV_7_9_g330m0aLRlqk-LEnQvz2lv0",
        "kty":"OKP"
      }
    },
    "did:example:123456789abcdefghi#keys-3"
  ],
  "@context": ["https://w3id.org/did/v1"],
  "publicKey": [{
    "id": "did:example:123456789abcdefghi#keys-3",
    "publicKeyBase58": "H3C2AVvLMv6gmMNam3uVAjZpfkcJCwDwnZn6z3wXmqPV",
    "type": "Secp256k1VerificationKey2018",
    "controller": "did:example:123456789abcdefghi"
  }]
}`},
	}

	_ = `{
		"controller":"did:trustbloc:testnet.trustbloc.local:EiDDTwzrFVAmnsPG8D10MNJ-Ga5OH_KsNX8uLGmirWXP-g",
		"id":"did:trustbloc:testnet.trustbloc.local:EiDDTwzrFVAmnsPG8D10MNJ-Ga5OH_KsNX8uLGmirWXP-g#key-1",
		"publicKeyJwk":{
			"kty":"OKP",
			"crv":"Ed25519",
			"x":"60-uLNeLPAT-gaV_7_9_g330m0aLRlqk-LEnQvz2lv0"
		},
		"type":"JwsVerificationKey2020"
	}`

	t.Run("test canonicalization of equal docs", func(t *testing.T) {
		for _, pair := range docs {
			doc1, err := did.ParseDocument([]byte(pair[0]))
			require.NoError(t, err)
			doc2, err := did.ParseDocument([]byte(pair[1]))
			require.NoError(t, err)

			doc1Canonicalized, err := canonicalizeDoc(doc1)
			require.NoError(t, err)
			doc2Canonicalized, err := canonicalizeDoc(doc2)
			require.NoError(t, err)

			require.Equal(t, doc1Canonicalized, doc2Canonicalized)
		}
	})
}

func TestOpts(t *testing.T) {
	t.Run("test opts", func(t *testing.T) {
		// test WithTLSConfig
		var opts []Option
		opts = append(opts, WithTLSConfig(&tls.Config{ServerName: "test"}), WithAuthToken("tk1"))

		v := &VDRI{}

		// Apply options
		for _, opt := range opts {
			opt(v)
		}

		require.Equal(t, "test", v.tlsConfig.ServerName)
		require.Equal(t, "tk1", v.authToken)
	})
}

//nolint:deadcode,unused
func generateDIDDoc(id string) *did.Doc {
	t := time.Unix(0, 0)

	return &did.Doc{
		Context: nil,
		ID:      id,
		PublicKey: []did.PublicKey{{
			ID:         "",
			Type:       "",
			Controller: "",
			Value:      []byte{0},
		}},
		Service: []did.Service{{
			ID:              "",
			Type:            "",
			Priority:        0,
			RecipientKeys:   []string{""},
			RoutingKeys:     []string{""},
			ServiceEndpoint: "",
			Properties:      map[string]interface{}{},
		}},
		Authentication: []did.VerificationMethod{{PublicKey: did.PublicKey{
			ID:         "",
			Type:       "",
			Controller: "",
			Value:      []byte{0},
		}}},
		Created: nil,
		Updated: nil,
		Proof: []did.Proof{{
			Type:       "",
			Created:    &t,
			Creator:    "",
			ProofValue: nil,
			Domain:     "",
			Nonce:      nil,
		}},
	}
}
