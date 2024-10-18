package openstack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/pkg/api/helper"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
)

const (
	chunkSize = 256
)

// updateUsersProjects updates users and projects of a given Openstack cluster.
func (o *openstackManager) updateUsersProjects(ctx context.Context, current time.Time) error {
	// Fetch current users and projects
	if userProjectsCache, err := o.usersProjectsAssoc(ctx, current); err != nil {
		return err
	} else {
		o.userProjectsCache = userProjectsCache
		o.userProjectsLastUpdateTime = current
	}

	return nil
}

// fetchUsers fetches a list of users or specific user from Openstack cluster.
func (o *openstackManager) fetchUsers(ctx context.Context) ([]User, error) {
	// Create a new GET request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		o.users().String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to fetch users for openstack cluster: %w", err)
	}

	// Get response
	resp, err := apiRequest[UsersResponse](req, o.client)
	if err != nil {
		return nil, fmt.Errorf("failed to complete request to fetch users for openstack cluster: %w", err)
	}

	return resp.Users, nil
}

// fetchUserProjects fetches a list of projects of a specific user from Openstack cluster.
func (o *openstackManager) fetchUserProjects(ctx context.Context, userID string) ([]Project, error) {
	// Create a new GET request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		o.userProjects(userID).String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to fetch user projects for openstack cluster: %w", err)
	}

	// Get response
	resp, err := apiRequest[ProjectsResponse](req, o.client)
	if err != nil {
		return nil, fmt.Errorf("failed to complete request to fetch user projects for openstack cluster: %w", err)
	}

	return resp.Projects, nil
}

// fetchUsers fetches a list of users or specific user from Openstack cluster.
func (o *openstackManager) usersProjectsAssoc(ctx context.Context, current time.Time) (userProjectsCache, error) {
	// Check if service is online
	if err := o.ping("identity"); err != nil {
		return userProjectsCache{}, err
	}

	// Current time string
	currentTime := current.Format(osTimeFormat)

	// First get all users
	users, err := o.fetchUsers(ctx)
	if err != nil {
		return userProjectsCache{}, fmt.Errorf("failed to fetch openstack users: %w", err)
	}

	// Get all user IDs
	userIDs := make([]string, len(users))
	usersMap := make(map[string]User, len(users))

	for iuser, user := range users {
		userIDs[iuser] = user.ID
		usersMap[user.ID] = user
	}

	// Chunk by userIDs in chunks of of a given size so that we make
	// concurrent corresponding to chunkSize each time to get projects
	// of each user
	userIDChunks := helper.ChunkBy[string](userIDs, chunkSize)

	// Get user projects
	userProjects := make(map[string][]Project, len(userIDs))

	var allErrs error

	for _, userIDs := range userIDChunks {
		wg := sync.WaitGroup{}
		wg.Add(len(userIDs))

		for _, userID := range userIDs {
			go func(id string) {
				defer wg.Done()

				projects, err := o.fetchUserProjects(ctx, id)

				projectLock.Lock()
				userProjects[id] = projects
				allErrs = errors.Join(allErrs, err)
				projectLock.Unlock()
			}(userID)
		}

		// Wait for all routines before moving to next chunk
		wg.Wait()
	}

	if len(userProjects) == 0 {
		return userProjectsCache{}, allErrs
	}

	if len(userProjects) < len(userIDs) {
		level.Warn(o.logger).Log("msg", "Failed to get projects of few users", "id", o.cluster.ID, "total_users", len(userIDs), "failed_user_project_requests", len(userIDs)-len(userProjects))
	}

	projectUsersList := make(map[string][]string)
	userProjectsList := make(map[string][]string)
	userIDNameMap := make(map[string]string)
	projectIDNameMap := make(map[string]string)

	var projectIDs []string

	for userID, projects := range userProjects {
		for _, project := range projects {
			userProjectsList[userID] = append(userProjectsList[userID], project.Name)
			projectUsersList[project.ID] = append(projectUsersList[project.ID], usersMap[userID].Name)
			projectIDs = append(projectIDs, project.ID)
			userIDNameMap[userID] = usersMap[userID].Name
			projectIDNameMap[project.ID] = project.Name
		}
	}

	// Sort and compact projects
	slices.Sort(projectIDs)
	projectIDs = slices.Compact(projectIDs)

	// Transform map into slice of projects
	projectModels := make([]models.Project, len(projectIDs))

	for iproject, projectID := range projectIDs {
		projectUsers := projectUsersList[projectID]

		// Sort users
		slices.Sort(projectUsers)

		var usersList models.List
		for _, u := range slices.Compact(projectUsers) {
			usersList = append(usersList, u)
		}

		// Make Association
		projectModels[iproject] = models.Project{
			UID:           projectID,
			Name:          projectIDNameMap[projectID],
			Users:         usersList,
			LastUpdatedAt: currentTime,
		}
	}

	// Transform map into slice of users
	userModels := make([]models.User, len(userIDs))

	for iuser, userID := range userIDs {
		userProjects := userProjectsList[userID]

		// Sort projects
		slices.Sort(userProjects)

		var projectsList models.List
		for _, p := range slices.Compact(userProjects) {
			projectsList = append(projectsList, p)
		}

		// Make Association
		userModels[iuser] = models.User{
			UID:           userID,
			Name:          userIDNameMap[userID],
			Projects:      projectsList,
			LastUpdatedAt: currentTime,
		}
	}

	level.Info(o.logger).
		Log("msg", "Openstack user data fetched",
			"cluster_id", o.cluster.ID, "num_users", len(userModels), "num_projects", len(projectModels))

	return userProjectsCache{userModels, projectModels, userIDNameMap, projectIDNameMap}, nil
}
