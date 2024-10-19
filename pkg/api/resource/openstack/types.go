package openstack

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func init() {
	// If we are in CI env, use fixed time location
	// for e2e tests
	if os.Getenv("CI") != "" {
		currentLocation, _ = time.LoadLocation("CET")
	} else {
		currentLocation = time.Now().Location()
	}

	fmt.Println("QQQQ", os.Getenv("CI"), currentLocation, time.Now()) //nolint:forbidigo
}

const RFC3339MilliNoZ = "2006-01-02T15:04:05.999999"

var currentLocation *time.Location

type JSONRFC3339MilliNoZ time.Time

func (jt *JSONRFC3339MilliNoZ) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	if s == "" {
		return nil
	}

	t, err := time.Parse(RFC3339MilliNoZ, s)
	if err != nil {
		return err
	}

	// Convert the UTC time to local
	*jt = JSONRFC3339MilliNoZ(
		time.Date(
			t.Year(),
			t.Month(),
			t.Day(),
			t.Hour(),
			t.Minute(),
			t.Second(),
			t.Nanosecond(),
			currentLocation,
		),
	)

	return nil
}

// Server represents a server/instance in the OpenStack cloud.
type Server struct {
	// ID uniquely identifies this server amongst all other servers,
	// including those not accessible to the current tenant.
	ID string `json:"id"`

	// TenantID identifies the tenant owning this server resource.
	TenantID string `json:"tenant_id"`

	// UserID uniquely identifies the user account owning the tenant.
	UserID string `json:"user_id"`

	// Name contains the human-readable name for the server.
	Name string `json:"name"`

	// Updated and Created contain ISO-8601 timestamps of when the state of the
	// server last changed, and when it was created.
	UpdatedAt time.Time `json:"updated"`
	CreatedAt time.Time `json:"created"`

	// HostID is the host where the server is located in the cloud.
	HostID string `json:"hostid"`

	// Status contains the current operational status of the server,
	// such as IN_PROGRESS or ACTIVE.
	Status string `json:"status"`

	// Flavor refers to a JSON object, which itself indicates the hardware
	// configuration of the deployed server.
	Flavor Flavor `json:"flavor"`

	// Metadata includes a list of all user-specified key-value pairs attached
	// to the server.
	Metadata map[string]string `json:"metadata"`

	// AttachedVolumes includes the volume attachments of this instance
	AttachedVolumes []AttachedVolume `json:"os-extended-volumes:volumes_attached"`

	// Fault contains failure information about a server.
	Fault Fault `json:"fault"`

	// Tags is a slice/list of string tags in a server.
	// The requires microversion 2.26 or later.
	Tags []string `json:"tags"`

	// ServerGroups is a slice of strings containing the UUIDs of the
	// server groups to which the server belongs. Currently this can
	// contain at most one entry.
	// New in microversion 2.71
	ServerGroups []string `json:"server_groups"`

	// Host is the host/hypervisor that the instance is hosted on.
	Host string `json:"OS-EXT-SRV-ATTR:host"`

	// InstanceName is the name of the instance.
	InstanceName string `json:"OS-EXT-SRV-ATTR:instance_name"`

	// HypervisorHostname is the hostname of the host/hypervisor that the
	// instance is hosted on.
	HypervisorHostname string `json:"OS-EXT-SRV-ATTR:hypervisor_hostname"`

	// ReservationID is the reservation ID of the instance.
	// This requires microversion 2.3 or later.
	ReservationID string `json:"OS-EXT-SRV-ATTR:reservation_id"`

	// LaunchIndex is the launch index of the instance.
	// This requires microversion 2.3 or later.
	LaunchIndex int `json:"OS-EXT-SRV-ATTR:launch_index"`

	TaskState  string     `json:"OS-EXT-STS:task_state"`
	VMState    string     `json:"OS-EXT-STS:vm_state"`
	PowerState PowerState `json:"OS-EXT-STS:power_state"`

	LaunchedAt   time.Time `json:"-"`
	TerminatedAt time.Time `json:"-"`

	// AvailabilityZone is the availability zone the server is in.
	AvailabilityZone string `json:"OS-EXT-AZ:availability_zone"`
}

func (r *Server) UnmarshalJSON(b []byte) error {
	type tmp Server

	var s struct {
		tmp
		LaunchedAt   JSONRFC3339MilliNoZ `json:"OS-SRV-USG:launched_at"`
		TerminatedAt JSONRFC3339MilliNoZ `json:"OS-SRV-USG:terminated_at"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*r = Server(s.tmp)

	r.LaunchedAt = time.Time(s.LaunchedAt)
	r.TerminatedAt = time.Time(s.TerminatedAt)

	// Convert CreatedAt and UpdatedAt to local times
	// Seems like returned values are always in UTC
	r.CreatedAt = time.Date(
		r.CreatedAt.Year(),
		r.CreatedAt.Month(),
		r.CreatedAt.Day(),
		r.CreatedAt.Hour(),
		r.CreatedAt.Minute(),
		r.CreatedAt.Second(),
		r.CreatedAt.Nanosecond(),
		currentLocation,
	)
	r.UpdatedAt = time.Date(
		r.UpdatedAt.Year(),
		r.UpdatedAt.Month(),
		r.UpdatedAt.Day(),
		r.UpdatedAt.Hour(),
		r.UpdatedAt.Minute(),
		r.UpdatedAt.Second(),
		r.UpdatedAt.Nanosecond(),
		currentLocation,
	)
	fmt.Println("BBB", r.CreatedAt, r.UpdatedAt, r.LaunchedAt, r.TerminatedAt, r.CreatedAt.Format(osTimeFormat), r.UpdatedAt.Format(osTimeFormat), r.LaunchedAt.Format(osTimeFormat), r.TerminatedAt.Format(osTimeFormat), currentLocation) //nolint:forbidigo

	return err
}

type AttachedVolume struct {
	ID string `json:"id"`
}

type Fault struct {
	Code    int       `json:"code"`
	Created time.Time `json:"created"`
	Details string    `json:"details"`
	Message string    `json:"message"`
}

type PowerState int

const (
	NOSTATE = iota
	RUNNING
	_UNUSED1 //nolint:stylecheck
	PAUSED
	SHUTDOWN
	_UNUSED2 //nolint:stylecheck
	CRASHED
	SUSPENDED
)

func (r PowerState) String() string {
	switch r {
	case NOSTATE:
		return "NOSTATE"
	case RUNNING:
		return "RUNNING"
	case PAUSED:
		return "PAUSED"
	case SHUTDOWN:
		return "SHUTDOWN"
	case CRASHED:
		return "CRASHED"
	case SUSPENDED:
		return "SUSPENDED"
	case _UNUSED1, _UNUSED2:
		return "_UNUSED"
	default:
		return "N/A"
	}
}

type ServersResponse struct {
	Servers []Server `json:"servers"`
}

// Flavor represent (virtual) hardware configurations for server resources
// in a region.
type Flavor struct {
	// ID is the flavor's unique ID.
	ID string `json:"id"`

	// Disk is the amount of root disk, measured in GB.
	Disk int `json:"disk"`

	// RAM is the amount of memory, measured in MB.
	RAM int `json:"ram"`

	// Name is the name of the flavor.
	Name string `json:"original_name"`

	// RxTxFactor describes bandwidth alterations of the flavor.
	RxTxFactor float64 `json:"rxtx_factor"`

	// Swap is the amount of swap space, measured in MB.
	Swap int `json:"-"`

	// VCPUs indicates how many (virtual) CPUs are available for this flavor.
	VCPUs int `json:"vcpus"`

	// IsPublic indicates whether the flavor is public.
	IsPublic bool `json:"os-flavor-access:is_public"`

	// Ephemeral is the amount of ephemeral disk space, measured in GB.
	Ephemeral int `json:"OS-FLV-EXT-DATA:ephemeral"`

	// Description is a free form description of the flavor. Limited to
	// 65535 characters in length. Only printable characters are allowed.
	// New in version 2.55
	Description string `json:"description"`

	// Properties is a dictionary of the flavor’s extra-specs key-and-value
	// pairs. This will only be included if the user is allowed by policy to
	// index flavor extra_specs
	// New in version 2.61
	ExtraSpecs map[string]string `json:"extra_specs"`
}

func (r *Flavor) UnmarshalJSON(b []byte) error {
	type tmp Flavor

	var s struct {
		tmp
		Swap any `json:"swap"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*r = Flavor(s.tmp)

	switch t := s.Swap.(type) {
	case float64:
		r.Swap = int(t)
	case string:
		switch t {
		case "":
			r.Swap = 0
		default:
			swap, err := strconv.ParseFloat(t, 64)
			if err != nil {
				return err
			}

			r.Swap = int(swap)
		}
	}

	return nil
}

type FlavorsResponse struct {
	Flavors []Flavor `json:"flavors"`
}

// User represents a User in the OpenStack Identity Service.
type User struct {
	// DefaultProjectID is the ID of the default project of the user.
	DefaultProjectID string `json:"default_project_id"`

	// Description is the description of the user.
	Description string `json:"description"`

	// DomainID is the domain ID the user belongs to.
	DomainID string `json:"domain_id"`

	// Enabled is whether or not the user is enabled.
	Enabled bool `json:"enabled"`

	// ID is the unique ID of the user.
	ID string `json:"id"`

	// Links contains referencing links to the user.
	Links map[string]any `json:"links"`

	// Name is the name of the user.
	Name string `json:"name"`
}

type UsersResponse struct {
	Users []User `json:"users"`
}

// Project represents an OpenStack Identity Project.
type Project struct {
	// IsDomain indicates whether the project is a domain.
	IsDomain bool `json:"is_domain"`

	// Description is the description of the project.
	Description string `json:"description"`

	// DomainID is the domain ID the project belongs to.
	DomainID string `json:"domain_id"`

	// Enabled is whether or not the project is enabled.
	Enabled bool `json:"enabled"`

	// ID is the unique ID of the project.
	ID string `json:"id"`

	// Name is the name of the project.
	Name string `json:"name"`

	// ParentID is the parent_id of the project.
	ParentID string `json:"parent_id"`

	// Tags is the list of tags associated with the project.
	Tags []string `json:"tags,omitempty"`
}

type ProjectsResponse struct {
	Projects []Project `json:"projects"`
}
