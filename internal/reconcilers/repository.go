// SPDX-FileCopyrightText: Copyright 2023 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package reconcilers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	entityMessage "github.com/mindersec/minder/internal/entities/handlers/message"
	"github.com/mindersec/minder/internal/reconcilers/messages"
	"github.com/mindersec/minder/pkg/eventer/constants"
)

// handleRepoReconcilerEvent handles events coming from the reconciler topic
func (r *Reconciler) handleRepoReconcilerEvent(msg *message.Message) error {
	var evt messages.RepoReconcilerEvent
	if err := json.Unmarshal(msg.Payload, &evt); err != nil {
		// We don't return the event since there's no use
		// retrying it if it's invalid.
		log.Printf("error unmarshalling event: %v", err)
		return nil
	}

	// validate event
	validate := validator.New()
	if err := validate.Struct(&evt); err != nil {
		// We don't return the event since there's no use
		// retrying it if it's invalid.
		log.Printf("error validating event: %v", err)
		return nil
	}

	ctx := msg.Context()
	log.Printf("handling reconciler event for project %s and repository %s", evt.Project.String(), evt.EntityID.String())
	return r.handleRepositoryReconcilerEvent(ctx, &evt)
}

// HandleArtifactsReconcilerEvent recreates the artifacts belonging to
// an specific repository
// nolint: gocyclo
func (r *Reconciler) handleRepositoryReconcilerEvent(ctx context.Context, evt *messages.RepoReconcilerEvent) error {
	entRefresh := entityMessage.NewEntityRefreshAndDoMessage().
		WithEntityID(evt.EntityID)

	m := message.NewMessage(uuid.New().String(), nil)
	if err := entRefresh.ToMessage(m); err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error marshalling message")
		// no point in retrying, so we return nil
		return nil
	}

	if evt.EntityID == uuid.Nil {
		// this might happen if we process old messages during an upgrade, but there's no point in retrying
		zerolog.Ctx(ctx).Error().Msg("entityID is nil")
		return nil
	}

	m.SetContext(ctx)
	if err := r.evt.Publish(constants.TopicQueueRefreshEntityByIDAndEvaluate, m); err != nil {
		// we retry in case watermill is having a bad day
		return fmt.Errorf("error publishing message: %w", err)
	}

	return nil
}
