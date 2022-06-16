/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"os/exec"

	"github.com/kubeshop/testkube/pkg/ui"
	"github.com/spf13/cobra"
)

var installOnUpgrade bool

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade kusk-gateway, envoy-fleet, api, and dashboard in a single command",
	Long: `
	Upgrade kusk-gateway, envoy-fleet, api, and dashboard in a single command.

	$ kusk upgrade

	Will upgrade kusk-gateway, a public (for your APIS) and private (for the kusk dashboard and api) 
	envoy-fleet, api, and dashboard in the kusk-system namespace using helm.

	$ kusk upgrade --name=my-release --namespace=my-namespace

	Will upgrade a helm release named with --name in the namespace specified by --namespace.

	$ kusk upgrade --install

	Will upgrade kusk-gateway, the dashboard, api, and envoy-fleets and install them if they are not installed`,
	Run: func(cmd *cobra.Command, args []string) {
		helmPath, err := exec.LookPath("helm")
		ui.ExitOnError("looking for helm", err)

		ui.Info("adding the kubeshop helm repository")
		err = addKubeshopHelmRepo(helmPath)
		ui.ExitOnError("adding kubeshop repo", err)
		ui.Info(ui.Green("done"))

		ui.Info("fetching the latest charts")
		err = updateHelmRepos(helmPath)
		ui.ExitOnError("updating helm repositories", err)
		ui.Info(ui.Green("done"))

		releases, err := listReleases(helmPath, releaseName, releaseNamespace)
		ui.ExitOnError("listing existing releases", err)

		if _, kuskGatewayInstalled := releases[releaseName]; kuskGatewayInstalled || installOnUpgrade {
			ui.Info("upgrading Kusk Gateway")
			err = installKuskGateway(helmPath, releaseName, releaseNamespace)
			ui.ExitOnError("upgrading kusk gateway", err)
			ui.Info(ui.Green("done"))
		} else {
			ui.Info("kusk gateway not installed and --install not specified, skipping")
		}

		envoyFleetName := fmt.Sprintf("%s-envoy-fleet", releaseName)

		if _, publicEnvoyFleetInstalled := releases[envoyFleetName]; publicEnvoyFleetInstalled || installOnUpgrade {
			if !noEnvoyFleet {
				ui.Info("upgrading Envoy Fleet")
				err = installPublicEnvoyFleet(helmPath, envoyFleetName, releaseNamespace)
				ui.ExitOnError("upgrading envoy fleet", err)
				ui.Info(ui.Green("done"))
			} else {
				ui.Info(ui.LightYellow("--no-envoy-fleet set - skipping envoy fleet installation"))
			}
		} else {
			ui.Info("envoy fleet not installed and --install not specified, skipping")
		}

		envoyFleetName = fmt.Sprintf("%s-private-envoy-fleet", releaseName)

		if _, privateEnvoyFleetInstalled := releases[envoyFleetName]; privateEnvoyFleetInstalled || installOnUpgrade {
			err = installPrivateEnvoyFleet(helmPath, envoyFleetName, releaseNamespace)
			ui.ExitOnError("upgrading envoy fleet", err)
		} else {
			ui.Info("private envoy fleet not installed and --install not specified, skipping")
		}

		apiReleaseName := fmt.Sprintf("%s-api", releaseName)
		if _, apiInstalled := releases[apiReleaseName]; apiInstalled || installOnUpgrade {
			ui.Info("upgrading Kusk API")
			err = installApi(helmPath, apiReleaseName, releaseNamespace, envoyFleetName)
			ui.ExitOnError("upgrading api", err)
			ui.Info(ui.Green("done"))
		} else {
			ui.Info("api not installed and --install not specified, skipping")
		}

		dashboardReleaseName := fmt.Sprintf("%s-dashboard", releaseName)
		if _, dashboardInstalled := releases[dashboardReleaseName]; dashboardInstalled || installOnUpgrade {
			ui.Info("upgrading Kusk Dashboard")
			err = installDashboard(helmPath, dashboardReleaseName, releaseNamespace, envoyFleetName)
			ui.ExitOnError("upgrading dashboard", err)

			ui.Info(ui.Green("done"))
		} else {
			ui.Info("dashboard not installed and --install not specified, skipping")
		}

		ui.Info(ui.Green("upgrade complete"))
	},
}

func init() {
	rootCmd.AddCommand(upgradeCmd)

	upgradeCmd.Flags().StringVar(&releaseName, "name", "kusk-gateway", "installation name")
	upgradeCmd.Flags().StringVar(&releaseNamespace, "namespace", "kusk-system", "namespace to upgrade in")
	upgradeCmd.Flags().BoolVar(&installOnUpgrade, "install", false, "install components if not installed")
}
