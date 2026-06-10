# Changelog

## [0.10.4](https://github.com/home-operations/external-dns-unifi-webhook/compare/0.10.3...0.10.4) (2026-06-10)


### Bug Fixes

* **metrics:** bound the HTTP endpoint label to matched routes ([#244](https://github.com/home-operations/external-dns-unifi-webhook/issues/244)) ([f29edcb](https://github.com/home-operations/external-dns-unifi-webhook/commit/f29edcba85a5c64a24d7d6a73edfc0f53f1fc1a0))
* **metrics:** reset stale per-type records gauge after a type's records are deleted ([#235](https://github.com/home-operations/external-dns-unifi-webhook/issues/235)) ([1e3fab7](https://github.com/home-operations/external-dns-unifi-webhook/commit/1e3fab7d1b41f9fc577535c15270fb04d0ac2ad1))
* **metrics:** track consecutive_errors/last_success per operation ([#236](https://github.com/home-operations/external-dns-unifi-webhook/issues/236)) ([9fee152](https://github.com/home-operations/external-dns-unifi-webhook/commit/9fee152ffbb488c2635284fa1a0ee723d720fd30))
* **server:** run the readiness probe detached from the caller's request context ([#232](https://github.com/home-operations/external-dns-unifi-webhook/issues/232)) ([ec3e3b8](https://github.com/home-operations/external-dns-unifi-webhook/commit/ec3e3b81f70502555ed02ca8e0fa80502b2913f8))
* **server:** stop /readyz from leaking upstream detail to anonymous callers ([#242](https://github.com/home-operations/external-dns-unifi-webhook/issues/242)) ([a65595b](https://github.com/home-operations/external-dns-unifi-webhook/commit/a65595bc5b088a6c3d505fb5b80ddcf79fe23fae))
* **unifi:** account for the create-endpoint response body size ([#246](https://github.com/home-operations/external-dns-unifi-webhook/issues/246)) ([ba72eb2](https://github.com/home-operations/external-dns-unifi-webhook/commit/ba72eb23171db86e3a2acf9a538b1f10bb83f9b8))
* **unifi:** bound apply workers, retry attempts, and the connection pool ([#234](https://github.com/home-operations/external-dns-unifi-webhook/issues/234)) ([d1cde15](https://github.com/home-operations/external-dns-unifi-webhook/commit/d1cde15cb179be4e3573f5480ea7a2197a9e117a))
* **unifi:** cap the buffered JSON response read ([#238](https://github.com/home-operations/external-dns-unifi-webhook/issues/238)) ([27ba5a7](https://github.com/home-operations/external-dns-unifi-webhook/commit/27ba5a7f47fa1474813008185cfc47081f38ecc4))
* **unifi:** clear controller-managed TTL on TXT/MX/SRV to stop reconcile churn ([#243](https://github.com/home-operations/external-dns-unifi-webhook/issues/243)) ([47d5f1a](https://github.com/home-operations/external-dns-unifi-webhook/commit/47d5f1a807a0137305184866281e67bfca26a5c5))
* **unifi:** don't retry non-idempotent POST creates on 5xx/network errors ([#233](https://github.com/home-operations/external-dns-unifi-webhook/issues/233)) ([0b8178c](https://github.com/home-operations/external-dns-unifi-webhook/commit/0b8178c2854e68d8244772064a50ad36807d9798))
* **unifi:** guard backoff jitter against a panicking divisor ([#245](https://github.com/home-operations/external-dns-unifi-webhook/issues/245)) ([b07a6be](https://github.com/home-operations/external-dns-unifi-webhook/commit/b07a6be82d2d1126bcef66294ab397f8f12a9acb))
* **unifi:** key record groups by a struct, not concatenated name+type ([#239](https://github.com/home-operations/external-dns-unifi-webhook/issues/239)) ([ad65d42](https://github.com/home-operations/external-dns-unifi-webhook/commit/ad65d422543f154b3efa6a3ac04aeabd2decc2b9))
* **unifi:** size the startup site-resolution timeout to the retry budget ([#240](https://github.com/home-operations/external-dns-unifi-webhook/issues/240)) ([bd6f975](https://github.com/home-operations/external-dns-unifi-webhook/commit/bd6f9750217ec5a2b2b9aa4e8617cf8f3a582d22))
* **unifi:** skip disabled UniFi policies instead of reporting them as live ([#241](https://github.com/home-operations/external-dns-unifi-webhook/issues/241)) ([8ff327c](https://github.com/home-operations/external-dns-unifi-webhook/commit/8ff327c543bc456bd1ec3e0ea210be9915cfa57d))
* **webhook:** log AdjustEndpoints errors; drop no-op Negotiate WriteHeader ([#247](https://github.com/home-operations/external-dns-unifi-webhook/issues/247)) ([af8e136](https://github.com/home-operations/external-dns-unifi-webhook/commit/af8e136a4d6724f200f688ea4d776cc0bfa68fb8)), closes [#227](https://github.com/home-operations/external-dns-unifi-webhook/issues/227) [#228](https://github.com/home-operations/external-dns-unifi-webhook/issues/228)

## [0.10.3](https://github.com/home-operations/external-dns-unifi-webhook/compare/0.10.2...0.10.3) (2026-06-10)


### Features

* **deps:** update module golang.org/x/sync (v0.20.0 → v0.21.0) ([#210](https://github.com/home-operations/external-dns-unifi-webhook/issues/210)) ([d2d80c3](https://github.com/home-operations/external-dns-unifi-webhook/commit/d2d80c3620ee4a8bea6462de25a6470f22e681fd))
* **mise:** update tool oxfmt (0.53.0 → 0.54.0) ([#211](https://github.com/home-operations/external-dns-unifi-webhook/issues/211)) ([8403ccc](https://github.com/home-operations/external-dns-unifi-webhook/commit/8403cccfc77671270743227bac7d1ef42c923dec))


### Bug Fixes

* **mise:** update tool go (1.26.3 → 1.26.4) ([38b8819](https://github.com/home-operations/external-dns-unifi-webhook/commit/38b881966e6a28e6b32cc52557b9a1baba9631ee))


### Miscellaneous Chores

* move mise to mise folder ([45e9f2f](https://github.com/home-operations/external-dns-unifi-webhook/commit/45e9f2f6abd011ee22d4b84fccf8a2f724d77179))
* update release-please-config.json to remove paths ([cf07cb9](https://github.com/home-operations/external-dns-unifi-webhook/commit/cf07cb9e46487a636c7f5c25e7a98175d65b8a34))
* update rlspls workflow name ([9f96b22](https://github.com/home-operations/external-dns-unifi-webhook/commit/9f96b220970727ed6e0cb3841bc9cd9a2cf4d12f))

## [0.10.2](https://github.com/home-operations/external-dns-unifi-webhook/compare/0.10.1...0.10.2) (2026-06-02)


### Features

* **mise:** update tool oxfmt (0.52.0 → 0.53.0) ([0a5c909](https://github.com/home-operations/external-dns-unifi-webhook/commit/0a5c9098ad81768bcabeb70d71fd7e781482cd55))


### Bug Fixes

* **mise:** update tool lefthook (2.1.8 → 2.1.9) ([ee5439f](https://github.com/home-operations/external-dns-unifi-webhook/commit/ee5439f0645f16734f3479c22316e2380ef5747b))


### Miscellaneous Chores

* Delete TESTING.md ([f1b4c05](https://github.com/home-operations/external-dns-unifi-webhook/commit/f1b4c05ff27500622bd009048864b8f1bf6370d4))
* mise lock ([3c8e388](https://github.com/home-operations/external-dns-unifi-webhook/commit/3c8e3886578ab78116d4b624615595ef9467c04b))
* remove 'Contents' section from README ([06d5007](https://github.com/home-operations/external-dns-unifi-webhook/commit/06d50075eb86e5778201e225edc019c3a923c314))
* update UniFi OS minimum version to 5.x ([bea273e](https://github.com/home-operations/external-dns-unifi-webhook/commit/bea273e7f5a18e9df96b2bc0974356317aa728dc))


### Code Refactoring

* webhook/provider generics + unset API key from env after load ([#207](https://github.com/home-operations/external-dns-unifi-webhook/issues/207)) ([90a4723](https://github.com/home-operations/external-dns-unifi-webhook/commit/90a4723737baa061495790fc45cc4a3a1f38fe70))

## [0.10.1](https://github.com/home-operations/external-dns-unifi-webhook/compare/0.10.0...0.10.1) (2026-05-31)


### Features

* support the api.ui.com cloud connector ([#203](https://github.com/home-operations/external-dns-unifi-webhook/issues/203)) ([4f804bb](https://github.com/home-operations/external-dns-unifi-webhook/commit/4f804bbb948110923bff0f99164010a9ec3ade8e))


### Code Refactoring

* code cleanliness, logging & Go 1.26 idioms ([#200](https://github.com/home-operations/external-dns-unifi-webhook/issues/200)) ([08b1415](https://github.com/home-operations/external-dns-unifi-webhook/commit/08b141552677bd570ecf76b633787176005ad697))

## [0.10.0](https://github.com/home-operations/external-dns-unifi-webhook/compare/v0.9.0...0.10.0) (2026-05-31)


### ⚠ BREAKING CHANGES

* code quality and Go 1.26 modernization pass ([#199](https://github.com/home-operations/external-dns-unifi-webhook/issues/199))

### Features

* migration to home-operations org ([570fab4](https://github.com/home-operations/external-dns-unifi-webhook/commit/570fab47be5d2f0b680dd2d9507d43abbf31c83e))


### Code Refactoring

* code quality and Go 1.26 modernization pass ([#199](https://github.com/home-operations/external-dns-unifi-webhook/issues/199)) ([9ecf2d3](https://github.com/home-operations/external-dns-unifi-webhook/commit/9ecf2d3e49b7361b82429890c70b34a886fae5f9))
