-- SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
-- SPDX-License-Identifier: Apache-2.0

-- name: GetLatestEvalStateForRuleEntity :one
SELECT eh.* FROM evaluation_rule_entities AS re
JOIN latest_evaluation_statuses AS les ON les.rule_entity_id = re.id
JOIN evaluation_statuses AS eh ON les.evaluation_history_id = eh.id
WHERE re.rule_id = $1 AND re.entity_instance_id = $2
FOR UPDATE;

-- name: InsertEvaluationRuleEntity :one
INSERT INTO evaluation_rule_entities(
    rule_id,
    entity_type,
    entity_instance_id
) VALUES (
    $1,
    $2,
    $3
)
RETURNING id;

-- name: InsertEvaluationStatus :one
INSERT INTO evaluation_statuses(
    rule_entity_id,
    status,
    details,
    checkpoint
) VALUES (
    $1,
    $2,
    $3,
    sqlc.arg(checkpoint)::jsonb
)
RETURNING id;

-- name: UpsertLatestEvaluationStatus :exec
INSERT INTO latest_evaluation_statuses(
    rule_entity_id,
    evaluation_history_id,
    profile_id
) VALUES (
    $1,
    $2,
    $3
)
ON CONFLICT (rule_entity_id) DO UPDATE
SET evaluation_history_id = $2;

-- name: InsertRemediationEvent :exec
INSERT INTO remediation_events(
    evaluation_id,
    status,
    details,
    metadata
) VALUES (
    $1,
    $2,
    $3,
    $4
);

-- name: InsertAlertEvent :exec
INSERT INTO alert_events(
    evaluation_id,
    status,
    details,
    metadata
) VALUES (
    $1,
    $2,
    $3,
    $4
);

-- name: GetEvaluationHistory :one
SELECT s.id::uuid AS evaluation_id,
    s.evaluation_time as evaluated_at,
    ere.entity_type,
    -- entity id
    ere.entity_instance_id as entity_id,
    -- entity name
    ei.name as entity_name,
    j.id as project_id,
    -- rule type, name, and profile
    rt.name AS rule_type,
    ri.name AS rule_name,
    rt.severity_value as rule_severity,
    p.name AS profile_name,
    -- evaluation status and details
    s.status AS evaluation_status,
    s.details AS evaluation_details,
    -- remediation status and details
    re.status AS remediation_status,
    re.details AS remediation_details,
    -- alert status and details
    ae.status AS alert_status,
    ae.details AS alert_details
FROM evaluation_statuses s
    JOIN evaluation_rule_entities ere ON ere.id = s.rule_entity_id
    JOIN rule_instances ri ON ere.rule_id = ri.id
    JOIN rule_type rt ON ri.rule_type_id = rt.id
    JOIN profiles p ON ri.profile_id = p.id
    JOIN entity_instances ei ON ere.entity_instance_id = ei.id
    JOIN projects j ON ei.project_id = j.id
    LEFT JOIN remediation_events re ON re.evaluation_id = s.id
    LEFT JOIN alert_events ae ON ae.evaluation_id = s.id
WHERE s.id = sqlc.arg(evaluation_id) AND j.id = sqlc.arg(project_id);

-- name: ListEvaluationHistory :many
SELECT s.id::uuid AS evaluation_id,
       s.evaluation_time as evaluated_at,
       ere.entity_type,
       -- entity id
        ere.entity_instance_id as entity_id,
       j.id as project_id,
       -- rule type, name, and profile
       rt.name AS rule_type,
       ri.name AS rule_name,
       rt.severity_value as rule_severity,
       p.name AS profile_name,
       p.labels as profile_labels,
       -- evaluation status and details
       s.status AS evaluation_status,
       s.details AS evaluation_details,
       -- remediation status and details
       re.status AS remediation_status,
       re.details AS remediation_details,
       -- alert status and details
       ae.status AS alert_status,
       ae.details AS alert_details
  FROM evaluation_statuses s
  JOIN evaluation_rule_entities ere ON ere.id = s.rule_entity_id
  JOIN rule_instances ri ON ere.rule_id = ri.id
  JOIN rule_type rt ON ri.rule_type_id = rt.id
  JOIN profiles p ON ri.profile_id = p.id
  JOIN entity_instances ei ON ere.entity_instance_id = ei.id
  JOIN projects j ON ei.project_id = j.id
  LEFT JOIN remediation_events re ON re.evaluation_id = s.id
  LEFT JOIN alert_events ae ON ae.evaluation_id = s.id
 WHERE (sqlc.narg(next)::timestamp without time zone IS NULL OR sqlc.narg(next) > s.evaluation_time)
   AND (sqlc.narg(prev)::timestamp without time zone IS NULL OR sqlc.narg(prev) < s.evaluation_time)
   -- inclusion filters
   AND (sqlc.slice(entityTypes)::entities[] IS NULL OR ere.entity_type = ANY(sqlc.slice(entityTypes)::entities[]))
   AND (sqlc.slice(entityNames)::text[] IS NULL OR ei.name = ANY(sqlc.slice(entityNames)::text[]))
   AND (sqlc.slice(profileNames)::text[] IS NULL OR p.name = ANY(sqlc.slice(profileNames)::text[]))
   AND (sqlc.slice(remediations)::remediation_status_types[] IS NULL OR re.status = ANY(sqlc.slice(remediations)::remediation_status_types[]))
   AND (sqlc.slice(alerts)::alert_status_types[] IS NULL OR ae.status = ANY(sqlc.slice(alerts)::alert_status_types[]))
   AND (sqlc.slice(statuses)::eval_status_types[] IS NULL OR s.status = ANY(sqlc.slice(statuses)::eval_status_types[]))
   -- exclusion filters
   AND (sqlc.slice(notEntityTypes)::entities[] IS NULL OR ere.entity_type != ALL(sqlc.slice(notEntityTypes)::entities[]))
   AND (sqlc.slice(notEntityNames)::text[] IS NULL OR ei.name != ALL(sqlc.slice(notEntityNames)::text[]))
   AND (sqlc.slice(notProfileNames)::text[] IS NULL OR p.name != ALL(sqlc.slice(notProfileNames)::text[]))
   AND (sqlc.slice(notRemediations)::remediation_status_types[] IS NULL OR re.status != ALL(sqlc.slice(notRemediations)::remediation_status_types[]))
   AND (sqlc.slice(notAlerts)::alert_status_types[] IS NULL OR ae.status != ALL(sqlc.slice(notAlerts)::alert_status_types[]))
   AND (sqlc.slice(notStatuses)::eval_status_types[] IS NULL OR s.status != ALL(sqlc.slice(notStatuses)::eval_status_types[]))
   -- time range filter
   AND (sqlc.narg(fromts)::timestamp without time zone IS NULL OR s.evaluation_time >= sqlc.narg(fromts))
   AND (sqlc.narg(tots)::timestamp without time zone IS NULL OR  s.evaluation_time < sqlc.narg(tots))
   -- implicit filter by project id
   AND j.id = sqlc.arg(projectId)
   -- implicit filter by profile labels
   AND ((sqlc.slice(labels)::text[] IS NULL AND p.labels = array[]::text[]) -- include only unlabelled records
	OR ((sqlc.slice(labels)::text[] IS NOT NULL AND sqlc.slice(labels)::text[] = array['*']::text[]) -- include all labels
	    OR (sqlc.slice(labels)::text[] IS NOT NULL AND p.labels && sqlc.slice(labels)::text[]) -- include only specified labels
	)
   )
   AND (sqlc.slice(notLabels)::text[] IS NULL OR NOT p.labels && sqlc.slice(notLabels)::text[]) -- exclude only specified labels
 ORDER BY
 CASE WHEN sqlc.narg(next)::timestamp without time zone IS NULL THEN s.evaluation_time END ASC,
 CASE WHEN sqlc.narg(prev)::timestamp without time zone IS NULL THEN s.evaluation_time END DESC
 LIMIT sqlc.arg(size)::bigint;

-- name: ListEvaluationHistoryStaleRecords :many
SELECT s.evaluation_time,
       s.id,
       ere.rule_id,
       -- entity type
       ere.entity_type,
       -- entity id
       ere.entity_instance_id as entity_id
  FROM evaluation_statuses s
       JOIN evaluation_rule_entities ere ON s.rule_entity_id = ere.id
       LEFT JOIN latest_evaluation_statuses l
	   ON l.rule_entity_id = s.rule_entity_id
	   AND l.evaluation_history_id = s.id
 WHERE s.evaluation_time < sqlc.arg(threshold)
  -- the following predicate ensures we get only "stale" records
   AND l.evaluation_history_id IS NULL
 -- listing from oldest to newest
 ORDER BY s.evaluation_time ASC, rule_id ASC, entity_id ASC
 LIMIT sqlc.arg(size)::integer;

-- name: DeleteEvaluationHistoryByIDs :execrows
DELETE FROM evaluation_statuses s
 WHERE s.id = ANY(sqlc.slice(evaluationIds)::uuid[]);
