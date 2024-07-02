---
sidebar_position: 1
---

# Prerequisites

There are no direct dependencies that are needed to install CEEMS stack. However, CEEMS 
is designed to work with a TSDB and hence, [Prometheus](https://prometheus.io/) or 
[Victoria Metrics](https://victoriametrics.com/) 
must be available to store the scrapped metrics. 

Installation of Prometheus can be found in its [docs](https://prometheus.io/download/) 
and it is out of current scope. 

CEEMS API server uses [SQLite](https://www.sqlite.org/) as DB engine and it it 
shipped by default in most of the OS distributions. CEEMS uses 
[JSON functions](https://www.sqlite.org/json1.html) of SQLite which have been 
integrated by default from version `3.38.0`. Thus, SQLite `>=3.38.0` is a required 
dependency for CEEMS to work properly.
