// SPDX-FileCopyrightText: Copyright 2023 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package repo

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/mindersec/minder/internal/util/cli"
	"github.com/mindersec/minder/internal/util/cli/table"
	"github.com/mindersec/minder/internal/util/cli/table/layouts"
	"github.com/mindersec/minder/internal/util/ptr"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
)

var repoRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a repository",
	Long:  `The repo register subcommand is used to register a repo within Minder.`,
	RunE:  cli.GRPCClientWrapRunE(RegisterCmd),
}

// RegisterCmd represents the register command to register a repo with minder
//
//nolint:gocyclo
func RegisterCmd(ctx context.Context, cmd *cobra.Command, _ []string, conn *grpc.ClientConn) error {
	repoClient := minderv1.NewRepositoryServiceClient(conn)

	provider := viper.GetString("provider")
	project := viper.GetString("project")
	inputRepoList := viper.GetStringSlice("name")
	registerAll := viper.GetBool("all")

	// No longer print usage on returned error, since we've parsed our inputs
	// See https://github.com/spf13/cobra/issues/340#issuecomment-374617413
	cmd.SilenceUsage = true

	if registerAll {
		if len(inputRepoList) > 0 {
			cmd.Println("Cannot use --all and --name together")
			return nil
		}

		providerClient := minderv1.NewProvidersServiceClient(conn)

		ret, err := enableAutoRegistration(ctx, providerClient, project, provider)
		if err != nil {
			cmd.Println("Enabling auto registration failed for one or more providers.")
		} else {
			cmd.Println("Enabling auto registration succeeded.")
		}
		if len(ret.alreadyEnabled) > 0 {
			cmd.Println(fmt.Sprintf("Auto registration is already enabled for: %v", ret.alreadyEnabled))
		}
		if len(ret.failedProviders) > 0 {
			cmd.Println(fmt.Sprintf("Failed to enable: %v", ret.failedProviders))
		}
		if len(ret.updatedProviders) > 0 {
			cmd.Println(fmt.Sprintf("Successfully enabled: %v", ret.updatedProviders))
			cmd.Println("Use `minder repo list` to check the list of registered repositories.")
		}
		return nil
	}

	for _, repo := range inputRepoList {
		if err := cli.ValidateRepositoryName(repo); err != nil {
			return cli.MessageAndError("Invalid repository name", err)
		}
	}

	// Fetch remote repos, both registered and unregistered.
	repos, err := fetchRepos(ctx, provider, project, repoClient)
	if err != nil {
		return cli.MessageAndError("Error getting registered repos", err)
	}

	// Maps for filtering
	registeredRepos := make(map[string]*minderv1.UpstreamRepositoryRef)
	unregisteredRepos := make(map[string]*minderv1.UpstreamRepositoryRef)
	for _, repo := range repos {
		key := cli.GetRepositoryName(repo.Owner, repo.Name)
		if repo.Registered {
			registeredRepos[key] = repo
		} else {
			unregisteredRepos[key] = repo
		}
	}

	// No repos left to register, exit cleanly
	if len(unregisteredRepos) == 0 {
		cmd.Println("No repos left to register")
		return nil
	}

	var selectedRepos []*minderv1.UpstreamRepositoryRef
	if len(inputRepoList) > 0 {
		// Repositories are provided as --name options
		for _, repo := range inputRepoList {
			// Repo was already registered, report it to
			// user and move on
			if registeredRepos[repo] != nil {
				cmd.Printf("Repository %s is already registered\n", repo)
			}

			// Repo was not already registered, add it to
			// those to process.
			if repoRef := unregisteredRepos[repo]; repoRef != nil {
				selectedRepos = append(selectedRepos, repoRef)
			}
		}
	} else {
		cmd.Printf(
			"Found %d remote repositories: %d registered and %d unregistered.\n",
			len(registeredRepos)+len(unregisteredRepos),
			len(registeredRepos),
			len(unregisteredRepos),
		)

		var err error
		selectedRepos, err = selectReposInteractively(
			cmd,
			unregisteredRepos,
		)
		if err != nil {
			return cli.MessageAndError("Error getting selected repositories", err)
		}
	}

	results, warnings := registerRepos(project, repoClient, selectedRepos)
	printWarnings(cmd, warnings)

	printRepoRegistrationStatus(cmd, results)
	return nil
}

func fetchRepos(
	ctx context.Context,
	provider string,
	project string,
	client minderv1.RepositoryServiceClient,
) ([]*minderv1.UpstreamRepositoryRef, error) {
	var provPtr *string
	if provider != "" {
		provPtr = &provider
	}

	resp, err := client.ListRemoteRepositoriesFromProvider(
		ctx,
		&minderv1.ListRemoteRepositoriesFromProviderRequest{
			Context: &minderv1.Context{
				Provider: provPtr,
				Project:  &project,
			},
		},
	)
	if err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func selectReposInteractively(
	cmd *cobra.Command,
	unregisteredRepos map[string]*minderv1.UpstreamRepositoryRef,
) ([]*minderv1.UpstreamRepositoryRef, error) {
	effectiveRepos := make([]*minderv1.UpstreamRepositoryRef, 0)

	repoNames := make([]string, 0, len(unregisteredRepos))
	for repoName := range unregisteredRepos {
		repoNames = append(repoNames, repoName)
	}

	selected, err := cli.MultiSelect(repoNames)
	if err != nil {
		return nil, err
	}

	for _, name := range selected {
		effectiveRepos = append(effectiveRepos, unregisteredRepos[name])
	}

	if len(effectiveRepos) == 0 {
		cmd.Println("No repositories selected")
	}

	return effectiveRepos, nil
}

func registerRepos(
	project string,
	client minderv1.RepositoryServiceClient,
	repos []*minderv1.UpstreamRepositoryRef,
) ([]*minderv1.RegisterRepoResult, []string) {
	var results []*minderv1.RegisterRepoResult
	var warnings []string
	for _, repo := range repos {
		result, err := client.RegisterRepository(
			context.Background(),
			&minderv1.RegisterRepositoryRequest{
				Context: &minderv1.Context{
					Provider: ptr.Ptr(repo.GetContext().GetProvider()),
					Project:  &project,
				},
				Repository: repo,
			},
		)

		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Error registering repository %s: %s", repo.Name, err))
			continue
		}
		results = append(results, result.Result)
	}

	return results, warnings
}

func printRepoRegistrationStatus(cmd *cobra.Command, results []*minderv1.RegisterRepoResult) {
	// If there were no results, print a message and return
	if len(results) == 0 {
		cmd.Println("No repositories registered")
		return
	}

	t := table.New(table.Simple, layouts.Default, []string{"Repository", "Status", "Message"})
	for _, result := range results {
		// in the case of a malformed response, skip over it to avoid segfaulting
		if result.Repository == nil {
			cmd.Printf("Skipping malformed response: %v", result)
		}
		row := []string{cli.GetRepositoryName(result.Repository.Owner, result.Repository.Name)}
		if result.Status.Success {
			row = append(row, "Registered")
		} else {
			row = append(row, "Failed")
		}

		if result.Status.Error != nil {
			row = append(row, *result.Status.Error)
		} else {
			row = append(row, "")
		}
		t.AddRow(row...)
	}
	t.Render()
}

var errAutoRegistrationAlreadyEnabled = errors.New("auto registration is already enabled")

type autoRegisterResult struct {
	updatedProviders []string
	failedProviders  []string
	alreadyEnabled   []string
}

func enableAutoRegistration(
	ctx context.Context,
	provCli minderv1.ProvidersServiceClient,
	project, provName string,
) (autoRegisterResult, error) {
	if provName != "" {
		err := enableAutoRegistrationForProvider(ctx, provCli, project, provName)
		if err != nil {
			if errors.Is(err, errAutoRegistrationAlreadyEnabled) {
				return autoRegisterResult{alreadyEnabled: []string{provName}}, nil
			}
			return autoRegisterResult{failedProviders: []string{provName}}, err
		}
		return autoRegisterResult{updatedProviders: []string{provName}}, nil
	}
	return enableAutoRegistrationAllProviders(ctx, provCli, project)
}

func enableAutoRegistrationAllProviders(
	ctx context.Context,
	provCli minderv1.ProvidersServiceClient,
	project string,
) (autoRegisterResult, error) {
	var cursor string
	ret := autoRegisterResult{}

	for {
		allProviders, err := provCli.ListProviders(ctx, &minderv1.ListProvidersRequest{
			Context: &minderv1.Context{
				Project: &project,
			},
			Cursor: cursor,
		})

		if err != nil {
			return ret, cli.MessageAndError("failed to get providers", err)
		}

		cursor = allProviders.Cursor

		for _, provider := range allProviders.Providers {
			if !slices.Contains(provider.GetImplements(), minderv1.ProviderType_PROVIDER_TYPE_REPO_LISTER) {
				continue
			}

			err := enableAutoRegistrationForProvider(ctx, provCli, project, provider.Name)
			if err != nil {
				if errors.Is(err, errAutoRegistrationAlreadyEnabled) {
					// Store the provider name that was already enabled
					ret.alreadyEnabled = append(ret.alreadyEnabled, provider.Name)
					continue
				}
				// Store the provider name that failed to update
				ret.failedProviders = append(ret.failedProviders, provider.Name)
				// we could print a diagnostic message here, but since the legacy github provider doesn't support
				// auto-enrollment and we still pre-create it, the user would see the message all the time.
				continue
			}
			// Store the provider name that was successfully updated
			ret.updatedProviders = append(ret.updatedProviders, provider.Name)
		}

		if allProviders.Cursor == "" {
			break
		}
	}

	// If there are failed providers, return an error
	if len(ret.failedProviders) > 0 {
		return ret, fmt.Errorf("failed to enable auto registration for some providers")
	}
	return ret, nil
}

func enableAutoRegistrationForProvider(
	ctx context.Context,
	provCli minderv1.ProvidersServiceClient,
	project, providerName string,
) error {
	serde, err := cli.GetProviderConfig(ctx, provCli, project, providerName)
	if err != nil {
		return cli.MessageAndError("failed to get provider config", err)
	}

	if serde == nil {
		serde = &cli.ProviderConfigUnion{}
	}

	if serde.ProviderConfig == nil {
		serde.ProviderConfig = &minderv1.ProviderConfig{}
	}

	if serde.AutoRegistration == nil {
		serde.AutoRegistration = &minderv1.AutoRegistration{}
	}

	if serde.AutoRegistration.Entities == nil {
		serde.AutoRegistration.Entities = make(map[string]*minderv1.EntityAutoRegistrationConfig)
	}

	repoReg, ok := serde.AutoRegistration.Entities[minderv1.Entity_ENTITY_REPOSITORIES.ToString()]
	if !ok {
		repoReg = &minderv1.EntityAutoRegistrationConfig{}
	}

	if repoReg.GetEnabled() {
		return errAutoRegistrationAlreadyEnabled
	}

	repoReg.Enabled = proto.Bool(true)
	serde.AutoRegistration.Entities[minderv1.Entity_ENTITY_REPOSITORIES.ToString()] = repoReg

	err = cli.SetProviderConfig(ctx, provCli, project, providerName, serde)
	if err != nil {
		return cli.MessageAndError("failed to update provider", err)
	}

	_, err = provCli.ReconcileEntityRegistration(ctx, &minderv1.ReconcileEntityRegistrationRequest{
		Context: &minderv1.Context{
			Provider: &providerName,
			Project:  &project,
		},
		Entity: minderv1.Entity_ENTITY_REPOSITORIES.ToString(),
	})
	if err != nil {
		return cli.MessageAndError("Error reconciling provider registration", err)
	}

	return nil
}

func printWarnings(cmd *cobra.Command, warnings []string) {
	for _, warning := range warnings {
		cmd.Println(warning)
	}
}

func init() {
	RepoCmd.AddCommand(repoRegisterCmd)
	// Flags
	repoRegisterCmd.Flags().StringSliceP("name", "n", []string{}, "List of repository names to register, i.e owner/repo,owner/repo")
	repoRegisterCmd.Flags().BoolP("all", "a", false, "Register all unregistered repositories")
}
