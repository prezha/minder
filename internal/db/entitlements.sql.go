// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0
// source: entitlements.sql

package db

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

const getFeatureInProject = `-- name: GetFeatureInProject :one

SELECT f.settings FROM entitlements e
INNER JOIN features f ON f.name = e.feature
WHERE e.project_id = $1::UUID AND e.feature = $2::TEXT
`

type GetFeatureInProjectParams struct {
	ProjectID uuid.UUID `json:"project_id"`
	Feature   string    `json:"feature"`
}

// GetFeatureInProject verifies if a feature is available for a specific project.
// It returns the settings for the feature if it is available.
func (q *Queries) GetFeatureInProject(ctx context.Context, arg GetFeatureInProjectParams) (json.RawMessage, error) {
	row := q.db.QueryRowContext(ctx, getFeatureInProject, arg.ProjectID, arg.Feature)
	var settings json.RawMessage
	err := row.Scan(&settings)
	return settings, err
}