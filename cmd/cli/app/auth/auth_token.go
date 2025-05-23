// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	"github.com/mindersec/minder/internal/util/cli"
	"github.com/mindersec/minder/pkg/config"
	clientconfig "github.com/mindersec/minder/pkg/config/client"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print your token for Minder",
	Long: `The token command allows for printing the token for Minder. This is useful
for using with automation scripts or other tools.`,
	RunE: TokenCommand,
}

// TokenCommand is the token subcommand
func TokenCommand(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	clientConfig, err := config.ReadConfigFromViper[clientconfig.Config](viper.GetViper())
	if err != nil {
		return cli.MessageAndError("Unable to read config", err)
	}

	skipBrowser := viper.GetBool("token.skip-browser")

	// No longer print usage on returned error, since we've parsed our inputs
	// See https://github.com/spf13/cobra/issues/340#issuecomment-374617413
	cmd.SilenceUsage = true

	grpcCfg := clientConfig.GRPCClientConfig
	opts := []grpc.DialOption{grpcCfg.TransportCredentialsOption()}
	// save credentials
	issuerUrl := clientConfig.Identity.CLI.IssuerUrl
	clientId := clientConfig.Identity.CLI.ClientId
	realm := clientConfig.Identity.CLI.Realm
	creds, err := cli.GetToken(grpcCfg.GetGRPCAddress(), opts, issuerUrl, realm, clientId)
	if err != nil {
		cmd.Printf("Error getting token: %v\n", err)
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, cli.ErrGettingRefreshToken) {
			// wait for the token to be received
			token, err := cli.Login(ctx, cmd, clientConfig, []string{}, skipBrowser)
			if err != nil {
				return err
			}

			creds = token.AccessToken
		} else {
			return cli.MessageAndError("Error getting token", err)
		}
	}

	cmd.Print(creds)
	return nil
}

func init() {
	AuthCmd.AddCommand(tokenCmd)

	// hidden flags
	tokenCmd.Flags().BoolP("skip-browser", "", false, "Skip opening the browser for OAuth flow")
	// Bind flags
	if err := viper.BindPFlag("token.skip-browser", tokenCmd.Flags().Lookup("skip-browser")); err != nil {
		panic(err)
	}
}
