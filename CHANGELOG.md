# Changelog

## [1.1.0](https://github.com/PolicyStack/PolicyStack/compare/v1.0.0...v1.1.0) (2026-11-28)


### Features

* **acm:** external S3 storage and custom metrics allowlist for observability ([#99](https://github.com/PolicyStack/PolicyStack/issues/99)) ([0eb6b76](https://github.com/PolicyStack/PolicyStack/commit/0eb6b76a7a0dbc7af9a110834f7a8dd46b19fa95))
* **api-hardening:** Added api hardening element for console banners, etcd encryption, tls profiles, etc ([#98](https://github.com/PolicyStack/PolicyStack/issues/98)) ([23b03ef](https://github.com/PolicyStack/PolicyStack/commit/23b03ef9944d55662693ffa5818c70e89a57acbe))
* **ci:** Added gitlab CI pipelines and docs ([#106](https://github.com/PolicyStack/PolicyStack/issues/106)) ([417c14e](https://github.com/PolicyStack/PolicyStack/commit/417c14e9fd7edf6e938469c578b31020be1417a0))
* **ci:** Added gitleaks to CI ([#94](https://github.com/PolicyStack/PolicyStack/issues/94)) ([6d4cf7b](https://github.com/PolicyStack/PolicyStack/commit/6d4cf7ba0e7d49ab015f479e535b9fe773c04f62))
* **ci:** gate stable element changes on maintainer sign-off ([#93](https://github.com/PolicyStack/PolicyStack/issues/93)) ([a64b40a](https://github.com/PolicyStack/PolicyStack/commit/a64b40aab7bed13eb2adf0bba894ca592929d56c))
* **compliance:** Added auto remediation a/ an exclude list ([#107](https://github.com/PolicyStack/PolicyStack/issues/107)) ([dce3d50](https://github.com/PolicyStack/PolicyStack/commit/dce3d50f3b8195e9db0751288c9e4ca44f754137))
* **global-hub:** Added global-hub element ([#101](https://github.com/PolicyStack/PolicyStack/issues/101)) ([0b01655](https://github.com/PolicyStack/PolicyStack/commit/0b0165516651841e6d08c4ba9c52053ed55a345b))
* **logging:** Added log forwarding ([#103](https://github.com/PolicyStack/PolicyStack/issues/103)) ([0ded558](https://github.com/PolicyStack/PolicyStack/commit/0ded558a7328232038719dcd8d98a3187a2a4f6e))
* **loki:** breaking: forwardedLogTypes is exact, removed types stop forwarding ([c212def](https://github.com/PolicyStack/PolicyStack/commit/c212defee77838a68092b657cb3e8223482b22f3))
* **nfd:** breaking: label PCI devices by vendor and use the operator's image ([c212def](https://github.com/PolicyStack/PolicyStack/commit/c212defee77838a68092b657cb3e8223482b22f3))
* **oauth:** Added oauth element w/ kubeadmin removal gate ([#104](https://github.com/PolicyStack/PolicyStack/issues/104)) ([8dbed9e](https://github.com/PolicyStack/PolicyStack/commit/8dbed9effb5722e97d0eb3206e67197cb456be77))
* **onboarding:** Eased onboarding new instances ([#102](https://github.com/PolicyStack/PolicyStack/issues/102)) ([397af5a](https://github.com/PolicyStack/PolicyStack/commit/397af5a38bce2567d572bb5938649073ab671807))
* **quay:** breaking: user initialize is off, set featureUserInitialize to use configure ([c212def](https://github.com/PolicyStack/PolicyStack/commit/c212defee77838a68092b657cb3e8223482b22f3))
* **storage-nodes:** breaking: taint nodes node.ocs.openshift.io/storage, which ODF tolerates ([c212def](https://github.com/PolicyStack/PolicyStack/commit/c212defee77838a68092b657cb3e8223482b22f3))
* **team-gitops:** Updated team-gitops to be able to provide resources per tenant instance ([#105](https://github.com/PolicyStack/PolicyStack/issues/105)) ([0dbda90](https://github.com/PolicyStack/PolicyStack/commit/0dbda906646c7bda7e2ca2af5beb524fa25a7b3d))
* **update-service:** Added update-service element ([#96](https://github.com/PolicyStack/PolicyStack/issues/96)) ([bade0a6](https://github.com/PolicyStack/PolicyStack/commit/bade0a6e2a839fcf53c115904d44f29c1813d290))
* **upgrade:** Added release signatures for disconnected upgrades ([#100](https://github.com/PolicyStack/PolicyStack/issues/100)) ([630a10d](https://github.com/PolicyStack/PolicyStack/commit/630a10d1490b0bec306bd7a7226ee4168d7f31f8))
* **validator:** Added ability to generate an imagesetconfig from available fleet files ([#97](https://github.com/PolicyStack/PolicyStack/issues/97)) ([7f450a1](https://github.com/PolicyStack/PolicyStack/commit/7f450a17ff68db74a8fc61545f4b85bb7a24cfd1))
* **validator:** reject unsafe overrides and validate promoted commits ([c212def](https://github.com/PolicyStack/PolicyStack/commit/c212defee77838a68092b657cb3e8223482b22f3))


### Bug Fixes

* **lifecycle:** keep gates InformOnly whatever values set ([c212def](https://github.com/PolicyStack/PolicyStack/commit/c212defee77838a68092b657cb3e8223482b22f3))
* **loki:** breaking: ClusterLogForwarder is a plain mustonlyhave render; outputs and pipelines added by hand are removed ([a960cbd](https://github.com/PolicyStack/PolicyStack/commit/a960cbd464e39ad2900a28eb11d612e39fba39a3))
* **quay:** breaking: QuayRegistry is a plain mustonlyhave render; list every component kind, set false instead of null ([a960cbd](https://github.com/PolicyStack/PolicyStack/commit/a960cbd464e39ad2900a28eb11d612e39fba39a3))
* replace live-spec merge templates with plain YAML ([#90](https://github.com/PolicyStack/PolicyStack/issues/90)) ([a960cbd](https://github.com/PolicyStack/PolicyStack/commit/a960cbd464e39ad2900a28eb11d612e39fba39a3))
* **validator:** drop the POLICY037 live-spec exception ([a960cbd](https://github.com/PolicyStack/PolicyStack/commit/a960cbd464e39ad2900a28eb11d612e39fba39a3))


### Reverts

* **remediations:** registry allowlist is musthave again, removed entries stay allowed ([a960cbd](https://github.com/PolicyStack/PolicyStack/commit/a960cbd464e39ad2900a28eb11d612e39fba39a3))

## [1.0.0](https://github.com/PolicyStack/PolicyStack/compare/v1.0.0-rc.3...v1.0.0) (2026-11-21)


### Bug Fixes

* drop stale paths from pre-commit exclude ([70f50d7](https://github.com/PolicyStack/PolicyStack/commit/70f50d73ccd4ef5ed5284796dd9b04c9de4c6213))

## [1.0.0-rc.3](https://github.com/PolicyStack/PolicyStack/compare/v1.0.0-rc.2...v1.0.0-rc.3) (2026-11-16)


### Features

* Added PS launcher ([#79](https://github.com/PolicyStack/PolicyStack/issues/79)) ([9adf469](https://github.com/PolicyStack/PolicyStack/commit/9adf469816aa9cede03788899cac9e547361fe23))
* **install:** Added argo sizing + improvements to the appsets ([#82](https://github.com/PolicyStack/PolicyStack/issues/82)) ([75564d1](https://github.com/PolicyStack/PolicyStack/commit/75564d1184d4081d5086ff03f900ad628866d084))
* **lifecycle:** add install.precheck to gate the first build on pre-install checks ([#81](https://github.com/PolicyStack/PolicyStack/issues/81)) ([f218b83](https://github.com/PolicyStack/PolicyStack/commit/f218b83e458d23a732f07e2991f2ec6f2aaf3c4a))

## [1.0.0-rc.2](https://github.com/PolicyStack/PolicyStack/compare/v1.0.0-rc.1...v1.0.0-rc.2) (2026-11-13)


### ⚠ BREAKING CHANGES

* Updated ACS auth to allow for role mapping beyond just the admin group ([#66](https://github.com/PolicyStack/PolicyStack/issues/66))

### Features

* Added element for the File Integrity Operator ([#63](https://github.com/PolicyStack/PolicyStack/issues/63)) ([030df69](https://github.com/PolicyStack/PolicyStack/commit/030df69546ed8b53e6109989480c2a8b82fa322c))
* Updated ACS auth to allow for role mapping beyond just the admin group ([#66](https://github.com/PolicyStack/PolicyStack/issues/66)) ([a02abbd](https://github.com/PolicyStack/PolicyStack/commit/a02abbd7c256177d7fa3cfa6b07861628f670b94))
* Updated elements to streamline code and remove dependency on rawTemplates ([#70](https://github.com/PolicyStack/PolicyStack/issues/70)) ([c39fb79](https://github.com/PolicyStack/PolicyStack/commit/c39fb7944868043fd2f362510add0f1504793a52))


### Bug Fixes

* **deps:** update module github.com/masterminds/semver/v3 to v3.5.0 ([#68](https://github.com/PolicyStack/PolicyStack/issues/68)) ([b8a71a8](https://github.com/PolicyStack/PolicyStack/commit/b8a71a8a7e387ff05a647efb2ba4a3e8b471dc0b))
* Updated go to 1.27.4 for security fix ([#75](https://github.com/PolicyStack/PolicyStack/issues/75)) ([7e9e9bc](https://github.com/PolicyStack/PolicyStack/commit/7e9e9bc2a3d3f1af44e7de5912941a24899f6d92))

## 1.0.0-rc.1 (2026-11-06)


### ⚠ BREAKING CHANGES

* 1.0 contract, copier distribution, in-repo policy-library ([#61](https://github.com/PolicyStack/PolicyStack/issues/61))
* Modified Values to be simpler to develop on ([#59](https://github.com/PolicyStack/PolicyStack/issues/59))

### Features

* 1.0 contract, copier distribution, in-repo policy-library ([#61](https://github.com/PolicyStack/PolicyStack/issues/61)) ([4c104c7](https://github.com/PolicyStack/PolicyStack/commit/4c104c7729d1a4bc6bf767c624f17241878f37f0))
* Modernized CI features ([#55](https://github.com/PolicyStack/PolicyStack/issues/55)) ([d556128](https://github.com/PolicyStack/PolicyStack/commit/d556128a121109e411937c5cdb8a62ba1779281d))
* Modified Values to be simpler to develop on ([#59](https://github.com/PolicyStack/PolicyStack/issues/59)) ([bacea59](https://github.com/PolicyStack/PolicyStack/commit/bacea594b3efe89425a9d9179cbbe7c81651b5cb))
* Rewrote docs to be more tenable in the future ([#60](https://github.com/PolicyStack/PolicyStack/issues/60)) ([972de90](https://github.com/PolicyStack/PolicyStack/commit/972de90f61d616df650c0df32406ba9b8e000ddc))
