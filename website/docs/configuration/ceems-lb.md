---
sidebar_position: 4
---

# CEEMS Load Balancer

The following shows the reference for CEEMS load balancer config. A valid sample 
configuration file can be found in the 
[repo](https://github.com/mahendrapaipuri/ceems/blob/main/build/config/ceems_lb/ceems_lb.yml).

```yaml
# Configuration file to configure CEEMS Load Balancer
#
# This config file has following sections:
#  - `ceems_lb`: Core configuration of CEEMS LB
#  - `ceems_api_server`: Client configuration of CEEMS API server
#  - `clusters`: This is optional config which can be used to validate backends IDs
#
---
ceems_lb:
  # Load balancing strategy. Three possibilites
  #
  # - round-robin
  # - least-connection
  # - resource-based
  #
  # Round robin and least connection are classic strategies.
  # Resource based works based on the query range in the TSDB query. The 
  # query will be proxied to the backend that covers the query_range
  #
  [ strategy: <lbstrategy> | default = round-robin ]

  # List of backends for each cluster
  #
  backends:
    [ - <backend_config> ] 
      

# CEEMS API server config.
# This config is essential to enable access control on the TSDB. By excluding 
# this config, no access control is imposed on the TSDB and a basic load balancing
# based on the chosen strategy will be made.
#
# Essentially, basic access control is implemented by checking the ownership of the
# queried unit. Users that belong to the same project can query the units belong
# to that project. 
# 
# For example, if there is a unit U that belongs to User A and 
# Project P. Any user that belongs to same project P can query for the metrics of unit U
# but not users from other projects.
#
ceems_api_server:
  # The DB contains the information of user and projet units and LB will verify
  # if user/project is the owner of the uuid under request to decide whether to
  # proxy request to backend or not.
  #
  # To identify the current user, X-Grafana-User header will be used that Grafana
  # is capable of sending to the datasource. Grafana essenatially adds this header
  # on the backend server and hence it is not possible for the users to spoof this 
  # header from the browser. 
  # In order to enable this feature, it is essential to set `send_user_header = true`
  # in Grafana config file.
  #
  # If both CEEMS API and CEEMS LB is running on the same host, it is preferable to
  # use the DB directly using `data.path` as DB query is way faster than a API request
  # If both apps are deployed on the same host, ensure that the user running `ceems_lb`
  # has permissions to open CEEMS API data files
  #
  data:
    [ <data_config> ]

  # In the case where CEEMS API and ceems LB are deployed on different hosts, we can
  # still perform access control using CEEMS API server by making a API request to
  # check the ownership of the queried unit. This method should be only preferred when
  # DB cannot be access directly as API request has additional latency than querying DB
  # directly.
  #
  # If both `data.path` and `web.url` are provided, DB will be preferred as it has lower
  # latencies.
  #
  web:
    [ <web_client_config> ]
```

## `<backend_config>`

A `backend_config` allows configuring backend TSDB servers for load balancer.

```yaml
# Identifier of the cluster
#
# This ID must match with the ones defined in `clusters` config. CEEMS API server
# will tag each compute unit from that cluster with this ID and when verifying
# for compute unit ownership, CEEMS LB will use the ID to query for the compute 
# units of that cluster.
#
# This identifier needs to be in the path parameter for requests to CEEMS LB
# to target correct cluster. For instance there are two different clusters,
# say `cluster-0` and `cluster-1`, that have different TSDBs configured. Using CEEMS 
# LB we can load balance the traffic for these two clusters using a single CEEMS LB 
# deployement. However, we need to tell CEEMS LB which cluster to target for the 
# incoming traffic. This is done via path parameter. 
#
# If CEEMS LB is running at http://localhost:9030, then the `cluster-0` is reachable at 
# `http://localhost:9030/cluster-0` and `cluster-1` at `http://localhost:9030/cluster-1`.
# Internally, CEEMS will strip the first part in the URL path, use it to identify
# cluster and proxy the rest of URL path to underlying TSDB backend. 
# Thus, all the requests to `http://localhost:9030/cluster-0` will be load 
# balanced across TSDB backends of `cluster-0`. 
#
id: <idname>

# List of TSDBs for this cluster. Load balancing between these TSDBs will be 
# made based on the strategy chosen.
#
# TLS is not supported for backends. CEEMS LB supports TLS and TLS terminates
# at the LB and requests are proxied to backends on HTTP. 
#
# LB and backend servers are meant to be in the same DMZ so that we do not need
# to encrypt communications. Backends however support basic auth and they can 
# be configured in URL with usual syntax.
#
# An example of configuring the basic auth username and password with backend
# - http://alice:password@localhost:9090
#
tsdb_urls: 
  [ - <host> ]
```
