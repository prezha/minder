// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0
// source: session_store.sql

package db

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
)

const createSessionState = `-- name: CreateSessionState :one
INSERT INTO session_store (provider, project_id, remote_user, session_state, owner_filter, provider_config, encrypted_redirect) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, provider, project_id, port, owner_filter, session_state, created_at, redirect_url, remote_user, encrypted_redirect, provider_config
`

type CreateSessionStateParams struct {
	Provider          string                `json:"provider"`
	ProjectID         uuid.UUID             `json:"project_id"`
	RemoteUser        sql.NullString        `json:"remote_user"`
	SessionState      string                `json:"session_state"`
	OwnerFilter       sql.NullString        `json:"owner_filter"`
	ProviderConfig    []byte                `json:"provider_config"`
	EncryptedRedirect pqtype.NullRawMessage `json:"encrypted_redirect"`
}

func (q *Queries) CreateSessionState(ctx context.Context, arg CreateSessionStateParams) (SessionStore, error) {
	row := q.db.QueryRowContext(ctx, createSessionState,
		arg.Provider,
		arg.ProjectID,
		arg.RemoteUser,
		arg.SessionState,
		arg.OwnerFilter,
		arg.ProviderConfig,
		arg.EncryptedRedirect,
	)
	var i SessionStore
	err := row.Scan(
		&i.ID,
		&i.Provider,
		&i.ProjectID,
		&i.Port,
		&i.OwnerFilter,
		&i.SessionState,
		&i.CreatedAt,
		&i.RedirectUrl,
		&i.RemoteUser,
		&i.EncryptedRedirect,
		&i.ProviderConfig,
	)
	return i, err
}

const deleteExpiredSessionStates = `-- name: DeleteExpiredSessionStates :execrows
DELETE FROM session_store WHERE created_at < NOW() - INTERVAL '1 day'
`

func (q *Queries) DeleteExpiredSessionStates(ctx context.Context) (int64, error) {
	result, err := q.db.ExecContext(ctx, deleteExpiredSessionStates)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

const deleteSessionStateByProjectID = `-- name: DeleteSessionStateByProjectID :exec
DELETE FROM session_store WHERE provider = $1 AND project_id = $2
`

type DeleteSessionStateByProjectIDParams struct {
	Provider  string    `json:"provider"`
	ProjectID uuid.UUID `json:"project_id"`
}

func (q *Queries) DeleteSessionStateByProjectID(ctx context.Context, arg DeleteSessionStateByProjectIDParams) error {
	_, err := q.db.ExecContext(ctx, deleteSessionStateByProjectID, arg.Provider, arg.ProjectID)
	return err
}

const getProjectIDBySessionState = `-- name: GetProjectIDBySessionState :one
SELECT provider, project_id, remote_user, owner_filter, provider_config, redirect_url, encrypted_redirect FROM session_store WHERE session_state = $1
`

type GetProjectIDBySessionStateRow struct {
	Provider          string                `json:"provider"`
	ProjectID         uuid.UUID             `json:"project_id"`
	RemoteUser        sql.NullString        `json:"remote_user"`
	OwnerFilter       sql.NullString        `json:"owner_filter"`
	ProviderConfig    []byte                `json:"provider_config"`
	RedirectUrl       sql.NullString        `json:"redirect_url"`
	EncryptedRedirect pqtype.NullRawMessage `json:"encrypted_redirect"`
}

func (q *Queries) GetProjectIDBySessionState(ctx context.Context, sessionState string) (GetProjectIDBySessionStateRow, error) {
	row := q.db.QueryRowContext(ctx, getProjectIDBySessionState, sessionState)
	var i GetProjectIDBySessionStateRow
	err := row.Scan(
		&i.Provider,
		&i.ProjectID,
		&i.RemoteUser,
		&i.OwnerFilter,
		&i.ProviderConfig,
		&i.RedirectUrl,
		&i.EncryptedRedirect,
	)
	return i, err
}
