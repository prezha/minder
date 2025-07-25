// SPDX-FileCopyrightText: Copyright 2023 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

// Package rest provides the REST remediation engine
package rest

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/v63/github"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/reflect/protoreflect"

	engerrors "github.com/mindersec/minder/internal/engine/errors"
	"github.com/mindersec/minder/internal/engine/interfaces"
	"github.com/mindersec/minder/internal/util"
	pb "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	"github.com/mindersec/minder/pkg/profiles/models"
	provifv1 "github.com/mindersec/minder/pkg/providers/v1"
)

const (
	// RemediateType is the type of the REST remediation engine
	RemediateType = "rest"

	// methodBytesLimit is the maximum number of bytes for the HTTP method
	methodBytesLimit = 10

	// endpointBytesLimit is the maximum number of bytes for the endpoint
	endpointBytesLimit = 1024

	// bodyBytesLimit is the maximum number of bytes for the body
	bodyBytesLimit = 5120
)

// Remediator keeps the status for a rule type that uses REST remediation
type Remediator struct {
	actionType       interfaces.ActionType
	method           *util.SafeTemplate
	cli              provifv1.REST
	endpointTemplate *util.SafeTemplate
	bodyTemplate     *util.SafeTemplate
	// Setting defines the current action setting. e.g. dry-run, on, off
	setting models.ActionOpt
}

// NewRestRemediate creates a new REST rule data ingest engine
func NewRestRemediate(
	actionType interfaces.ActionType, restCfg *pb.RestType, cli provifv1.REST,
	setting models.ActionOpt,
) (*Remediator, error) {
	if actionType == "" {
		return nil, fmt.Errorf("action type cannot be empty")
	}

	endpointTmpl, err := util.NewSafeTextTemplate(&restCfg.Endpoint, "endpoint")
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}

	var bodyTmpl *util.SafeTemplate
	if restCfg.Body != nil {
		bodyTmpl, err = util.NewSafeTextTemplate(restCfg.Body, "body")
		if err != nil {
			return nil, fmt.Errorf("invalid body: %w", err)
		}
	}

	methodStr := cmp.Or(restCfg.Method, http.MethodPatch)
	methodTemplate, err := util.NewSafeTextTemplate(&methodStr, "method")
	if err != nil {
		return nil, fmt.Errorf("invalid method: %w", err)
	}

	return &Remediator{
		cli:              cli,
		actionType:       actionType,
		method:           methodTemplate,
		endpointTemplate: endpointTmpl,
		bodyTemplate:     bodyTmpl,
		setting:          setting,
	}, nil
}

// EndpointTemplateParams is the parameters for the REST endpoint template
type EndpointTemplateParams struct {
	// Entity is the entity to be evaluated
	Entity any
	// Profile is the parameters to be used in the template
	Profile map[string]any
	// Params are the rule instance parameters
	Params map[string]any
	// EvalResultOutput is the output from the rule evaluation
	EvalResultOutput any
}

// Class returns the action type of the remediation engine
func (r *Remediator) Class() interfaces.ActionType {
	return r.actionType
}

// Type returns the action subtype of the remediation engine
func (*Remediator) Type() string {
	return RemediateType
}

// GetOnOffState returns the alert action state read from the profile
func (r *Remediator) GetOnOffState() models.ActionOpt {
	return models.ActionOptOrDefault(r.setting, models.ActionOptOff)
}

// Do perform the remediation
func (r *Remediator) Do(
	ctx context.Context,
	cmd interfaces.ActionCmd,
	entity protoreflect.ProtoMessage,
	params interfaces.ActionsParams,
	_ *json.RawMessage,
) (json.RawMessage, error) {
	// Remediating through rest doesn't really have a turn-off behavior so
	// only proceed with the remediation if the command is to turn on the action
	if cmd != interfaces.ActionCmdOn {
		return nil, engerrors.ErrActionSkipped
	}

	retp := &EndpointTemplateParams{
		Entity:  entity,
		Profile: params.GetRule().Def,
		Params:  params.GetRule().Params,
	}
	if params.GetEvalResult() != nil {
		retp.EvalResultOutput = params.GetEvalResult().Output
	}

	method := new(bytes.Buffer)
	if err := r.method.Execute(ctx, method, retp, methodBytesLimit); err != nil {
		return nil, fmt.Errorf("cannot execute method template: %w", err)
	}

	endpoint := new(bytes.Buffer)
	if err := r.endpointTemplate.Execute(ctx, endpoint, retp, endpointBytesLimit); err != nil {
		return nil, fmt.Errorf("cannot execute endpoint template: %w", err)
	}

	body := new(bytes.Buffer)
	if r.bodyTemplate != nil {
		if err := r.bodyTemplate.Execute(ctx, body, retp, bodyBytesLimit); err != nil {
			return nil, fmt.Errorf("cannot execute endpoint template: %w", err)
		}
	}

	zerolog.Ctx(ctx).Debug().
		Msgf("remediating with endpoint: [%s] and body [%+v]", endpoint.String(), body.String())

	var err error
	switch r.setting {
	case models.ActionOptOn:
		err = r.run(ctx, method.String(), endpoint.String(), body.Bytes())
	case models.ActionOptDryRun:
		err = r.dryRun(ctx, method.String(), endpoint.String(), body.String())
	case models.ActionOptOff, models.ActionOptUnknown:
		err = errors.New("unexpected action")
	}
	return nil, err
}

func (r *Remediator) run(ctx context.Context, method string, endpoint string, body []byte) error {
	// create an empty map, not a nil map to avoid passing nil to NewRequest
	bodyJson := make(map[string]any)

	if len(body) > 0 {
		err := json.Unmarshal(body, &bodyJson)
		if err != nil {
			return fmt.Errorf("cannot unmarshal body: %w", err)
		}
	}

	req, err := r.cli.NewRequest(strings.ToUpper(method), endpoint, bodyJson)
	if err != nil {
		return fmt.Errorf("cannot create request: %w", err)
	}

	resp, err := r.cli.Do(ctx, req)
	if err != nil {
		var respErr *github.ErrorResponse
		if errors.As(err, &respErr) {
			zerolog.Ctx(ctx).Error().Msgf("Error message: %v", respErr.Message)
			for _, e := range respErr.Errors {
				zerolog.Ctx(ctx).Error().Msgf("Field: %s, Message: %s", e.Field, e.Message)
			}
		}
		return fmt.Errorf("cannot make request: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("cannot close response body: %v", err)
		}
	}()
	// Translate the http status code response to an error
	if engerrors.HTTPErrorCodeToErr(resp.StatusCode) != nil {
		return engerrors.NewErrActionFailed("remediation failed: %s", err)
	}
	return nil
}

func (r *Remediator) dryRun(ctx context.Context, method, endpoint, body string) error {
	curlCmd, err := util.GenerateCurlCommand(ctx, method, r.cli.GetBaseURL(), endpoint, body)
	if err != nil {
		return fmt.Errorf("cannot generate curl command: %w", err)
	}

	log.Printf("run the following curl command: \n%s\n", curlCmd)
	return nil
}
