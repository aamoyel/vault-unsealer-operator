package vault

import (
	"fmt"
	"net/http"
	"time"

	vault "github.com/hashicorp/vault/api"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func GetVaultStatus(vaultNodes []string) ([]string, error) {
	var sealedNodes []string
	for _, node := range vaultNodes {
		client, err := vault.NewClient(&vault.Config{Address: node, HttpClient: httpClient})
		if err != nil {
			return nil, fmt.Errorf("unable to initialize Vault client: %v", err)
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
