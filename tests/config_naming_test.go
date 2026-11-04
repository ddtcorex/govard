package tests

import (
	"govard/internal/engine"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestActiveProjectNamesFromContainersWithUnderscores(t *testing.T) {
	containers := []container.Summary{
		{
			Names: []string{"/project-one-db-1"},
			State: "running",
		},
		{
			Names: []string{"/project_two_db_1"},
			State: "running",
		},
		{
			Names: []string{"/other-container"},
			State: "running",
		},
	}

	active := engine.ActiveProjectNamesFromContainersForTest(containers)
	assert.Len(t, active, 2)
	assert.Contains(t, active, "project-one")
	assert.Contains(t, active, "project_two")
}
