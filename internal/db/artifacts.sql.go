// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0
// source: artifacts.sql

package db

import (
	"context"

	"github.com/google/uuid"
)

const deleteArtifact = `-- name: DeleteArtifact :exec
DELETE FROM artifacts
WHERE id = $1
`

func (q *Queries) DeleteArtifact(ctx context.Context, id uuid.UUID) error {
	_, err := q.db.ExecContext(ctx, deleteArtifact, id)
	return err
}

const getArtifactByID = `-- name: GetArtifactByID :one
SELECT id, repository_id, artifact_name, artifact_type, artifact_visibility, created_at, updated_at, project_id, provider_id, provider_name FROM artifacts
WHERE artifacts.id = $1 AND artifacts.project_id = $2
`

type GetArtifactByIDParams struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
}

func (q *Queries) GetArtifactByID(ctx context.Context, arg GetArtifactByIDParams) (Artifact, error) {
	row := q.db.QueryRowContext(ctx, getArtifactByID, arg.ID, arg.ProjectID)
	var i Artifact
	err := row.Scan(
		&i.ID,
		&i.RepositoryID,
		&i.ArtifactName,
		&i.ArtifactType,
		&i.ArtifactVisibility,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.ProjectID,
		&i.ProviderID,
		&i.ProviderName,
	)
	return i, err
}

const getArtifactByName = `-- name: GetArtifactByName :one
SELECT id, repository_id, artifact_name, artifact_type, artifact_visibility, created_at, updated_at, project_id, provider_id, provider_name FROM artifacts 
WHERE lower(artifacts.artifact_name) = lower($3)
AND artifacts.repository_id = $1 AND artifacts.project_id = $2
`

type GetArtifactByNameParams struct {
	RepositoryID uuid.NullUUID `json:"repository_id"`
	ProjectID    uuid.UUID     `json:"project_id"`
	ArtifactName string        `json:"artifact_name"`
}

func (q *Queries) GetArtifactByName(ctx context.Context, arg GetArtifactByNameParams) (Artifact, error) {
	row := q.db.QueryRowContext(ctx, getArtifactByName, arg.RepositoryID, arg.ProjectID, arg.ArtifactName)
	var i Artifact
	err := row.Scan(
		&i.ID,
		&i.RepositoryID,
		&i.ArtifactName,
		&i.ArtifactType,
		&i.ArtifactVisibility,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.ProjectID,
		&i.ProviderID,
		&i.ProviderName,
	)
	return i, err
}

const listArtifactsByRepoID = `-- name: ListArtifactsByRepoID :many
SELECT id, repository_id, artifact_name, artifact_type, artifact_visibility, created_at, updated_at, project_id, provider_id, provider_name FROM artifacts
WHERE repository_id = $1
ORDER BY id
`

func (q *Queries) ListArtifactsByRepoID(ctx context.Context, repositoryID uuid.NullUUID) ([]Artifact, error) {
	rows, err := q.db.QueryContext(ctx, listArtifactsByRepoID, repositoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Artifact{}
	for rows.Next() {
		var i Artifact
		if err := rows.Scan(
			&i.ID,
			&i.RepositoryID,
			&i.ArtifactName,
			&i.ArtifactType,
			&i.ArtifactVisibility,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.ProjectID,
			&i.ProviderID,
			&i.ProviderName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const upsertArtifact = `-- name: UpsertArtifact :one
INSERT INTO artifacts (
    repository_id,
    artifact_name,
    artifact_type,
    artifact_visibility,
    project_id,
    provider_id,
    provider_name
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (project_id, LOWER(artifact_name))
DO UPDATE SET
    artifact_type = $3,
    artifact_visibility = $4
WHERE artifacts.repository_id = $1 AND artifacts.artifact_name = $2
RETURNING id, repository_id, artifact_name, artifact_type, artifact_visibility, created_at, updated_at, project_id, provider_id, provider_name
`

type UpsertArtifactParams struct {
	RepositoryID       uuid.NullUUID `json:"repository_id"`
	ArtifactName       string        `json:"artifact_name"`
	ArtifactType       string        `json:"artifact_type"`
	ArtifactVisibility string        `json:"artifact_visibility"`
	ProjectID          uuid.UUID     `json:"project_id"`
	ProviderID         uuid.UUID     `json:"provider_id"`
	ProviderName       string        `json:"provider_name"`
}

func (q *Queries) UpsertArtifact(ctx context.Context, arg UpsertArtifactParams) (Artifact, error) {
	row := q.db.QueryRowContext(ctx, upsertArtifact,
		arg.RepositoryID,
		arg.ArtifactName,
		arg.ArtifactType,
		arg.ArtifactVisibility,
		arg.ProjectID,
		arg.ProviderID,
		arg.ProviderName,
	)
	var i Artifact
	err := row.Scan(
		&i.ID,
		&i.RepositoryID,
		&i.ArtifactName,
		&i.ArtifactType,
		&i.ArtifactVisibility,
		&i.CreatedAt,
		&i.UpdatedAt,
		&i.ProjectID,
		&i.ProviderID,
		&i.ProviderName,
	)
	return i, err
}