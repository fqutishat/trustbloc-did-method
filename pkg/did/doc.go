/*
Copyright SecureKey Technologies Inc. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
*/

package did

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"encoding/json"
	"fmt"

	docdid "github.com/hyperledger/aries-framework-go/pkg/doc/did"
	"github.com/square/go-jose/v3"
	"github.com/trustbloc/sidetree-core-go/pkg/jws"
	"github.com/trustbloc/sidetree-core-go/pkg/util/pubkey"
)

const (
	jsonldID            = "id"
	jsonldType          = "type"
	jsonldUsage         = "purpose"
	jsonldServicePoint  = "endpoint"
	jsonldRecipientKeys = "recipientKeys"
	jsonldRoutingKeys   = "routingKeys"
	jsonldPriority      = "priority"

	jsonldPublicKeyjwk = "jwk"

	// PublicKeyEncodingJwk define jwk encoding type
	PublicKeyEncodingJwk = "Jwk"

	// KeyPurposeAuth defines key purpose as authentication key
	KeyPurposeAuth = "auth"
	// KeyPurposeAssertion defines key purpose as assertion key
	KeyPurposeAssertion = "assertion"
	// KeyPurposeDelegation defines key purpose as delegation key
	KeyPurposeDelegation = "delegation"
	// KeyPurposeInvocation defines key purpose as invocation key
	KeyPurposeInvocation = "invocation"
	// KeyPurposeGeneral defines key purpose as general key
	KeyPurposeGeneral = "general"

	// JWSVerificationKey2020 defines key type signature
	JWSVerificationKey2020 = "JwsVerificationKey2020"

	// Ed25519VerificationKey2018 define key type signature
	Ed25519VerificationKey2018 = "Ed25519VerificationKey2018"

	// Ed25519KeyType defines ed25119 key type
	Ed25519KeyType = "Ed25519"

	// P256KeyType EC P-256 key type
	P256KeyType = "P256"
)

type rawDoc struct {
	PublicKey []map[string]interface{} `json:"publicKey,omitempty"`
	Service   []map[string]interface{} `json:"service,omitempty"`
}

// Doc DID Document definition
type Doc struct {
	PublicKey []PublicKey
	Service   []docdid.Service
}

// PublicKey DID doc public key.
type PublicKey struct {
	ID       string
	Type     string
	Encoding string
	KeyType  string
	Purpose  []string
	Recovery bool
	Update   bool

	Value []byte
}

// JSONBytes converts document to json bytes
func (doc *Doc) JSONBytes() ([]byte, error) {
	publicKeys, err := populateRawPublicKeys(doc.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("JSON unmarshalling of Public Key failed: %w", err)
	}

	raw := &rawDoc{
		PublicKey: publicKeys,
		Service:   populateRawServices(doc.Service),
	}

	byteDoc, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("JSON unmarshalling of document failed: %w", err)
	}

	return byteDoc, nil
}

// GetValueFromJWK Populate the PublicKey contents from a JSON Web Key
func (pk *PublicKey) GetValueFromJWK(jwk *jose.JSONWebKey) error {
	if edKey, ok := jwk.Key.(ed25519.PublicKey); ok {
		pk.Value = edKey
		return nil
	}

	return fmt.Errorf("unsupported PublicKey source key type")
}

func populateRawPublicKeys(pks []PublicKey) ([]map[string]interface{}, error) {
	var rawPKs []map[string]interface{}

	for i := range pks {
		if !pks[i].Recovery && !pks[i].Update {
			publicKey, err := populateRawPublicKey(&pks[i])
			if err != nil {
				return nil, err
			}

			rawPKs = append(rawPKs, publicKey)
		}
	}

	return rawPKs, nil
}

func populateRawPublicKey(pk *PublicKey) (map[string]interface{}, error) {
	rawPK := make(map[string]interface{})
	rawPK[jsonldID] = pk.ID
	rawPK[jsonldType] = pk.Type
	rawPK[jsonldUsage] = pk.Purpose

	switch pk.Encoding {
	case PublicKeyEncodingJwk:
		var jwk *jws.JWK

		var err error

		switch pk.KeyType {
		case Ed25519KeyType:
			jwk, err = pubkey.GetPublicKeyJWK(ed25519.PublicKey(pk.Value))
			if err != nil {
				return nil, err
			}
		case P256KeyType:
			x, y := elliptic.Unmarshal(elliptic.P256(), pk.Value)

			jwk, err = pubkey.GetPublicKeyJWK(&ecdsa.PublicKey{X: x, Y: y, Curve: elliptic.P256()})
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid key type: %s", pk.KeyType)
		}

		rawPK[jsonldPublicKeyjwk] = jwk
	default:
		return nil, fmt.Errorf("public key encoding not supported: %s", pk.Encoding)
	}

	return rawPK, nil
}

func populateRawServices(services []docdid.Service) []map[string]interface{} {
	var rawServices []map[string]interface{}

	for _, service := range services {
		rawService := make(map[string]interface{})

		for k, v := range service.Properties {
			rawService[k] = v
		}

		rawService[jsonldID] = service.ID
		rawService[jsonldType] = service.Type
		rawService[jsonldServicePoint] = service.ServiceEndpoint
		rawService[jsonldRecipientKeys] = service.RecipientKeys
		rawService[jsonldRoutingKeys] = service.RoutingKeys
		rawService[jsonldPriority] = service.Priority

		rawServices = append(rawServices, rawService)
	}

	return rawServices
}
