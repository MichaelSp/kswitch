
## Verifying release artifacts

All release artifacts are signed with [cosign](https://github.com/sigstore/cosign)
using keyless signing. No private key is stored — the signing identity is bound to
this GitHub Actions workflow run and recorded in the
[Sigstore Rekor](https://rekor.sigstore.dev) transparency log.

Download the binary, its `.sig` signature and `.pem` certificate from the assets
below, then verify with:

```sh
cosign verify-blob \
  --certificate <artifact>.pem \
  --signature   <artifact>.sig \
  --certificate-identity-regexp 'https://github.com/MichaelSp/kswitch' \
  --certificate-oidc-issuer    'https://token.actions.githubusercontent.com' \
  <artifact>
```
