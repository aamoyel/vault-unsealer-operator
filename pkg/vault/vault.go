package vault

import (
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/vault/api"
)

type VSParams struct {
	VaultNodes []string
	Insecure   bool
	CaCert     string
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func GetVaultStatus(p VSParams) (sealedNodes []string, err error) {
	var client *api.Client
	for _, node := range p.VaultNodes {
		if p.Insecure {
			config := &api.Config{Address: node}
			config.ConfigureTLS(&api.TLSConfig{Insecure: true})
			client, err = api.NewClient(config)
			if err != nil {
				return nil, fmt.Errorf("unable to initialize Vault tls client: %v", err)
			}
		} else if len(p.CaCert) == 0 {
			client, err = api.NewClient(&api.Config{Address: node, HttpClient: httpClient})
			if err != nil {
				return nil, fmt.Errorf("unable to initialize Vault client: %v", err)
			}
		} else {
			config := &api.Config{Address: node}
			config.ConfigureTLS(&api.TLSConfig{CACertBytes: []byte(p.CaCert)})
			client, err = api.NewClient(config)
			if err != nil {
				return nil, fmt.Errorf("unable to initialize Vault tls client: %v", err)
			}
		}
		sealStatus, err := client.Sys().SealStatus()
		if err != nil {
			return nil, fmt.Errorf("unable to get seal status: %v", err)
		}
		if sealStatus.Sealed {
			sealedNodes = append(sealedNodes, node)
		}
	}
	if len(sealedNodes) > 0 {
		return sealedNodes, nil
	} else {
		return nil, nil
	}
}
