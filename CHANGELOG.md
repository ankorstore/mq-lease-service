# Changelog

## [1.0.2](https://github.com/ankorstore/mq-lease-service/compare/v1.0.1...v1.0.2) (2025-02-13)


### Bug Fixes

* goreleaser ([#5](https://github.com/ankorstore/mq-lease-service/issues/5)) ([aca5a05](https://github.com/ankorstore/mq-lease-service/commit/aca5a05bb65ac60cc27d54d97010e9ea875adb62))

## [1.0.1](https://github.com/ankorstore/mq-lease-service/compare/v1.0.0...v1.0.1) (2025-02-13)


### Bug Fixes

* goreleaser ([#2](https://github.com/ankorstore/mq-lease-service/issues/2)) ([a78007f](https://github.com/ankorstore/mq-lease-service/commit/a78007f67223a5d6296d557ec9d34432774addb8))

## 1.0.0 (2025-02-13)


### Features

* add Dockerfile ([#11](https://github.com/ankorstore/mq-lease-service/issues/11)) ([fe8e5fb](https://github.com/ankorstore/mq-lease-service/commit/fe8e5fba4f55726d2c8b34ca0e5e07acc2cf49a2))
* add fiber handler utils functions ([be6b00a](https://github.com/ankorstore/mq-lease-service/commit/be6b00ae49d85f655c00429e155f7d1ad2956eb9))
* add gha ([7ec86b2](https://github.com/ankorstore/mq-lease-service/commit/7ec86b2ace4d65519efc4e81033073497f995df0))
* add GithubAction for interacting with the lease service ([#10](https://github.com/ankorstore/mq-lease-service/issues/10)) ([e9a4ded](https://github.com/ankorstore/mq-lease-service/commit/e9a4ded16e3c4424ec9fbcf55cb270d724ef6424))
* add gofiber handlers ([c149837](https://github.com/ankorstore/mq-lease-service/commit/c149837e4effb11058172fa8a2032b7077ac3723))
* add provider config in API responses ([137909a](https://github.com/ankorstore/mq-lease-service/commit/137909ad5586e25a7cc805d908a86978e599df97))
* add release workflow ([39d24bb](https://github.com/ankorstore/mq-lease-service/commit/39d24bbdeb6f961abe792885eab4c8a0bf9f82a6))
* add stacked pull requests info in API resources ([ce689f0](https://github.com/ankorstore/mq-lease-service/commit/ce689f0c1a92391ffe5d85e5bbc481280edf2341))
* add yaml config file for server ([9fb9431](https://github.com/ankorstore/mq-lease-service/commit/9fb943142fe58a466b67ea712fdb83a94f64ce47))
* badger storage, e2e, improved testability ([#4](https://github.com/ankorstore/mq-lease-service/issues/4)) ([3ca6190](https://github.com/ankorstore/mq-lease-service/commit/3ca61904161b460e2f0b29186baa8c209d8be99a))
* **ci:** add linter, pre-commit & CI github workflows ([a3a35f5](https://github.com/ankorstore/mq-lease-service/commit/a3a35f51f28ccd8043194a41a821942d9c5a5d4d))
* delay lease acquisition by N poll requests ([#23](https://github.com/ankorstore/mq-lease-service/issues/23)) ([86d59bd](https://github.com/ankorstore/mq-lease-service/commit/86d59bd2e14b9cfb3bd470dda8fd2baa4bc5f262))
* implement basic structure + leaseprovider service ([577f1a2](https://github.com/ankorstore/mq-lease-service/commit/577f1a2ad6dd0787cfcd74d7b6c51567f6a77804))
* k8s probes endpoints ([#6](https://github.com/ankorstore/mq-lease-service/issues/6)) ([1294f19](https://github.com/ankorstore/mq-lease-service/commit/1294f190aa056bddc25902b9d89d5c38534588ee))
* native support for basicauth ([0bb0370](https://github.com/ankorstore/mq-lease-service/commit/0bb0370767a2c458a058a68e2e04dca1587b872c))
* prometheus metrics, route names in logger & panic bug fix ([#5](https://github.com/ankorstore/mq-lease-service/issues/5)) ([e9a9d21](https://github.com/ankorstore/mq-lease-service/commit/e9a9d218ddc228994caf98fd0df8e1b3fb42db87))
* provider clear endpoint ([f6bb9cb](https://github.com/ankorstore/mq-lease-service/commit/f6bb9cbdfd11f219eb84f55a2ed4621fe6851a71))
* use fiber handler utils in existing handlers + add payload validation ([d3de85e](https://github.com/ankorstore/mq-lease-service/commit/d3de85e0beec93f02ceff182eb859f8dca9e4329))


### Bug Fixes

* add DelayLeaseAssignmentBy management in e2e tests ([e6b3cfb](https://github.com/ankorstore/mq-lease-service/commit/e6b3cfbe73dff7a44060f40179d351c06932cf94))
* CR on error text ([418bfdd](https://github.com/ankorstore/mq-lease-service/commit/418bfdd80e622ae71cc003a2286cea759626777d))
* CRs ([f81e6ea](https://github.com/ankorstore/mq-lease-service/commit/f81e6ea33e3ded8226573610588bb6fb652baf14))
* don't return stacked_pull_requests if the request status is not ([7ab4e05](https://github.com/ankorstore/mq-lease-service/commit/7ab4e05f3d9ad92b6df4ddc1aa80af1329070e57))
* expose metrics ([#27](https://github.com/ankorstore/mq-lease-service/issues/27)) ([9022d61](https://github.com/ankorstore/mq-lease-service/commit/9022d61a835510e866f81499432a63ce8d507c86))
* fiber log middleware log fields + lint ([10b26af](https://github.com/ankorstore/mq-lease-service/commit/10b26af9309ff46c8cde8991d04a5945447b1067))
* fiber log middleware traceparent considerartion ([580ccde](https://github.com/ankorstore/mq-lease-service/commit/580ccdee228982696b971464d858c4b82762337c))
* prevent race condition when failure is before the end of the mq ([0620121](https://github.com/ankorstore/mq-lease-service/commit/0620121847018cb996516f08c7b8ec03170cc66a))
* readme ([75b9c35](https://github.com/ankorstore/mq-lease-service/commit/75b9c35ba2f71476ee55300e35a65e6c865fb83a))
* readme diagram ([f21c656](https://github.com/ankorstore/mq-lease-service/commit/f21c65623ca2286d4169c6de2ee4b855163d533a))
* review feedback ([08f387c](https://github.com/ankorstore/mq-lease-service/commit/08f387c7755ba49faf0d8de6a5b487125beafb31))
* Segfault linked to wrong TTL handling ([#8](https://github.com/ankorstore/mq-lease-service/issues/8)) ([678cb08](https://github.com/ankorstore/mq-lease-service/commit/678cb08f2d3f05bfb95ebc92e12de1b560c435df))
* set lastUpdatedAt for the first time when the first request is registered ([8cef22b](https://github.com/ankorstore/mq-lease-service/commit/8cef22be9418b31f924cac49edb7ce7ac80086e4))
* STM & sequence diagram ([face4ff](https://github.com/ankorstore/mq-lease-service/commit/face4ff353779ae6be9f0911d84b1722af39aba1))
* tests ([65b982b](https://github.com/ankorstore/mq-lease-service/commit/65b982bc2cfa964aa7674f04ce688808037301b4))
* to verify: last updated wasn't updated in some insert ([2ac189a](https://github.com/ankorstore/mq-lease-service/commit/2ac189a8f293aad4b5401b5a35bda1af575e9d45))
* typo ([5ecdd56](https://github.com/ankorstore/mq-lease-service/commit/5ecdd56026b3738c6266f750aebb00741d66cc02))
* update pre-commit-config.yaml ([#25](https://github.com/ankorstore/mq-lease-service/issues/25)) ([c353275](https://github.com/ankorstore/mq-lease-service/commit/c353275150762379fc1fdf225628d8f4f015138b))
* updated ([367da13](https://github.com/ankorstore/mq-lease-service/commit/367da13fd6c628a3a2a859bc3a3f42772fc59ce8))
