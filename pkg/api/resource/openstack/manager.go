// Package openstack implements the fetcher interface to fetch instances from Openstack
// resource manager
package openstack

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/resource"
	config_util "github.com/prometheus/common/config"
)

var novaMicroVersionHeaders = []string{
	"X-OpenStack-Nova-API-Version",
	"OpenStack-API-Version",
}

var osTimeFormat = base.DatetimeLayout + "-0700"

type userProjectsCache struct {
	userModels       []models.User
	projectModels    []models.Project
	userIDNameMap    map[string]string
	projectIDNameMap map[string]string
}

// openstackManager is the struct containing the configuration of a given openstack cluster.
type openstackManager struct {
	logger                     log.Logger
	cluster                    models.Cluster
	apiURLs                    map[string]*url.URL
	client                     *http.Client
	userProjectsCache          userProjectsCache
	userProjectsCacheTTL       time.Duration
	userProjectsLastUpdateTime time.Time
}

type apiConfig struct {
	ComputeAPIURL  string `yaml:"compute_api_url"`
	IdentityAPIURL string `yaml:"identity_api_url"`
}

const openstackVMManager = "openstack"

func init() {
	// Register openstack VM manager
	resource.Register(openstackVMManager, New)
}

// New returns a new openstackManager that returns compute instances.
func New(cluster models.Cluster, logger log.Logger) (resource.Fetcher, error) {
	// Make openstackManager configs from clusters
	openstackManager := openstackManager{
		logger:               logger,
		cluster:              cluster,
		apiURLs:              make(map[string]*url.URL, 2),
		userProjectsCacheTTL: 12 * time.Hour,
	}

	var err error
	// Check if HTTPClientConfig has Nova Micro version header
	headerFound := false

	if cluster.Web.HTTPClientConfig.HTTPHeaders != nil {
		for header := range cluster.Web.HTTPClientConfig.HTTPHeaders.Headers {
			if slices.Contains(novaMicroVersionHeaders, header) {
				headerFound = true

				break
			}
		}
	} else {
		cluster.Web.HTTPClientConfig.HTTPHeaders = &config_util.Headers{
			Headers: make(map[string]config_util.Header),
		}
	}

	// If no Nova Micro Version header found, inject one
	if !headerFound {
		cluster.Web.HTTPClientConfig.HTTPHeaders.Headers[novaMicroVersionHeaders[0]] = config_util.Header{
			Values: []string{"latest"},
		}
	}

	// Make a HTTP client for Openstack from client config
	if openstackManager.client, err = config_util.NewClientFromConfig(cluster.Web.HTTPClientConfig, "openstack"); err != nil {
		level.Error(logger).Log("msg", "Failed to create HTTP client for Openstack cluster", "id", cluster.ID, "err", err)

		return nil, err
	}

	// Fetch compute and identity API URLs from extra_config
	apiConfig := &apiConfig{}
	if err := cluster.Extra.Decode(apiConfig); err != nil {
		level.Error(logger).Log("msg", "Failed to decode extra_config for Openstack cluster", "id", cluster.ID, "err", err)

		return nil, err
	}

	// Ensure we have valid compute and identity API URLs
	// Unwrap original error to avoid leaking sensitive passwords in output
	openstackManager.apiURLs["compute"], err = url.Parse(apiConfig.ComputeAPIURL)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to parse compute service API URL for Openstack cluster", "id", cluster.ID, "err", err)

		return nil, errors.Unwrap(err)
	}

	openstackManager.apiURLs["identity"], err = url.Parse(apiConfig.IdentityAPIURL)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to parse identity service API URL for Openstack cluster", "id", cluster.ID, "err", err)

		return nil, errors.Unwrap(err)
	}

	// // Get initial list of flavors
	// if err = openstackManager.updateFlavors(context.Background()); err != nil {
	// 	return nil, err
	// }

	// Get initial users and projects
	if err = openstackManager.updateUsersProjects(context.Background(), time.Now()); err != nil {
		level.Error(logger).Log("msg", "Failed to update users and projects for Openstack cluster", "id", cluster.ID, "err", err)

		return nil, err
	}

	level.Info(logger).Log("msg", "Fetching VM instances from Openstack cluster", "id", cluster.ID)

	return &openstackManager, nil
}

// FetchUnits fetches instances from openstack.
func (o *openstackManager) FetchUnits(
	ctx context.Context,
	start time.Time,
	end time.Time,
) ([]models.ClusterUnits, error) {
	// Fetch all instances
	instances, err := o.activeInstances(ctx, start, end)
	if err != nil {
		return nil, err
	}

	return []models.ClusterUnits{{Cluster: o.cluster, Units: instances}}, nil
}

// FetchUsersProjects fetches current Openstack users and projects.
func (o *openstackManager) FetchUsersProjects(
	ctx context.Context,
	current time.Time,
) ([]models.ClusterUsers, []models.ClusterProjects, error) {
	// Update user and project data only when cache has expired.
	// We need to make an API request for each user to fetch projects of that user
	// Doing this at each update interval is very resource consuming, so we cache
	// the data for TTL period and update them whenever cache has expired.
	if time.Since(o.userProjectsLastUpdateTime) > o.userProjectsCacheTTL {
		level.Debug(o.logger).Log("msg", "Updating users and projects for Openstack cluster", "id", o.cluster.ID)

		if err := o.updateUsersProjects(ctx, current); err != nil {
			level.Error(o.logger).Log("msg", "Failed to update users and projects data for Openstack cluster", "id", o.cluster.ID, "err", err)
		}
	}

	return []models.ClusterUsers{
			{Cluster: o.cluster, Users: o.userProjectsCache.userModels},
		}, []models.ClusterProjects{
			{Cluster: o.cluster, Projects: o.userProjectsCache.projectModels},
		}, nil
}

// servers endpoint.
func (o *openstackManager) servers() *url.URL {
	return o.apiURLs["compute"].JoinPath("/servers/detail")
}

// // flavors endpoint.
// func (o *openstackManager) flavors() *url.URL {
// 	return o.apiURLs["compute"].JoinPath("/flavors/detail")
// }

// users endpoint.
func (o *openstackManager) users() *url.URL {
	return o.apiURLs["identity"].JoinPath("/v3/users")
}

// user details endpoint.
func (o *openstackManager) userProjects(id string) *url.URL {
	return o.apiURLs["identity"].JoinPath(fmt.Sprintf("/v3/users/%s/projects", id))
}

// ping attempts to ping Openstack compute and identity API servers.
func (o *openstackManager) ping(service string) error {
	if url, ok := o.apiURLs[service]; ok {
		var d net.Dialer

		conn, err := d.Dial("tcp", url.Host)
		if err != nil {
			return fmt.Errorf("openstack service %s is unreachable: %w", service, err)
		}

		defer conn.Close()
	}

	return nil
}
