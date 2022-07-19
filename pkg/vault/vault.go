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

func GetVaultStatus(vaultAddr string) (bool, error) {
	client, err := vault.NewClient(&vault.Config{Address: vaultAddr, HttpClient: httpClient})
	if err != nil {
		return false, fmt.Errorf("unable to initialize Vault client: %v", err)
	}

	sealStatus, err := client.Sys().SealStatus()
	if err != nil {
		return false, fmt.Errorf("unable to get seal status: %v", err)
	}

	if sealStatus.Sealed {
		return true, nil
	} else {
		return false, nil
	}
}
