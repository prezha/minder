// Copyright 2024 Stacklok, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manager

import (
	"fmt"
	"net/http"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	gitlablib "github.com/xanzy/go-gitlab"

	entmsg "github.com/mindersec/minder/internal/entities/handlers/message"
	"github.com/mindersec/minder/internal/entities/properties"
	"github.com/mindersec/minder/internal/events"
	"github.com/mindersec/minder/internal/providers/gitlab"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
)

func (m *providerClassManager) handleRelease(l zerolog.Logger, r *http.Request) error {
	l.Debug().Msg("handling release event")

	releaseEvent := gitlablib.ReleaseEvent{}
	if err := decodeJSONSafe(r.Body, &releaseEvent); err != nil {
		return fmt.Errorf("error decoding release event: %w", err)
	}

	releaseID := releaseEvent.ID
	if releaseID == 0 {
		return fmt.Errorf("release event missing ID")
	}

	tag := releaseEvent.Tag
	if tag == "" {
		return fmt.Errorf("merge request event missing IID")
	}

	rawProjectID := releaseEvent.Project.ID
	if rawProjectID == 0 {
		return fmt.Errorf("merge request event missing project ID")
	}

	// TODO: Should we explicitly handle upcoming/historical releases?

	switch {
	case releaseEvent.Action == "create":
		return m.publishReleaseMessage(releaseID, tag, rawProjectID,
			events.TopicQueueOriginatingEntityAdd)
	case releaseEvent.Action == "update":
		return m.publishReleaseMessage(releaseID, tag, rawProjectID,
			events.TopicQueueRefreshEntityAndEvaluate)
	case releaseEvent.Action == "delete":
		return m.publishReleaseMessage(releaseID, tag, rawProjectID,
			events.TopicQueueOriginatingEntityDelete)
	default:
		return nil
	}
}

func (m *providerClassManager) publishReleaseMessage(
	releaseID int, tag string, rawProjectID int, queueTopic string) error {
	mrUpstreamID := gitlab.FormatPullRequestUpstreamID(releaseID)
	mrProjectID := gitlab.FormatRepositoryUpstreamID(rawProjectID)

	// Form identifying properties
	identifyingProps, err := properties.NewProperties(map[string]any{
		properties.PropertyUpstreamID: mrUpstreamID,
		gitlab.ReleasePropertyTag:     tag,
		gitlab.PullRequestProjectID:   mrProjectID,
	})
	if err != nil {
		return fmt.Errorf("error creating identifying properties: %w", err)
	}

	repoIdentifyingProps, err := properties.NewProperties(map[string]any{
		properties.PropertyUpstreamID: mrProjectID,
	})
	if err != nil {
		return fmt.Errorf("error creating repo identifying properties: %w", err)
	}

	// Form message to publish
	outm := entmsg.NewEntityRefreshAndDoMessage()
	outm.WithEntity(minderv1.Entity_ENTITY_RELEASE, identifyingProps)
	outm.WithOriginator(minderv1.Entity_ENTITY_REPOSITORIES, repoIdentifyingProps)
	outm.WithProviderClassHint(gitlab.Class)

	// Convert message for publishing
	msgID := uuid.New().String()
	msg := message.NewMessage(msgID, nil)
	if err := outm.ToMessage(msg); err != nil {
		return fmt.Errorf("error converting message to protobuf: %w", err)
	}

	// Publish message
	if err := m.pub.Publish(queueTopic, msg); err != nil {
		return fmt.Errorf("error publishing refresh and eval message: %w", err)
	}

	return nil
}