# Changelog

## 0.5.3 / 2025-01-24

- [BUGFIX] Minor corrections in SLURM fetcher and TSDB updater [#280](https://github.com/mahendrapaipuri/ceems/pull/280) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Set MIG instance in a separate label, when present [#279](https://github.com/mahendrapaipuri/ceems/pull/279) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] More configurability on tsdb updater's query batching [#277](https://github.com/mahendrapaipuri/ceems/pull/277) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Handle running query parameter correctly [#271](https://github.com/mahendrapaipuri/ceems/pull/271) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] TSDB retention period estimation [#270](https://github.com/mahendrapaipuri/ceems/pull/270) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#273](https://github.com/mahendrapaipuri/ceems/pull/273), [#274](https://github.com/mahendrapaipuri/ceems/pull/274), [#276](https://github.com/mahendrapaipuri/ceems/pull/276), [#278](https://github.com/mahendrapaipuri/ceems/pull/278) ([@dependabot](https://github.com/dependabot))

## 0.5.2 / 2025-01-17

- [BUGFIX] Re-establish session when token invalidates for Redfish collector [#268](https://github.com/mahendrapaipuri/ceems/pull/268) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] TSDB estimate batch size dynamically and update OWID data [#262](https://github.com/mahendrapaipuri/ceems/pull/262) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#264](https://github.com/mahendrapaipuri/ceems/pull/264), [#265](https://github.com/mahendrapaipuri/ceems/pull/265), [#269](https://github.com/mahendrapaipuri/ceems/pull/269) ([@dependabot](https://github.com/dependabot))

## 0.5.1 / 2025-01-08

- [FEATURE] Add Cray's pm_counters collector [#261](https://github.com/mahendrapaipuri/ceems/pull/261) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Use total swap as limit when cgroup sets it as max [#260](https://github.com/mahendrapaipuri/ceems/pull/260) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Configurable Timezone for CEEMS DB [#253](https://github.com/mahendrapaipuri/ceems/pull/253) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Support for Pyroscope servers for CEEMS LB [#252](https://github.com/mahendrapaipuri/ceems/pull/252) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#246](https://github.com/mahendrapaipuri/ceems/pull/246), [#247](https://github.com/mahendrapaipuri/ceems/pull/247), [#248](https://github.com/mahendrapaipuri/ceems/pull/248), [#249](https://github.com/mahendrapaipuri/ceems/pull/249), [#250](https://github.com/mahendrapaipuri/ceems/pull/250), [#251](https://github.com/mahendrapaipuri/ceems/pull/251), [#254](https://github.com/mahendrapaipuri/ceems/pull/254), [#255](https://github.com/mahendrapaipuri/ceems/pull/255), [#256](https://github.com/mahendrapaipuri/ceems/pull/256) ([@dependabot](https://github.com/dependabot))

## 0.5.0 / 2024-12-12

- [BUGFIX] Support IPMI package on 32/64 bit platforms [#245](https://github.com/mahendrapaipuri/ceems/pull/245) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Upgrade Go to 1.23.x [#244](https://github.com/mahendrapaipuri/ceems/pull/244) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Update dockerfile to include redfish_proxy [#243](https://github.com/mahendrapaipuri/ceems/pull/243) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add Redfish Collector [#240](https://github.com/mahendrapaipuri/ceems/pull/240) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Pure go IPMI implementation using OpenIPMI interface [#238](https://github.com/mahendrapaipuri/ceems/pull/238) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Embed demo Grafana in iframe in documentation welcome page [#233](https://github.com/mahendrapaipuri/ceems/pull/233) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Report usage statistics by taking running units into account [#232](https://github.com/mahendrapaipuri/ceems/pull/232) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Support automatic token rotation for Openstack [#227](https://github.com/mahendrapaipuri/ceems/pull/227) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Prioritize SLURM_JOB_GPUS env for GPU mapping [#221](https://github.com/mahendrapaipuri/ceems/pull/221) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Migrate to slog logging [#211](https://github.com/mahendrapaipuri/ceems/pull/211) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Implement correct scaling of perf hardware counters [#210](https://github.com/mahendrapaipuri/ceems/pull/210) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#212](https://github.com/mahendrapaipuri/ceems/pull/212), [#213](https://github.com/mahendrapaipuri/ceems/pull/213), [#215](https://github.com/mahendrapaipuri/ceems/pull/215), [#222](https://github.com/mahendrapaipuri/ceems/pull/222), [#225](https://github.com/mahendrapaipuri/ceems/pull/225), [#226](https://github.com/mahendrapaipuri/ceems/pull/226), [#228](https://github.com/mahendrapaipuri/ceems/pull/228), [#229](https://github.com/mahendrapaipuri/ceems/pull/229), [#236](https://github.com/mahendrapaipuri/ceems/pull/236) ([@dependabot](https://github.com/dependabot)), [#237](https://github.com/mahendrapaipuri/ceems/pull/237), [#241](https://github.com/mahendrapaipuri/ceems/pull/241) ([@dependabot](https://github.com/dependabot)), [#242](https://github.com/mahendrapaipuri/ceems/pull/242) ([@dependabot](https://github.com/dependabot))

## 0.5.0-rc.2 / 2024-10-31

- [BUFGIX] Scale perf counters based on times enabled and ran [#209](https://github.com/mahendrapaipuri/ceems/pull/209) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.5.0-rc.1 / 2024-10-29

- [MAINT] Major refactor to improve performance of exporter [#204](https://github.com/mahendrapaipuri/ceems/pull/204) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#205](https://github.com/mahendrapaipuri/ceems/pull/205), [#206](https://github.com/mahendrapaipuri/ceems/pull/206), [#207](https://github.com/mahendrapaipuri/ceems/pull/207) ([@dependabot](https://github.com/dependabot))

## 0.4.1 / 2024-10-25

- [FEATURE] Use custom header to find target cluster [#203](https://github.com/mahendrapaipuri/ceems/pull/203) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.4.0 / 2024-10-23

- [FEATURE] Add support for HTTP alloy discovery [#198](https://github.com/mahendrapaipuri/ceems/pull/198) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add openstack resource manager support to API server [#196](https://github.com/mahendrapaipuri/ceems/pull/196) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add support for MIG and vGPUs in exporter [#193](https://github.com/mahendrapaipuri/ceems/pull/193) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Export power limit from RAPL counters [#189](https://github.com/mahendrapaipuri/ceems/pull/189) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add libvirt collector [#186](https://github.com/mahendrapaipuri/ceems/pull/186) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add RDMA collector [#182](https://github.com/mahendrapaipuri/ceems/pull/182) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Fix cmd execution mode detection [#181](https://github.com/mahendrapaipuri/ceems/pull/181) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Hide test related CLI flags [#180](https://github.com/mahendrapaipuri/ceems/pull/180) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add ebpf support for mips,ppc and risc archs [#179](https://github.com/mahendrapaipuri/ceems/pull/179) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#183](https://github.com/mahendrapaipuri/ceems/pull/183), [#184](https://github.com/mahendrapaipuri/ceems/pull/184), [#185](https://github.com/mahendrapaipuri/ceems/pull/185), [#192](https://github.com/mahendrapaipuri/ceems/pull/192), [#194](https://github.com/mahendrapaipuri/ceems/pull/194), [#199](https://github.com/mahendrapaipuri/ceems/pull/199), [#200](https://github.com/mahendrapaipuri/ceems/pull/200), [#201](https://github.com/mahendrapaipuri/ceems/pull/201), [#202](https://github.com/mahendrapaipuri/ceems/pull/202) ([@dependabot](https://github.com/dependabot))

## 0.3.1 / 2024-10-03

- [BUGFIX] Fix cmd execution mode detection [#181](https://github.com/mahendrapaipuri/ceems/pull/181) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Hide test related CLI flags [#180](https://github.com/mahendrapaipuri/ceems/pull/180) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEAT] Add ebpf support for mips,ppc and risc archs [#179](https://github.com/mahendrapaipuri/ceems/pull/179) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.3.0 / 2024-09-28

- [CI] Move docs workflow to separate file [#178](https://github.com/mahendrapaipuri/ceems/pull/178) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Verify TSDB actual retention period [#177](https://github.com/mahendrapaipuri/ceems/pull/177) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Make CEEMS apps capability aware [#176](https://github.com/mahendrapaipuri/ceems/pull/176) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Remove unnecessary log lines [#167](https://github.com/mahendrapaipuri/ceems/pull/167) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Refactor slurm collector organization [#155](https://github.com/mahendrapaipuri/ceems/pull/155) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Graceful exporter shutdown and misc fixes [#153](https://github.com/mahendrapaipuri/ceems/pull/153) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use consistent CLI flags for exporter [#144](https://github.com/mahendrapaipuri/ceems/pull/144) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add perf collector that exports perf metrics [#137](https://github.com/mahendrapaipuri/ceems/pull/137) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Bump dependencies [#138](https://github.com/mahendrapaipuri/ceems/pull/138), [#139](https://github.com/mahendrapaipuri/ceems/pull/139), [#140](https://github.com/mahendrapaipuri/ceems/pull/140), [#141](https://github.com/mahendrapaipuri/ceems/pull/141), [#142](https://github.com/mahendrapaipuri/ceems/pull/142), [#143](https://github.com/mahendrapaipuri/ceems/pull/143), [#145](https://github.com/mahendrapaipuri/ceems/pull/145), [#146](https://github.com/mahendrapaipuri/ceems/pull/146), [#147](https://github.com/mahendrapaipuri/ceems/pull/147), [#148](https://github.com/mahendrapaipuri/ceems/pull/148), [#149](https://github.com/mahendrapaipuri/ceems/pull/149), [#150](https://github.com/mahendrapaipuri/ceems/pull/150), [#151](https://github.com/mahendrapaipuri/ceems/pull/151) , [#152](https://github.com/mahendrapaipuri/ceems/pull/152), [#154](https://github.com/mahendrapaipuri/ceems/pull/154), [#157](https://github.com/mahendrapaipuri/ceems/pull/157), [#158](https://github.com/mahendrapaipuri/ceems/pull/158), [#159](https://github.com/mahendrapaipuri/ceems/pull/159), [#160](https://github.com/mahendrapaipuri/ceems/pull/160), [#161](https://github.com/mahendrapaipuri/ceems/pull/161), [#162](https://github.com/mahendrapaipuri/ceems/pull/162), [#163](https://github.com/mahendrapaipuri/ceems/pull/163), [#164](https://github.com/mahendrapaipuri/ceems/pull/164), [#168](https://github.com/mahendrapaipuri/ceems/pull/168), [#169](https://github.com/mahendrapaipuri/ceems/pull/169), [#171](https://github.com/mahendrapaipuri/ceems/pull/171), [#172](https://github.com/mahendrapaipuri/ceems/pull/172), [#173](https://github.com/mahendrapaipuri/ceems/pull/173), [#174](https://github.com/mahendrapaipuri/ceems/pull/174), [#175](https://github.com/mahendrapaipuri/ceems/pull/175) ([@dependabot](https://github.com/dependabot))

## 0.2.1 / 2024-08-17

- [BUGFIX] Fix setting sysprocattr correctly based on command [#136](https://github.com/mahendrapaipuri/ceems/pull/136) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.2.0 / 2024-08-11

- [FEATURE] Pass context to downstream functions [#133](https://github.com/mahendrapaipuri/ceems/pull/133) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Enable more linters [#132](https://github.com/mahendrapaipuri/ceems/pull/132) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] General maintenance [#129](https://github.com/mahendrapaipuri/ceems/pull/129) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use native JSON functions in aggregate query [#128](https://github.com/mahendrapaipuri/ceems/pull/128) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Stats API endpoint [#127](https://github.com/mahendrapaipuri/ceems/pull/127) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Cache current usage query result [#122](https://github.com/mahendrapaipuri/ceems/pull/122) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.1 / 2024-07-24

- [MAINT] DB query performance improvements [#113](https://github.com/mahendrapaipuri/ceems/pull/113) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Fix metric aggregation [#112](https://github.com/mahendrapaipuri/ceems/pull/112) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Incremental improvements on API server [#111](https://github.com/mahendrapaipuri/ceems/pull/111) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Dont cache failed requests for emissions [#110](https://github.com/mahendrapaipuri/ceems/pull/110) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Upgrade to Go 1.22.x [#109](https://github.com/mahendrapaipuri/ceems/pull/109) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [TEST] Migrate to testify for unit tests [#108](https://github.com/mahendrapaipuri/ceems/pull/108) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0 / 2024-07-06

- [BUGFIX] Build swag using native arch in cross build [#107](https://github.com/mahendrapaipuri/ceems/pull/107) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Avoid building test bins for release workflows [#106](https://github.com/mahendrapaipuri/ceems/pull/106) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Fix tsdb updater [#104](https://github.com/mahendrapaipuri/ceems/pull/104) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Store metrics as map in DB [#102](https://github.com/mahendrapaipuri/ceems/pull/102) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Improve docs on Slurm collector [#101](https://github.com/mahendrapaipuri/ceems/pull/101) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Improve docs on Slurm collector [#101](https://github.com/mahendrapaipuri/ceems/pull/101) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Test DEB packages in CI [#100](https://github.com/mahendrapaipuri/ceems/pull/100) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Extract go code for CodeQL analysis [#99](https://github.com/mahendrapaipuri/ceems/pull/99) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Enforce rules on cluster and updater IDs [#98](https://github.com/mahendrapaipuri/ceems/pull/98) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Update Docs [#97](https://github.com/mahendrapaipuri/ceems/pull/97) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Add CodeQL workflow [#96](https://github.com/mahendrapaipuri/ceems/pull/96) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add user and project tables to DB [#95](https://github.com/mahendrapaipuri/ceems/pull/95) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Multicluster support [#94](https://github.com/mahendrapaipuri/ceems/pull/94) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] General maintenance and enhancements [#92](https://github.com/mahendrapaipuri/ceems/pull/92) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Add swagger docs [#90](https://github.com/mahendrapaipuri/ceems/pull/90) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Setup docs website [#88](https://github.com/mahendrapaipuri/ceems/pull/88) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [DOCS] Publish README to registries [#87](https://github.com/mahendrapaipuri/ceems/pull/87) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use weighted mean for agg stats [#86](https://github.com/mahendrapaipuri/ceems/pull/86) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Make and publish container images [#85](https://github.com/mahendrapaipuri/ceems/pull/85) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add demo end points [#84](https://github.com/mahendrapaipuri/ceems/pull/84) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Support DB and API modes for access control [#83](https://github.com/mahendrapaipuri/ceems/pull/83) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Enhancement api server [#78](https://github.com/mahendrapaipuri/ceems/pull/78) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add `cpu_per_core_count` metric to CPU collector [#76](https://github.com/mahendrapaipuri/ceems/pull/76) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add `last_updated_at` col in usage table [#75](https://github.com/mahendrapaipuri/ceems/pull/75) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Use auth middleware for LB [#74](https://github.com/mahendrapaipuri/ceems/pull/74) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add recording rules for Prometheus [#67](https://github.com/mahendrapaipuri/ceems/pull/67) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Ensure non-negative values in agg metrics [#66](https://github.com/mahendrapaipuri/ceems/pull/66) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0-rc.6 / 2024-04-04

- [REFACTOR] Use generic name in metric names [#65](https://github.com/mahendrapaipuri/ceems/pull/65) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use custom float64 type [#62](https://github.com/mahendrapaipuri/ceems/pull/62) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Configurable TSDB updater queries and DB migrations [#64](https://github.com/mahendrapaipuri/ceems/pull/64) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use custom float64 type [#62](https://github.com/mahendrapaipuri/ceems/pull/62) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [TEST] Add unit tests [#61](https://github.com/mahendrapaipuri/ceems/pull/61) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Fix go coverage badge in README [#60](https://github.com/mahendrapaipuri/ceems/pull/60) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [CI] Add coverage badge to README [#59](https://github.com/mahendrapaipuri/ceems/pull/59) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Debian and RPM packaging  [#58](https://github.com/mahendrapaipuri/ceems/pull/58) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add a default resource manager [#57](https://github.com/mahendrapaipuri/ceems/pull/57) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Auto detect IPMI command and add support for capmc [#56](https://github.com/mahendrapaipuri/ceems/pull/44) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] chore: Several enhancements for CEEMS LB [#54](https://github.com/mahendrapaipuri/ceems/pull/54) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Incremental metrics aggregation [#53](https://github.com/mahendrapaipuri/ceems/pull/53) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Backend Auth for CEEMS LB  [#52](https://github.com/mahendrapaipuri/ceems/pull/52) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0-rc.5 / 2024-03-02

- [FEATURE] feat: Support RDMA stats in exporter [#45](https://github.com/mahendrapaipuri/ceems/pull/45) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Rename stats pkg to api [#44](https://github.com/mahendrapaipuri/ceems/pull/44) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] TSDB Load Balancer [#43](https://github.com/mahendrapaipuri/ceems/pull/43) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] DB migrations support [#42](https://github.com/mahendrapaipuri/ceems/pull/42) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [MAINT] Refactor DB schema [#41](https://github.com/mahendrapaipuri/ceems/pull/41) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0-rc.4 / 2024-02-18

- [BUGFIX] Misc bugfixes [#40](https://github.com/mahendrapaipuri/ceems/pull/40) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Support different IPMI implementations [#39](https://github.com/mahendrapaipuri/ceems/pull/39) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Rename pkg to ceems [#38](https://github.com/mahendrapaipuri/ceems/pull/38) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Cache job props for SLURM collector [#37](https://github.com/mahendrapaipuri/ceems/pull/37) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Extend DB schema to add new fields [#36](https://github.com/mahendrapaipuri/ceems/pull/36) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Backup DB at configured interval [#35](https://github.com/mahendrapaipuri/ceems/pull/35) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0-rc.3 / 2024-01-22

- [REFACTOR] refactor: Remove support for job steps [#34](https://github.com/mahendrapaipuri/ceems/pull/34) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Fetch admin users from grafana [#33](https://github.com/mahendrapaipuri/ceems/pull/33) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Rename pkg [#32](https://github.com/mahendrapaipuri/ceems/pull/32) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Enhancements in collector [#31](https://github.com/mahendrapaipuri/ceems/pull/31) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Fix tsdb cleanup [#30](https://github.com/mahendrapaipuri/ceems/pull/30) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Split node metrics into separate collectors [#29](https://github.com/mahendrapaipuri/ceems/pull/29) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add total procs cputime metric [#28](https://github.com/mahendrapaipuri/ceems/pull/28) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add support for TSDB vacuuming [#27](https://github.com/mahendrapaipuri/ceems/pull/27) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use a separate time series for each job for mapping GPU [#26](https://github.com/mahendrapaipuri/ceems/pull/26) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use query builder [#25](https://github.com/mahendrapaipuri/ceems/pull/25) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Job stats server enhancements [#24](https://github.com/mahendrapaipuri/ceems/pull/24) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Use cgroups v2 pkg [#23](https://github.com/mahendrapaipuri/ceems/pull/23) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Rename emissions factory from source to provider [#22](https://github.com/mahendrapaipuri/ceems/pull/22) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Export min and max power readings from ipmi [#21](https://github.com/mahendrapaipuri/ceems/pull/21) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add hostname label to exporter metrics [#20](https://github.com/mahendrapaipuri/ceems/pull/20) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] Correct env var name for getting gpu index [#19](https://github.com/mahendrapaipuri/ceems/pull/19) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0-rc.2 / 2023-12-26

- [REFACTOR] Refactor jobstats pkg [#18](https://github.com/mahendrapaipuri/ceems/pull/18) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Use default http client for requests for emissions collector [#16](https://github.com/mahendrapaipuri/ceems/pull/16) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [REFACTOR] Refactor emissions pkg [#16](https://github.com/mahendrapaipuri/ceems/pull/16) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [BUGFIX] bugfix: Correctly parse SLURM nodelist range string [#15](https://github.com/mahendrapaipuri/ceems/pull/15) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))

## 0.1.0-rc.1 / 2023-12-20

- [FEATURE] Bug fixes and refactoring [#14](https://github.com/mahendrapaipuri/ceems/pull/14) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Misc improvements [#13](https://github.com/mahendrapaipuri/ceems/pull/13) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Merge job stats DB and server commands [#12](https://github.com/mahendrapaipuri/ceems/pull/12) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Support GPU jobID map from /proc [#11](https://github.com/mahendrapaipuri/ceems/pull/11) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add Runtime pkg [#10](https://github.com/mahendrapaipuri/ceems/pull/10) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Misc features [#9](https://github.com/mahendrapaipuri/ceems/pull/9) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add API server to serve job stats [#8](https://github.com/mahendrapaipuri/ceems/pull/8) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add jobstats pkg [#7](https://github.com/mahendrapaipuri/ceems/pull/7) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use pkg structure [#6](https://github.com/mahendrapaipuri/ceems/pull/6) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Use UID and GID to job labels [#5](https://github.com/mahendrapaipuri/ceems/pull/5) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Reorganise repo [#4](https://github.com/mahendrapaipuri/ceems/pull/4) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add unique jobid label for SLURM jobs [#3](https://github.com/mahendrapaipuri/ceems/pull/3) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] Add Emission collector [#2](https://github.com/mahendrapaipuri/ceems/pull/2) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
- [FEATURE] CircleCI setup [#1](https://github.com/mahendrapaipuri/ceems/pull/1) ([@mahendrapaipuri](https://github.com/mahendrapaipuri))
