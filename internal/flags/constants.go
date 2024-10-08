//
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

package flags

const (
	// UserManagement enables user management, i.e. invitations, role assignments, etc.
	UserManagement Experiment = "user_management"
	// DockerHubProvider enables the DockerHub provider.
	DockerHubProvider Experiment = "dockerhub_provider"
	// GitLabProvider enables the GitLab provider.
	GitLabProvider Experiment = "gitlab_provider"
	// VulnCheckErrorTemplate enables improved evaluation details
	// messages in the vulncheck rule.
	VulnCheckErrorTemplate Experiment = "vulncheck_error_template"
)
