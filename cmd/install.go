/*
The MIT License (MIT)

Copyright © 2022 Kubeshop

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.

*/
package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kubeshop/testkube/pkg/process"
	"github.com/kubeshop/testkube/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	noApi                         bool
	noDashboard                   bool
	noEnvoyFleet                  bool
	releaseName, releaseNamespace string
)

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().StringVar(&releaseName, "name", "kusk-gateway", "installation name")
	installCmd.Flags().StringVar(&releaseNamespace, "namespace", "kusk-system", "namespace to install in")

	installCmd.Flags().BoolVar(&noDashboard, "no-dashboard", false, "don't the install dashboard")
	installCmd.Flags().BoolVar(&noApi, "no-api", false, "don't install the api. Setting this flag implies --no-dashboard")
	installCmd.Flags().BoolVar(&noEnvoyFleet, "no-envoy-fleet", false, "don't install any envoy fleets")
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install kusk-gateway, envoy-fleet, api, and dashboard in a single command",
	Long: `
	Install kusk-gateway, envoy-fleet, api, and dashboard in a single command.

	$ kusk install

	Will install kusk-gateway, a public (for your APIS) and private (for the kusk dashboard and api) 
	envoy-fleet, api, and dashboard in the kusk-system namespace using helm.

	$ kusk install --name=my-release --namespace=my-namespace

	Will create a helm release named with --name in the namespace specified by --namespace.

	$ kusk install --no-dashboard --no-api --no-envoy-fleet

	Will install kusk-gateway, but not the dashboard, api, or envoy-fleet.
	`,
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

		ui.Info("installing Kusk Gateway")
		err = installKuskGateway(helmPath, releaseName, releaseNamespace)
		ui.ExitOnError("Installing kusk gateway", err)
		ui.Info(ui.Green("done"))

		if !noEnvoyFleet {
			ui.Info("installing Envoy Fleet")
			err = installPublicEnvoyFleet(helmPath, releaseName, releaseNamespace)
			ui.ExitOnError("Installing envoy fleet", err)
			ui.Info(ui.Green("done"))
		} else {
			ui.Info(ui.LightYellow("--no-envoy-fleet set - skipping envoy fleet installation"))
		}

		if noApi {
			ui.Info(ui.LightYellow("--no-api set - skipping api installation"))
			return
		}

		ui.Info("installing Kusk API")
		envoyFleetName := fmt.Sprintf("%s-envoy-fleet", releaseName)
		if !noEnvoyFleet {
			envoyFleetName = fmt.Sprintf("%s-private-envoy-fleet", releaseName)
			err = installPrivateEnvoyFleet(helmPath, fmt.Sprintf("%s-private", releaseName), releaseNamespace)
			ui.ExitOnError("Installing envoy fleet", err)
		}

		err = installApi(helmPath, releaseName, releaseNamespace, envoyFleetName)
		ui.ExitOnError("Installing api", err)
		ui.Info(ui.Green("done"))

		if noDashboard {
			ui.Info(ui.LightYellow("--no-dashboard set - skipping dashboard installation"))
			printPortForwardInstructions("API", releaseNamespace, envoyFleetName)
			return
		}

		ui.Info("installing Kusk Dashboard")
		err = installDashboard(helmPath, releaseName, releaseNamespace, envoyFleetName)
		ui.ExitOnError("Installing dashboard", err)

		ui.Info(ui.Green("done"))

		printPortForwardInstructions("dashboard", releaseNamespace, envoyFleetName)
	},
}

func printPortForwardInstructions(service, releaseNamespace, envoyFleetName string) {
	ui.Info(ui.Green("To access the " + service + ", port forward to the envoy-fleet service that exposes it"))
	ui.Info(ui.LightBlue(fmt.Sprintf("\t$ kubectl port-forward -n %s svc/%s 8080:80", releaseNamespace, envoyFleetName)))

	endpoint := "http://localhost:8080/"
	if service == "api" {
		endpoint += "api"
	}
	ui.Info(ui.LightBlue("\tand go " + endpoint))
}

func addKubeshopHelmRepo(helmPath string) error {
	_, err := process.Execute(helmPath, "repo", "add", "kubeshop", "https://kubeshop.github.io/helm-charts/")
	if err != nil && !strings.Contains(err.Error(), "Error: repository name (kubeshop) already exists, please specify a different name") {
		return err
	}

	return nil
}

func updateHelmRepos(helmPath string) error {
	_, err := process.Execute(helmPath, "repo", "update")
	return err
}

func installKuskGateway(helmPath, releaseName, releaseNamespace string) error {
	command := []string{
		"upgrade",
		"--install",
		"--wait",
		"--create-namespace",
		"--namespace",
		releaseNamespace,
		"--set", fmt.Sprintf("fullnameOverride=%s", releaseName),
		releaseName,
		"kubeshop/kusk-gateway",
	}

	out, err := process.Execute(helmPath, command...)
	if err != nil {
		return err
	}

	ui.Debug("helm install kusk gateway output", string(out))

	return nil
}

func installPublicEnvoyFleet(helmPath, releaseName, releaseNamespace string) error {
	return installEnvoyFleet(helmPath, releaseName, releaseNamespace, "LoadBalancer")
}

func installPrivateEnvoyFleet(helmPath, releaseName, releaseNamespace string) error {
	return installEnvoyFleet(helmPath, releaseName, releaseNamespace, "ClusterIP")
}

func installEnvoyFleet(helmPath, releaseName, releaseNamespace, serviceType string) error {
	envoyFleetName := fmt.Sprintf("%s-envoy-fleet", releaseName)
	command := []string{
		"upgrade",
		"--install",
		"--wait",
		"--create-namespace",
		"--namespace",
		releaseNamespace,
		"--set", fmt.Sprintf("fullnameOverride=%s", envoyFleetName),
		"--set", fmt.Sprintf("service.type=%s", serviceType),
		envoyFleetName,
		"kubeshop/kusk-gateway-envoyfleet",
	}

	out, err := process.Execute(helmPath, command...)
	if err != nil {
		return err
	}

	ui.Debug("helm install envoy fleet output", string(out))

	return nil
}

func installApi(helmPath, releaseName, releaseNamespace, envoyFleetName string) error {
	command := []string{
		"upgrade",
		"--install",
		"--wait",
		"--create-namespace",
		"--namespace",
		releaseNamespace,
		"--set", fmt.Sprintf("fullnameOverride=%s-api", releaseName),
		"--set", fmt.Sprintf("envoyfleet.name=%s", envoyFleetName),
		"--set", fmt.Sprintf("envoyfleet.namespace=%s", releaseNamespace),
		fmt.Sprintf("%s-api", releaseName),
		"kubeshop/kusk-gateway-api",
	}

	out, err := process.Execute(helmPath, command...)
	if err != nil {
		return err
	}

	ui.Debug("helm install api output", string(out))

	return nil
}

func installDashboard(helmPath, releaseName, releaseNamespace, envoyFleetName string) error {
	command := []string{
		"upgrade",
		"--install",
		"--wait",
		"--create-namespace",
		"--namespace",
		releaseNamespace,
		"--set", fmt.Sprintf("fullnameOverride=%s-dashboard", releaseName),
		"--set", fmt.Sprintf("envoyfleet.name=%s", envoyFleetName),
		"--set", fmt.Sprintf("envoyfleet.namespace=%s", releaseNamespace),
		fmt.Sprintf("%s-dashboard", releaseName),
		"kubeshop/kusk-gateway-dashboard",
	}

	out, err := process.Execute(helmPath, command...)
	if err != nil {
		return err
	}

	ui.Debug("helm install dashboard output", string(out))

	return nil
}
