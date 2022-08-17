# vault-unsealer-operator

<!-- TABLE OF CONTENTS -->
<details open>
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <a href="#about-the-project">About The Project</a>
      <ul>
        <li><a href="#purpose">Purpose</a></li>
        <li><a href="#built-with">Built With</a></li>
      </ul>
    </li>
    <li>
      <a href="#getting-started">Getting Started</a>
      <ul>
        <li><a href="#prerequisites">Prerequisites</a></li>
        <li><a href="#installation">Installation</a></li>
      </ul>
    </li>
    <li><a href="#usage">Usage</a></li>
    <li><a href="#Contribute">Contribute</a></li>
    <li><a href="#license">License</a></li>
    <li><a href="#contact">Contact</a></li>
  </ol>
</details>
</br>



<!-- ABOUT THE PROJECT -->
## About The Project

### Purpose
This kubernetes operator allows you to automate unseal process of your HashiCorp Vault clusters or instances with a sample file and secret.
<p align="right">(<a href="#top">back to top</a>)</p>


### Built With
* [Kubebuilder](https://book.kubebuilder.io/)

<p align="right">(<a href="#top">back to top</a>)</p>



<!-- GETTING STARTED -->
## Getting Started

### Prerequisites
You need to have :
* An operationnal Kubernetes cluster
* HashiCorp Vault cluster or instance
* kubectl binary

### Installation
1. Deploy the latest operator release via the 'bundle' file :
   ```sh
   kubectl apply -f https://raw.githubusercontent.com/aamoyel/vault-unsealer-operator/main/deploy/bundle.yml
   ```

<p align="right">(<a href="#top">back to top</a>)</p>



<!-- USAGE EXAMPLES -->
## Usage
1. First you need to create your secret with your threshold unseal keys. You can find an example at [this link](https://github.com/aamoyel/vault-unsealer-operator/blob/main/config/samples/thresholdKeys.yml) . Here you can find an example:
   ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: thresholdkeys
    type: Opaque
    stringData:
      key1: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
      key2: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```
   Apply this file with `kubectl`
2. (Optionnal) If you have your own PKI and CA certificate, you can create a secret (example file [here](https://github.com/aamoyel/vault-unsealer-operator/blob/main/config/samples/cacertificate.yml)) like that:
   ```yaml
    apiVersion: v1
    kind: Secret
    metadata:
      name: cacertificate
    type: Opaque
    stringData:
      ca.crt: |
        -----BEGIN CERTIFICATE-----
        .....................................
        -----END CERTIFICATE-----
   ```
   Apply this file with `kubectl`
3. Now you can create your config file and custom fields:
   ```yaml
    apiVersion: unsealer.amoyel.fr/v1alpha1
    kind: Unseal
    metadata:
      name: unseal-sample
    spec:
      vaultNodes:
        - https://vault-cluster-node-url-1:8200
        - https://vault-cluster-node-url-2:8200
        - https://vault-cluster-node-url-3:8200
      thresholdKeysSecret: thresholdkeys
      # Optional, but important if you have internal pki for your vault certificate. Secret need to be in the same namespace as this resource
      caCertSecret: cacertificate
      # Optional, set this parameter to true if you want to skip tls certificate verification
      tlsSkipVerify: false
      # Optional
      retryCount: 3
   ```
   Apply this file with `kubectl`

<p align="right">(<a href="#top">back to top</a>)</p>


<!-- Contribute -->
## Contribute
You can create issues on this project if you have any problems or suggestions.

<p align="right">(<a href="#top">back to top</a>)</p>

<!-- LICENSE -->
## License

Distributed under the Apache-2.0 license. See `LICENSE.txt` for more information.

<p align="right">(<a href="#top">back to top</a>)</p>



<!-- CONTACT -->
## Contact

Alan Amoyel - [@AlanAmoyel](https://twitter.com/AlanAmoyel)

Project Link: [https://github.com/aamoyel/vault-unsealer-operator](https://github.com/aamoyel/vault-unsealer-operator)

<p align="right">(<a href="#top">back to top</a>)</p>
