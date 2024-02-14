# Changelog

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
